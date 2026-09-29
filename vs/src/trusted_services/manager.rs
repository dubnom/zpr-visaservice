//! Coordination and revision tracking across trusted services.

use arc_swap::ArcSwap;
use dashmap::DashMap;
use futures::future::join_all;
use std::collections::HashMap;
use std::net::IpAddr;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use serde::Serialize;

use libeval::attribute::Attribute;

use crate::error::ServiceError;

use super::TrustedServiceInterface;

/// Coordinates concurrent access to the configured trusted-service implementations.
pub struct TrustedServicesMgr {
    services: ArcSwap<Vec<Arc<dyn TrustedServiceInterface>>>,
    lookup_outcomes: DashMap<String, LookupOutcome>,
    /// Per actor (keyed by ZPR address) and source, the revision from which attributes
    /// were last refreshed. ZPR addresses are recycled from a pool, so entries MUST be
    /// purged on disconnect ([TrustedServicesMgr::forget_actor_revisions]) before the
    /// address can be reassigned.
    actor_revisions: DashMap<IpAddr, HashMap<String, u64>>,
}

#[derive(Clone, Serialize)]
pub struct TrustedServiceStatus {
    pub name: String,
    pub health: &'static str,
    pub last_lookup_ms: Option<u64>,
    pub last_success_ms: Option<u64>,
}

#[derive(Clone)]
struct LookupOutcome {
    succeeded: bool,
    last_lookup_ms: u64,
    last_success_ms: Option<u64>,
}

impl TrustedServicesMgr {
    /// Create a manager with no configured services.
    pub fn new() -> Self {
        Self {
            services: ArcSwap::new(Arc::new(Vec::new())),
            lookup_outcomes: DashMap::new(),
            actor_revisions: DashMap::new(),
        }
    }

    /// Return sources whose current revision differs from the record for the actor at
    /// `zpr_addr`. A source with no record at all is stale: the actor has never been
    /// refreshed from it (or the last attempt failed), so it must be consulted even
    /// when the actor holds no attribute from it -- otherwise a source that vended
    /// nothing on the first lookup would be skipped forever, including after a flush
    /// adds attributes for the actor.
    pub fn stale_sources_for_actor(&self, zpr_addr: &IpAddr) -> Vec<(String, u64)> {
        let services = self.services.load_full();
        let recorded = self.actor_revisions.get(zpr_addr);
        services
            .iter()
            .filter_map(|service| {
                let source_id = service.get_source_id();
                let recorded_revision = recorded
                    .as_ref()
                    .and_then(|revisions| revisions.value().get(source_id).copied());
                let current_revision = service.current_revision();
                (recorded_revision != Some(current_revision))
                    .then(|| (source_id.to_string(), current_revision))
            })
            .collect()
    }

    /// Record the source revision used to refresh the attributes of the actor at `zpr_addr`.
    pub fn record_revision(&self, zpr_addr: &IpAddr, source: &str, revision: u64) {
        self.actor_revisions
            .entry(*zpr_addr)
            .or_default()
            .insert(source.to_string(), revision);
    }

    /// Drop all recorded per-source revisions for the actor at `zpr_addr`. Call before
    /// the address returns to the pool, so a recycled address cannot inherit them.
    pub fn forget_actor_revisions(&self, zpr_addr: &IpAddr) {
        self.actor_revisions.remove(zpr_addr);
    }

    /// Atomically replace the entire trusted-service list.
    pub fn update_services(&self, services: Vec<Arc<dyn TrustedServiceInterface>>) {
        self.lookup_outcomes.clear();
        self.services.store(Arc::new(services));
    }

    pub fn statuses(&self) -> Vec<TrustedServiceStatus> {
        let mut statuses: Vec<_> = self
            .services
            .load()
            .iter()
            .map(|service| {
                let name = service.get_source_id().to_string();
                let outcome = self.lookup_outcomes.get(&name);
                TrustedServiceStatus {
                    name,
                    health: match outcome.as_ref() {
                        None => "unverified",
                        Some(value) if value.succeeded => "working",
                        Some(_) => "failed",
                    },
                    last_lookup_ms: outcome.as_ref().map(|value| value.last_lookup_ms),
                    last_success_ms: outcome.as_ref().and_then(|value| value.last_success_ms),
                }
            })
            .collect();
        statuses.sort_by(|a, b| a.name.cmp(&b.name));
        statuses
    }

