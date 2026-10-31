//! Coordination and revision tracking across trusted services.

use arc_swap::ArcSwap;
use dashmap::DashMap;
use futures::future::join_all;
use std::collections::{HashMap, HashSet};
use std::net::IpAddr;
use std::sync::Arc;
use std::time::{SystemTime, UNIX_EPOCH};

use serde::Serialize;

use libeval::attribute::{Attribute, AttributeSource};

use crate::error::ServiceError;

use super::TrustedServiceInterface;
use super::attribute_mapper::source_qualified_key;

/// Coordinates concurrent access to the configured trusted-service implementations.
pub struct TrustedServicesMgr {
    services: ArcSwap<ServiceSnapshot>,
    pub(crate) control_lock: tokio::sync::Mutex<()>,
    /// Per actor (keyed by ZPR address) and source, the revision from which attributes
    /// were last refreshed. ZPR addresses are recycled from a pool, so entries MUST be
    /// purged on disconnect ([TrustedServicesMgr::forget_actor_revisions]) before the
    /// address can be reassigned.
    actor_revisions: DashMap<IpAddr, HashMap<String, u64>>,
}

struct ServiceSnapshot {
    services: Vec<Arc<dyn TrustedServiceInterface>>,
    disabled: HashSet<String>,
    // Retired lookups must not publish health into a replacement configuration.
    lookup_outcomes: DashMap<String, LookupOutcome>,
}

/// Pins a publication so replacing and re-adding the same stores is detectable.
pub(crate) struct ServiceConfiguration(Arc<ServiceSnapshot>);

#[derive(Clone, Serialize)]
pub struct TrustedServiceStatus {
    pub name: String,
    pub enabled: bool,
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

pub(crate) fn qualify_source_attributes(
    attrs: Vec<Attribute>,
    source: &str,
    allow_unqualified: bool,
) -> Vec<Attribute> {
    let source_attributes = AttributeSource::new(source.to_string());
    let mut qualified = Vec::with_capacity(attrs.len() * if allow_unqualified { 2 } else { 1 });
    for attr in attrs {
        let key = attr.get_key().to_string();
        let namespaced_key = source_qualified_key(&key, source);
        qualified.push(
            source_attributes
                .builder(namespaced_key)
                .expires(attr.get_expires())
                .values(attr.get_value()),
        );
        if allow_unqualified {
            qualified.push(attr);
        }
    }
    qualified
}

impl TrustedServicesMgr {
    pub(crate) fn configuration(&self) -> ServiceConfiguration {
        ServiceConfiguration(self.services.load_full())
    }

    pub(crate) fn configuration_is_current(&self, configuration: &ServiceConfiguration) -> bool {
        Arc::ptr_eq(&configuration.0, &self.services.load_full())
    }