    fn record_lookup(&self, name: &str, succeeded: bool) {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|duration| u64::try_from(duration.as_millis()).unwrap_or(u64::MAX))
            .unwrap_or(0);
        let last_success_ms = if succeeded {
            Some(now)
        } else {
            self.lookup_outcomes
                .get(name)
                .and_then(|outcome| outcome.last_success_ms)
        };
        self.lookup_outcomes.insert(
            name.to_string(),
            LookupOutcome {
                succeeded,
                last_lookup_ms: now,
                last_success_ms,
            },
        );
    }

    /// Query every trusted service concurrently for an actor's attributes.
    ///
    /// `identities` is the actor's lookup-identity (key, value) set; see
    /// [TrustedServiceInterface::get_attributes_for_actor].
    ///
    /// Each result is paired with the service's source id so the caller can attribute
    /// it (e.g. to derive `user.zpr.authority` from the vending source) without
    /// trusting the source string stamped on the returned attributes themselves.
    pub async fn get_attributes_for_actor(
        &self,
        identities: &[(String, String)],
    ) -> Vec<(String, Result<Vec<Attribute>, ServiceError>)> {
        let snapshot = self.services.load_full();
        let futures = snapshot.iter().map(|service| {
            let service = service.clone();
            async move {
                let name = service.get_source_id().to_string();
                let result = service.get_attributes_for_actor(identities).await;
                self.record_lookup(&name, result.is_ok());
                (name, result)
            }
        });
        join_all(futures).await
    }

    /// Query one named trusted service for an actor's attributes.
    ///
    /// `identities` is the actor's lookup-identity (key, value) set; see
    /// [TrustedServiceInterface::get_attributes_for_actor].
    pub async fn get_attributes_from_source_for_actor(
        &self,
        source_ident: &str,
        identities: &[(String, String)],
    ) -> Vec<Result<Vec<Attribute>, ServiceError>> {
        let snapshot = self.services.load_full();
        if let Some(service) = snapshot
            .iter()
            .find(|service| service.get_source_id() == source_ident)
        {
            let service = service.clone();
            let result = service.get_attributes_for_actor(identities).await;
            self.record_lookup(source_ident, result.is_ok());
            return vec![result];
        }

        vec![Err(ServiceError::TrustedServiceNotFound(
            source_ident.to_string(),
        ))]
    }

    /// Flush every trusted service without stopping after an individual failure.
    #[allow(dead_code)]
    pub async fn flush_all(&self) -> Vec<Result<(), ServiceError>> {
        let snapshot = self.services.load_full();
        let futures = snapshot.iter().map(|service| {
            let service = service.clone();
            async move { service.flush().await }
        });
        join_all(futures).await
    }

    /// Flush one named trusted service.
    pub async fn flush_one(&self, source_ident: &str) -> Result<(), ServiceError> {
        let snapshot = self.services.load_full();
        if let Some(service) = snapshot
            .iter()
            .find(|service| service.get_source_id() == source_ident)
        {
            let service = service.clone();
            return service.flush().await;
        }

        Err(ServiceError::TrustedServiceNotFound(
            source_ident.to_string(),
        ))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trusted_services::file_attribute_store::FileAttributeStore;
    use crate::trusted_services::test_support::{test_mapper, write_fixture};
    use std::fs;
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::time::Duration;

    struct FlakyService(AtomicBool);

    #[async_trait::async_trait]
    impl TrustedServiceInterface for FlakyService {
        async fn get_attributes_for_actor(
            &self,
            _identities: &[(String, String)],
        ) -> Result<Vec<Attribute>, ServiceError> {
            if self.0.load(Ordering::Relaxed) {
                Err(ServiceError::AttributesIndeterminate("unavailable".into()))
            } else {
                Ok(Vec::new())
            }
        }

        async fn flush(&self) -> Result<(), ServiceError> {
            Ok(())
        }
        fn current_revision(&self) -> u64 {
            1
        }
        fn get_source_id(&self) -> &str {
            "test"
        }
    }

    /// Flushing all services reaches each registered implementation.
    #[tokio::test]
    async fn test_manager_flush_all_reaches_every_service() {
        let fp = write_fixture(
            "vs-fas-flush-all.json",
            r#"{"device.zpr.adapter.cn": {"alice": {"color": ["red"]}}}"#,
        );
        let manager = TrustedServicesMgr::new();
        let store = Arc::new(
            FileAttributeStore::new(
                "test".to_string(),
                test_mapper(),
                Duration::from_secs(3600),
                &fp,
            )
            .unwrap(),
        );
        manager.update_services(vec![store.clone()]);

        let revision_before = store.current_revision();
        let results = manager.flush_all().await;
        assert_eq!(results.len(), 1);
        assert!(results[0].is_ok());
        assert!(store.current_revision() > revision_before);

        fs::remove_file(&fp).unwrap();
    }

    /// Actor revision records become stale whenever a relevant service advances.
    #[tokio::test]
    async fn test_stale_sources_for_actor_tracks_revisions() {
        let fp = write_fixture(
            "vs-fas-stale.json",
            r#"{"device.zpr.adapter.cn": {"alice": {"color": ["red"]}}}"#,
        );
        let manager = TrustedServicesMgr::new();
        let store = Arc::new(
            FileAttributeStore::new(
                "test".to_string(),
                test_mapper(),
                Duration::from_secs(3600),
                &fp,
            )
            .unwrap(),
        );
        manager.update_services(vec![store.clone()]);

        // A configured source the actor has no record for is stale, even though the
        // actor holds no attribute from it -- otherwise a source that vended nothing on
        // the first lookup would never be consulted again.
        let addr: IpAddr = "fd5a:5052::a1".parse().unwrap();
        let stale = manager.stale_sources_for_actor(&addr);
        assert_eq!(stale, vec![("test".to_string(), store.current_revision())]);

        manager.record_revision(&addr, "test", stale[0].1);
        assert!(manager.stale_sources_for_actor(&addr).is_empty());

        store.flush().await.unwrap();
        assert_eq!(
            manager.stale_sources_for_actor(&addr),
            vec![("test".to_string(), store.current_revision())]
        );

        // Forgetting the actor's records makes the source stale again, so a recycled
        // address cannot inherit the previous actor's revision history.
        manager.record_revision(&addr, "test", store.current_revision());
        assert!(manager.stale_sources_for_actor(&addr).is_empty());
        manager.forget_actor_revisions(&addr);
        assert_eq!(
            manager.stale_sources_for_actor(&addr),
            vec![("test".to_string(), store.current_revision())]
        );

        fs::remove_file(&fp).unwrap();
    }

    #[tokio::test]
    async fn test_status_reflects_actual_lookups() {
        let manager = TrustedServicesMgr::new();
        let store = Arc::new(FlakyService(AtomicBool::new(false)));
        manager.update_services(vec![store.clone()]);
        assert_eq!(manager.statuses()[0].health, "unverified");

        let identities = [("device.zpr.adapter.cn".into(), "alice".into())];
        assert!(
            manager
                .get_attributes_from_source_for_actor("test", &identities)
                .await[0]
                .is_ok()
        );
        let working = manager.statuses().pop().unwrap();
        assert_eq!(working.health, "working");
        assert!(working.last_success_ms.is_some());

        store.0.store(true, Ordering::Relaxed);
        assert!(
            manager.get_attributes_for_actor(&identities).await[0]
                .1
                .is_err()
        );
        let failed = manager.statuses().pop().unwrap();
        assert_eq!(failed.health, "failed");
        assert_eq!(failed.last_success_ms, working.last_success_ms);
        assert!(failed.last_lookup_ms.is_some());

        manager.update_services(Vec::new());
        assert!(manager.statuses().is_empty());
    }
}