    /// Create a manager with no configured services.
    pub fn new() -> Self {
        Self {
            services: ArcSwap::from_pointee(ServiceSnapshot {
                services: Vec::new(),
                disabled: HashSet::new(),
                lookup_outcomes: DashMap::new(),
            }),
            actor_revisions: DashMap::new(),
            control_lock: tokio::sync::Mutex::new(()),
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
            .services
            .iter()
            .filter_map(|service| {
                let source_id = service.get_source_id();
                let recorded_revision = recorded
                    .as_ref()
                    .and_then(|revisions| revisions.value().get(source_id).copied());
                if services.disabled.contains(source_id) {
                    return Some((source_id.to_string(), super::REVISION_NEVER));
                }
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

    /// Identify removed sources from persisted claim ownership and revision memory.
    pub(crate) fn removed_sources_for_actor<'a>(
        &self,
        zpr_addr: &IpAddr,
        observed_sources: impl Iterator<Item = &'a str>,
    ) -> HashSet<String> {
        let snapshot = self.services.load_full();
        let mut sources: HashSet<String> = observed_sources.map(str::to_string).collect();
        if let Some(recorded) = self.actor_revisions.get(zpr_addr) {
            sources.extend(recorded.keys().cloned());
        }
        for service in &snapshot.services {
            sources.remove(service.get_source_id());
        }
        sources
    }

    /// Call only after the actor's removed-source cleanup has been persisted.
    pub(crate) fn forget_source_revision(&self, zpr_addr: &IpAddr, source: &str) {
        if let Some(mut recorded) = self.actor_revisions.get_mut(zpr_addr) {
            recorded.remove(source);
        }
    }

    /// Drop all recorded per-source revisions for the actor at `zpr_addr`. Call before
    /// the address returns to the pool, so a recycled address cannot inherit them.
    pub fn forget_actor_revisions(&self, zpr_addr: &IpAddr) {
        self.actor_revisions.remove(zpr_addr);
    }

    /// Atomically replace the entire trusted-service list.
    pub fn update_services(&self, services: Vec<Arc<dyn TrustedServiceInterface>>) {
        self.services.rcu(|previous| ServiceSnapshot {
            services: services.clone(),
            disabled: previous.disabled.clone(),
            lookup_outcomes: DashMap::new(),
        });
    }

    pub(crate) fn source_enabled(&self, source: &str) -> bool {
        !self.services.load().disabled.contains(source)
    }

    pub(crate) fn set_source_enabled(&self, source: &str, enabled: bool) {
        self.services.rcu(|previous| {
            let mut disabled = previous.disabled.clone();
            if enabled {
                disabled.remove(source);
            } else {
                disabled.insert(source.to_string());
            }
            ServiceSnapshot {
                services: previous.services.clone(),
                disabled,
                lookup_outcomes: DashMap::new(),
            }
        });
    }

    pub(crate) fn declares_source(&self, source: &str) -> bool {
        self.services
            .load()
            .services
            .iter()
            .any(|service| service.get_source_id() == source)
    }

    pub fn statuses(&self) -> Vec<TrustedServiceStatus> {
        let snapshot = self.services.load_full();
        let mut statuses: Vec<_> = snapshot
            .services
            .iter()
            .map(|service| {
                let name = service.get_source_id().to_string();
                let outcome = snapshot.lookup_outcomes.get(&name);
                TrustedServiceStatus {
                    enabled: !snapshot.disabled.contains(&name),
                    name,
                    health: match outcome.as_ref() {
                        _ if snapshot.disabled.contains(service.get_source_id()) => "disabled",
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

    fn record_lookup(snapshot: &ServiceSnapshot, name: &str, succeeded: bool) {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .map(|duration| u64::try_from(duration.as_millis()).unwrap_or(u64::MAX))
            .unwrap_or(0);
        snapshot
            .lookup_outcomes
            .entry(name.to_string())
            .and_modify(|outcome| {
                outcome.succeeded = succeeded;
                outcome.last_lookup_ms = now;
                if succeeded {
                    outcome.last_success_ms = Some(now);
                }
            })
            .or_insert(LookupOutcome {
                succeeded,
                last_lookup_ms: now,
                last_success_ms: succeeded.then_some(now),
            });
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
        let allow_unqualified = snapshot.services.len() == 1;
        let futures = snapshot.services.iter().map(|service| {
            let service = service.clone();
            let snapshot = &snapshot;
            async move {
                let name = service.get_source_id().to_string();
                if snapshot.disabled.contains(&name) {
                    return (
                        name.clone(),
                        Err(ServiceError::AttributesIndeterminate(format!(
                            "trusted source '{name}' is disabled"
                        ))),
                    );
                }
                let result = service
                    .get_attributes_for_actor(identities)
                    .await
                    .map(|attrs| qualify_source_attributes(attrs, &name, allow_unqualified));
                Self::record_lookup(snapshot, &name, result.is_ok());
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
            .services
            .iter()
            .find(|service| service.get_source_id() == source_ident)
        {
            if snapshot.disabled.contains(source_ident) {
                return vec![Err(ServiceError::AttributesIndeterminate(format!(
                    "trusted source '{source_ident}' is disabled"
                )))];
            }
            let service = service.clone();
            let allow_unqualified = snapshot.services.len() == 1;
            let result = service
                .get_attributes_for_actor(identities)
                .await
                .map(|attrs| qualify_source_attributes(attrs, source_ident, allow_unqualified));
            Self::record_lookup(&snapshot, source_ident, result.is_ok());
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
        let futures = snapshot.services.iter().map(|service| {
            let service = service.clone();
            async move { service.flush().await }
        });
        join_all(futures).await
    }

    /// Flush one named trusted service.
    pub async fn flush_one(&self, source_ident: &str) -> Result<(), ServiceError> {
        let snapshot = self.services.load_full();
        if let Some(service) = snapshot
            .services
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
#[path = "manager_lifecycle_tests.rs"]
mod lifecycle_tests;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trusted_services::file_attribute_store::FileAttributeStore;
    use crate::trusted_services::test_support::{test_mapper, write_fixture};
    use std::fs;
    use std::sync::atomic::{AtomicBool, Ordering};
    use std::time::Duration;

    struct FlakyService(AtomicBool);

    #[tokio::test]
    async fn expired_identity_cannot_refresh_claims_from_either_real_store() {
        use libeval::attribute::AttributeSource;
        use std::time::SystemTime;

        let directory = tempfile::tempdir().unwrap();
        let manager = TrustedServicesMgr::new();
        let stores = ["company", "lab"]
            .into_iter()
            .map(|source| {
                let path = directory.path().join(format!("{source}.json"));
                fs::write(&path, r#"{"user.sub":{"same-subject":{"color":["red"]}}}"#).unwrap();
                Arc::new(
                    FileAttributeStore::new(
                        source.into(),
                        test_mapper(),
                        Duration::from_secs(600),
                        &path,
                    )
                    .unwrap(),
                ) as Arc<dyn TrustedServiceInterface>
            })
            .collect();
        manager.update_services(stores);

        for expired in [true, false] {
            let claim = AttributeSource::new("authenticated-issuer")
                .builder("user.sub")
                .expires(if expired {
                    SystemTime::now() - Duration::from_secs(1)
                } else {
                    SystemTime::now() + Duration::from_secs(600)
                })
                .value("same-subject");
            let identities = super::super::lookup_identities(&["user.sub"], [&claim].into_iter());
            if expired {
                assert!(matches!(
                    identities,
                    Err(ServiceError::AttributesIndeterminate(_))
                ));
                continue;
            }
            let identities = identities.unwrap();
            for (source, result) in manager.get_attributes_for_actor(&identities).await {
                let attrs = result.unwrap();
                assert_eq!(attrs.is_empty(), expired, "{source}");
                if !expired {
                    assert_eq!(attrs.len(), 1);
                    assert_eq!(attrs[0].get_key(), format!("user.color__source__{source}"));
                }
                let selected = manager
                    .get_attributes_from_source_for_actor(&source, &identities)
                    .await;
                assert_eq!(selected.len(), 1);
                assert_eq!(
                    selected[0].as_ref().unwrap().is_empty(),
                    expired,
                    "{source}"
                );
            }
        }
    }

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

    #[test]
    fn test_single_source_keeps_qualified_and_legacy_attribute_keys() {
        let attrs = qualify_source_attributes(
            vec![Attribute::builder("user.department").value("staff")],
            "directory",
            true,
        );
        assert_eq!(attrs.len(), 2);
        assert!(attrs.iter().any(|attr| attr.get_key() == "user.department"));
        assert!(
            attrs
                .iter()
                .any(|attr| attr.get_key() == "user.department__source__directory")
        );
    }

    #[tokio::test]
    async fn test_multisource_same_subject_is_queried_in_both_stores() {
        use libeval::actor::Actor;

        let company_path = write_fixture(
            "multisource-company.json",
            r#"{"user.sub":{"alice":{"color":["red"],"lazy":[""]}}}"#,
        );
        let lab_path = write_fixture(
            "multisource-lab.json",
            r#"{"user.sub":{"alice":{"color":["blue"],"roles":["reviewer"]}}}"#,
        );
        let company = Arc::new(
            FileAttributeStore::new(
                "company".into(),
                test_mapper(),
                Duration::from_secs(3600),
                &company_path,
            )
            .unwrap(),
        );
        let lab = Arc::new(
            FileAttributeStore::new(
                "lab".into(),
                test_mapper(),
                Duration::from_secs(3600),
                &lab_path,
            )
            .unwrap(),
        );
        let manager = TrustedServicesMgr::new();
        manager.update_services(vec![company.clone(), lab.clone()]);
        let identities = [("user.sub".into(), "alice".into())];
        let results = manager.get_attributes_for_actor(&identities).await;
        assert_eq!(results.len(), 2);
        let mut actor = Actor::new();
        for (source, result) in results {
            let attrs = result.unwrap();
            assert!(attrs.iter().all(|attr| attr.get_source() == source));
            for attr in attrs {
                actor.add_attribute(attr).unwrap();
            }
        }
        assert!(actor.get_attribute("user.color").is_none());
        assert_eq!(
            actor
                .get_attribute("user.color__source__company")
                .unwrap()
                .get_value(),
            &["red".to_string()]
        );
        assert_eq!(
            actor
                .get_attribute("user.color__source__lab")
                .unwrap()
                .get_value(),
            &["blue".to_string()]
        );
        assert_eq!(
            actor
                .get_attribute("user.role__source__lab")
                .unwrap()
                .get_value(),
            &["reviewer".to_string()]
        );
        assert!(actor.get_attribute("user.zpr.tag.lazy").is_none());
        assert!(
            actor
                .get_attribute("user.zpr.tag.lazy__source__company")
                .is_some()
        );

        let addr: IpAddr = "fd5a:5052::a1".parse().unwrap();
        manager.record_revision(&addr, "company", company.current_revision());
        manager.record_revision(&addr, "lab", lab.current_revision());
        company.flush().await.unwrap();
        assert_eq!(
            manager.stale_sources_for_actor(&addr),
            vec![("company".into(), company.current_revision())]
        );
        assert!(
            manager
                .get_attributes_from_source_for_actor("company", &identities)
                .await[0]
                .is_ok()
        );
        assert!(
            manager
                .get_attributes_from_source_for_actor("missing", &identities)
                .await[0]
                .is_err()
        );
        fs::remove_file(company_path).unwrap();
        fs::remove_file(lab_path).unwrap();
    }
    #[tokio::test]
    #[ignore = "requires source-built ZPR_TEST_ZPLC; run the documented compiler interop command"]
    async fn test_source_built_compiler_to_stores_to_real_evaluation() {
        use crate::trusted_services::factory::{build_services, trusted_service_definitions};
        use bytes::{Buf, Bytes};
        use libeval::{
            actor::Actor, attribute::key, eval::EvalContext, eval_result::PartialEvalResult,
            policy::Policy,
        };
        use zpr::{policy::v1 as policy_capnp, vsapi_types::PacketDesc};

        let compiler =
            std::env::var("ZPR_TEST_ZPLC").expect("set ZPR_TEST_ZPLC to the source-built compiler");
        let directory = std::env::temp_dir().join(format!("source-interop-{}", std::process::id()));
        fs::create_dir_all(&directory).unwrap();
        let fixture = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../libeval/tests/zpl");
        for use_default in [false, true] {
            let source = if use_default {
                let text = fs::read_to_string(fixture.join("source-qualified.zpl"))
                    .unwrap()
                    .replace("user.department__source__corp-1_a:", "user.department:")
                    .replace("tag cleared__source__corp-1_a", "tag cleared");
                let path = directory.join("default.zpl");
                fs::write(&path, text).unwrap();
                path
            } else {
                fixture.join("source-qualified.zpl")
            };
            let bundle = directory.join("source-qualified.bin2");
            let mut command = std::process::Command::new(&compiler);
            command
                .args(["--require-source-qualification", "-c"])
                .arg(fixture.join("source-qualified.zplc"))
                .arg("-o")
                .arg(&bundle)
                .arg(source);
            if use_default {
                command.args(["--default-source", "corp-1_a"]);
            }
            let output = command.output().unwrap();
            assert!(
                output.status.success(),
                "{}",
                String::from_utf8_lossy(&output.stderr)
            );
            let encoded = Bytes::from(fs::read(bundle).unwrap());
            let message = capnp::serialize::read_message(
                encoded.reader(),
                capnp::message::ReaderOptions::new(),
            )
            .unwrap();
            let container = message
                .get_root::<policy_capnp::policy_container::Reader>()
                .unwrap();
            let policy = Arc::new(
                Policy::new_from_policy_bytes(Bytes::copy_from_slice(
                    container.get_policy().unwrap(),
                ))
                .unwrap(),
            );
            // Real factory decoding must accept the compiler's raw provider mappings.
            let definitions = trusted_service_definitions(&policy).unwrap();
            assert_eq!(definitions.len(), 2);
            fs::write(directory.join("corp-1_a.json"), r#"{"user.sub":{"alice":{"cleared":[""],"department":["engineering__source__corp-1_a"],"content":["red__source__corp-1_a"]},"bob":{"cleared":[""],"department":["contractor"],"content":["red__source__corp-1_a"]}}}"#).unwrap();
            fs::write(directory.join("lab-2_b.json"), r#"{"user.sub":{"alice":{"department":["contractor"]},"bob":{"cleared":[""],"department":["engineering__source__corp-1_a"]}}}"#).unwrap();
            let manager = TrustedServicesMgr::new();
            manager.update_services(build_services(&definitions, &directory).unwrap());
            let evaluator = EvalContext::new(policy);
            let packet =
                PacketDesc::new_tcp("fd5a:5052:3000::1", "fd5a:5052:3000::2", 12345, 80).unwrap();
            for (subject, expected_grant) in [("alice", true), ("bob", false)] {
                let mut actor = Actor::new();
                for (_, result) in manager
                    .get_attributes_for_actor(&[("user.sub".into(), subject.into())])
                    .await
                {
                    for attr in result.unwrap() {
                        actor.add_attribute(attr).unwrap();
                    }
                }
                actor
                    .add_attribute(Attribute::builder(key::SERVICES).value("database.svc.zpr"))
                    .unwrap();
                actor
                    .add_attribute(
                        Attribute::builder("user.zpr.authority").value("test-authenticated"),
                    )
                    .unwrap();
                assert!(actor.get_attribute("user.department").is_none());
                assert!(
                    actor
                        .get_attribute("user.department__source__corp-1_a")
                        .is_some()
                );
                assert!(
                    actor
                        .get_attribute("user.department__source__lab-2_b")
                        .is_some()
                );
                let result = evaluator.eval_request(&actor, &actor, &packet).unwrap();
                assert_eq!(
                    matches!(result, PartialEvalResult::AllowWithoutRoute(_)),
                    expected_grant,
                    "{subject}: {result:?}"
                );
            }
        }
        fs::remove_dir_all(directory).unwrap();
    }
}
