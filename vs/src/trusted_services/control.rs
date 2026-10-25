//! Provisional durable operational overrides for policy-declared trusted sources.
use std::sync::Arc;

use serde::{Deserialize, Serialize};
use tracing::error;

use super::factory::valid_source_id;
use crate::assembly::Assembly;
use crate::db::DbConnection;
use crate::error::{ServiceError, StoreError};
use crate::logging::targets::EVENT;
use crate::trusted_services::TrustedServicesMgr;

const KEY: &str = "trusted-source-controls";

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub(crate) struct Control {
    pub enabled: bool,
    pub revision: String,
    pub pending: bool,
}

fn decode(source: &str, value: &str) -> Result<Control, ServiceError> {
    if !valid_source_id(source) {
        return Err(StoreError::InvalidData("invalid stored trusted source ID".into()).into());
    }
    let control: Control = serde_json::from_str(value)?;
    if uuid::Uuid::parse_str(&control.revision).is_err() {
        return Err(StoreError::InvalidData(
            "invalid stored trusted source control revision".into(),
        )
        .into());
    }
    Ok(control)
}

// Load before policy construction/admission. Missing overrides are default-on;
// malformed/unavailable persisted state must stop startup, never imply enabled.
pub(crate) async fn restore(
    db: &dyn DbConnection,
    manager: &TrustedServicesMgr,
) -> Result<(), ServiceError> {
    let values: std::collections::HashMap<String, String> =
        db.hgetall(KEY.into()).await.map_err(StoreError::from)?;
    let controls = values
        .iter()
        .map(|(source, value)| decode(source, value).map(|control| (source, control)))
        .collect::<Result<Vec<_>, _>>()?;
    for (source, control) in controls {
        manager.set_source_enabled(source, control.enabled);
    }
    Ok(())
}

pub(crate) async fn read(db: &dyn DbConnection, source: &str) -> Result<Control, ServiceError> {
    if !valid_source_id(source) {
        return Err(ServiceError::Param("invalid trusted source ID".into()));
    }
    match db.hget(KEY, source).await.map_err(StoreError::from)? {
        Some(value) => decode(source, &value),
        None => Ok(Control {
            enabled: true,
            revision: String::new(),
            pending: false,
        }),
    }
}

async fn apply(
    manager: &TrustedServicesMgr,
    source: &str,
    enabled: bool,
) -> Result<(), ServiceError> {
    // Gate first, including during re-enable: old cached permissions must not
    // become usable if the fresh reload fails.
    manager.set_source_enabled(source, false);
    if enabled && manager.declares_source(source) {
        manager.flush_one(source).await?;
    }
    manager.set_source_enabled(source, enabled);
    Ok(())
}

pub(crate) async fn set(
    db: &dyn DbConnection,
    manager: &TrustedServicesMgr,
    source: &str,
    enabled: bool,
) -> Result<Control, ServiceError> {
    if !valid_source_id(source) {
        return Err(ServiceError::Param("invalid trusted source ID".into()));
    }
    let _guard = manager.control_lock.lock().await;
    let control = Control {
        enabled,
        revision: uuid::Uuid::new_v4().to_string(),
        pending: true,
    };
    // Desired state and pending work occupy the same atomic hash field.
    // Disable locally before I/O: persistence uncertainty must not leave access on.
    manager.set_source_enabled(source, false);
    db.hset(KEY, source, &serde_json::to_string(&control)?)
        .await
        .map_err(StoreError::from)?;
    apply(manager, source, enabled).await?;
    Ok(control)
}

pub(crate) async fn process_pending(asm: &Arc<Assembly>) {
    if let Err(error) = process(asm).await {
        error!(target: EVENT, "trusted source control reconciliation failed: {}", error);
    }
}

async fn process(asm: &Arc<Assembly>) -> Result<(), ServiceError> {
    let _guard = asm.ts_mgr.control_lock.lock().await;
    let values: std::collections::HashMap<String, String> = asm
        .state_db
        .hgetall(KEY.into())
        .await
        .map_err(StoreError::from)?;
    let mut first_error = None;
    for (source, value) in values {
        if let Err(error) = reconcile(asm, &source, &value).await {
            error!(target: EVENT, "trusted source {} control retry failed: {}", source, error);
            first_error.get_or_insert(error);
        }
    }
    first_error.map_or(Ok(()), Err)
}

async fn reconcile(asm: &Arc<Assembly>, source: &str, value: &str) -> Result<(), ServiceError> {
    let mut control = decode(source, value)?;
    if !control.pending {
        return Ok(());
    }
    apply(&asm.ts_mgr, source, control.enabled).await?;
    // Whole-actor failure closure is intentionally conservative. Disabled
    // sources may leave reconciliation pending after visas have been denied.
    if !crate::event_mgr::handle_trusted_service_change(asm).await {
        return Ok(());
    }
    control.pending = false;
    // This worker and API writes share control_lock. The VS owns its database
    // under the existing instance lease, so completion cannot erase a newer
    // in-process request.
    asm.state_db
        .hset(KEY, source, &serde_json::to_string(&control)?)
        .await
        .map_err(StoreError::from)?;
    Ok(())
}

#[cfg(test)]
pub(crate) async fn assert_persistence_contract(db: &dyn DbConnection) {
    let manager = TrustedServicesMgr::new();
    assert!(read(db, "directory").await.unwrap().enabled);
    let accepted = set(db, &manager, "directory", false).await.unwrap();
    assert!(!manager.source_enabled("directory"));
    let restarted = TrustedServicesMgr::new();
    restore(db, &restarted).await.unwrap();
    assert!(!restarted.source_enabled("directory"));
    let persisted = read(db, "directory").await.unwrap();
    assert_eq!(persisted.revision, accepted.revision);
    assert!(!persisted.enabled && persisted.pending);
    assert!(read(db, "different").await.unwrap().enabled);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::FakeDb;
    use crate::trusted_services::TrustedServiceInterface;
    use crate::trusted_services::file_attribute_store::FileAttributeStore;
    use crate::trusted_services::test_support::test_mapper;
    use std::time::Duration;

    fn store(directory: &std::path::Path, name: &str, color: &str) -> Arc<FileAttributeStore> {
        let path = directory.join(format!("{name}.json"));
        std::fs::write(
            &path,
            format!(r#"{{"user.sub":{{"alice":{{"color":["{color}"]}}}}}}"#),
        )
        .unwrap();
        Arc::new(
            FileAttributeStore::new(name.into(), test_mapper(), Duration::from_secs(600), &path)
                .unwrap(),
        )
    }

    fn identities() -> [(String, String); 1] {
        [("user.sub".into(), "alice".into())]
    }

    #[tokio::test]
    async fn fake_database_persistence_contract() {
        assert_persistence_contract(&FakeDb::new()).await;
    }

    #[tokio::test]
    async fn actual_policy_install_replacement_and_restore_preserve_disabled_override() {
        use crate::db::PolicyRepo;
        use crate::policy_mgr::PolicyMgr;
        use crate::test_helpers::{FakeResolver, make_trusted_service_policy};
        let directory = tempfile::tempdir().unwrap();
        store(directory.path(), "directory", "red");
        store(directory.path(), "lab", "blue");
        let db = Arc::new(FakeDb::new());
        let manager = Arc::new(TrustedServicesMgr::new());
        let original =
            make_trusted_service_policy("directory", "file", Some(600), &["color -> user.color"]);
        let policy = PolicyMgr::new_with_initial_policy(
            original.clone(),
            PolicyRepo::new(db.clone()),
            Arc::new(FakeResolver::ip_only()),
            manager.clone(),
            directory.path().into(),
        )
        .await
        .unwrap();
        assert!(manager.statuses()[0].enabled);
        set(db.as_ref(), &manager, "directory", false)
            .await
            .unwrap();
        for candidate in [
            original.clone(),
            make_trusted_service_policy("directory", "file", Some(900), &["color -> user.color"]),
            make_trusted_service_policy("lab", "file", Some(600), &["color -> user.color"]),
            original,
        ] {
            policy
                .update_policy_from_container_bytes(candidate)
                .await
                .unwrap();
            assert!(!manager.source_enabled("directory"));
        }
        let restarted = Arc::new(TrustedServicesMgr::new());
        restore(db.as_ref(), &restarted).await.unwrap();
        let _restored_policy = PolicyMgr::new_from_state(
            PolicyRepo::new(db),
            Arc::new(FakeResolver::ip_only()),
            restarted.clone(),
            directory.path().into(),
        )
        .await
        .unwrap();
        assert_eq!(restarted.statuses()[0].name, "directory");
        assert!(!restarted.statuses()[0].enabled);
        assert!(
            restarted
                .get_attributes_from_source_for_actor("directory", &identities())
                .await[0]
                .is_err()
        );
    }

    #[tokio::test]
    async fn broken_enable_retry_does_not_starve_other_sources_disable_reconciliation() {
        let asm = Arc::new(crate::assembly::tests::new_assembly_for_tests(None).await);
        let directory = tempfile::tempdir().unwrap();
        asm.ts_mgr.update_services(vec![
            store(directory.path(), "broken", "red"),
            store(directory.path(), "healthy", "blue"),
        ]);
        set(asm.state_db.as_ref(), &asm.ts_mgr, "healthy", false)
            .await
            .unwrap();
        std::fs::write(directory.path().join("broken.json"), "broken").unwrap();
        assert!(
            set(asm.state_db.as_ref(), &asm.ts_mgr, "broken", true)
                .await
                .is_err()
        );
        assert!(process(&asm).await.is_err());
        assert!(read(asm.state_db.as_ref(), "broken").await.unwrap().pending);
        assert!(
            !read(asm.state_db.as_ref(), "healthy")
                .await
                .unwrap()
                .pending
        );
        assert!(!asm.ts_mgr.source_enabled("healthy"));
        assert!(!asm.ts_mgr.source_enabled("broken"));
    }

    #[tokio::test]
    async fn default_on_disabled_blocks_both_queries_and_pins_configuration() {
        let db = FakeDb::new();
        let directory = tempfile::tempdir().unwrap();
        let manager = TrustedServicesMgr::new();
        let source = store(directory.path(), "directory", "red");
        manager.update_services(vec![source.clone()]);
        restore(&db, &manager).await.unwrap();
        assert!(read(&db, "directory").await.unwrap().enabled);
        assert!(
            manager.get_attributes_for_actor(&identities()).await[0]
                .1
                .is_ok()
        );
        let configuration = manager.configuration();
        let actor = "fd5a:5052::123".parse().unwrap();
        manager.record_revision(&actor, "directory", source.current_revision());
        assert!(manager.stale_sources_for_actor(&actor).is_empty());
        let accepted = set(&db, &manager, "directory", false).await.unwrap();
        assert!(!accepted.enabled && accepted.pending);
        assert!(!manager.configuration_is_current(&configuration));
        assert!(!manager.stale_sources_for_actor(&actor).is_empty());
        assert_eq!(manager.statuses()[0].health, "disabled");
        assert!(!manager.statuses()[0].enabled);
        assert!(matches!(
            &manager.get_attributes_for_actor(&identities()).await[0].1,
            Err(ServiceError::AttributesIndeterminate(_))
        ));
        assert!(matches!(
            &manager
                .get_attributes_from_source_for_actor("directory", &identities())
                .await[0],
            Err(ServiceError::AttributesIndeterminate(_))
        ));
    }

    #[tokio::test]
    async fn disable_survives_policy_republish_removal_readdition_and_restart() {
        let db = FakeDb::new();
        let directory = tempfile::tempdir().unwrap();
        let manager = TrustedServicesMgr::new();
        let source = store(directory.path(), "directory", "red");
        manager.update_services(vec![source.clone()]);
        set(&db, &manager, "directory", false).await.unwrap();
        manager.update_services(vec![source.clone()]);
        assert!(!manager.source_enabled("directory"));
        manager.update_services(vec![]);
        assert!(manager.statuses().is_empty());
        assert!(!read(&db, "directory").await.unwrap().enabled);
        manager.update_services(vec![source.clone()]);
        assert!(!manager.statuses()[0].enabled);
        let restarted = TrustedServicesMgr::new();
        restore(&db, &restarted).await.unwrap();
        restarted.update_services(vec![source]);
        assert!(!restarted.statuses()[0].enabled);
        assert!(
            restarted
                .get_attributes_from_source_for_actor("directory", &identities())
                .await[0]
                .is_err()
        );
    }

    #[tokio::test]
    async fn reenable_requires_reload_and_never_resurrects_old_cached_claims() {
        let db = FakeDb::new();
        let directory = tempfile::tempdir().unwrap();
        let manager = TrustedServicesMgr::new();
        let source = store(directory.path(), "directory", "red");
        manager.update_services(vec![source.clone()]);
        set(&db, &manager, "directory", false).await.unwrap();
        let revision = source.current_revision();
        std::fs::write(directory.path().join("directory.json"), "broken").unwrap();
        assert!(set(&db, &manager, "directory", true).await.is_err());
        assert!(!manager.source_enabled("directory"));
        assert_eq!(source.current_revision(), revision);
        assert!(read(&db, "directory").await.unwrap().pending);
        std::fs::write(
            directory.path().join("directory.json"),
            r#"{"user.sub":{"alice":{"color":["green"]}}}"#,
        )
        .unwrap();
        set(&db, &manager, "directory", true).await.unwrap();
        assert_ne!(source.current_revision(), revision);
        assert!(manager.source_enabled("directory"));
        let results = manager
            .get_attributes_from_source_for_actor("directory", &identities())
            .await;
        let attrs = results[0].as_ref().unwrap();
        assert!(attrs.iter().any(|attr| attr.value_has("green")));
        assert!(!attrs.iter().any(|attr| attr.value_has("red")));
    }

    #[tokio::test]
    async fn disabled_source_does_not_collapse_multi_source_namespace() {
        let db = FakeDb::new();
        let directory = tempfile::tempdir().unwrap();
        let manager = TrustedServicesMgr::new();
        manager.update_services(vec![
            store(directory.path(), "directory", "red"),
            store(directory.path(), "lab", "blue"),
        ]);
        set(&db, &manager, "directory", false).await.unwrap();
        for (source, result) in manager.get_attributes_for_actor(&identities()).await {
            if source == "directory" {
                assert!(result.is_err());
            } else {
                let attrs = result.unwrap();
                assert!(
                    attrs
                        .iter()
                        .all(|attr| attr.get_key().contains("__source__lab"))
                );
            }
        }
    }

    #[tokio::test]
    async fn malformed_controls_and_database_errors_never_restore_default_on() {
        for value in [
            "{}",
            r#"{"enabled":false,"revision":"bad","pending":true}"#,
            r#"{"enabled":false,"revision":"00000000-0000-0000-0000-000000000001","pending":true,"extra":1}"#,
        ] {
            let db = FakeDb::new();
            db.hset(KEY, "directory", value).await.unwrap();
            assert!(restore(&db, &TrustedServicesMgr::new()).await.is_err());
        }
        let db = FakeDb::new();
        db.set(KEY, "wrong type").await.unwrap();
        let manager = TrustedServicesMgr::new();
        assert!(restore(&db, &manager).await.is_err());
        assert!(set(&db, &manager, "directory", false).await.is_err());
        assert!(!manager.source_enabled("directory"));
        assert!(read(&db, "directory").await.is_err());
    }

    #[tokio::test]
    async fn durable_pending_record_is_consumed_even_without_wakeup_or_provider() {
        let asm = Arc::new(crate::assembly::tests::new_assembly_for_tests(None).await);
        let accepted = set(asm.state_db.as_ref(), &asm.ts_mgr, "directory", false)
            .await
            .unwrap();
        let restarted = TrustedServicesMgr::new();
        restore(asm.state_db.as_ref(), &restarted).await.unwrap();
        assert!(!restarted.source_enabled("directory"));
        process(&asm).await.unwrap();
        let completed = read(asm.state_db.as_ref(), "directory").await.unwrap();
        assert_eq!(completed.revision, accepted.revision);
        assert!(!completed.pending && !completed.enabled);
        process(&asm).await.unwrap();
    }

    fn packet() -> zpr::vsapi_types::PacketDesc {
        zpr::vsapi_types::PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80)
            .unwrap()
    }

    #[tokio::test]
    async fn disabled_source_denies_cached_request_and_queues_existing_visa_revocation() {
        use crate::visareq_worker::{VisaDecision, process_visa_request_for_test};
        let (asm, node, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
        let VisaDecision::Allow(visa, _) =
            process_visa_request_for_test(asm.clone(), node, packet()).await
        else {
            panic!("baseline visa must allow");
        };
        set(asm.state_db.as_ref(), &asm.ts_mgr, "hr", false)
            .await
            .unwrap();
        assert!(matches!(
            process_visa_request_for_test(asm.clone(), node, packet()).await,
            VisaDecision::Deny(_)
        ));
        process(&asm).await.unwrap();
        assert_eq!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap(),
            [visa.issuer_id]
        );
        // A queued revoke is not proof that the gateway has removed its visa.
        assert!(read(asm.state_db.as_ref(), "hr").await.unwrap().pending);
        set(asm.state_db.as_ref(), &asm.ts_mgr, "hr", true)
            .await
            .unwrap();
        process(&asm).await.unwrap();
        assert!(!read(asm.state_db.as_ref(), "hr").await.unwrap().pending);
        assert_eq!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap(),
            [visa.issuer_id]
        );
    }

    struct GatedSource {
        entered: tokio::sync::Notify,
        release: tokio::sync::Notify,
    }

    struct GatedReload {
        entered: tokio::sync::Notify,
        release: tokio::sync::Notify,
    }

    #[async_trait::async_trait]
    impl TrustedServiceInterface for GatedReload {
        async fn get_attributes_for_actor(
            &self,
            _: &[(String, String)],
        ) -> Result<Vec<libeval::attribute::Attribute>, ServiceError> {
            Ok(vec![])
        }
        async fn flush(&self) -> Result<(), ServiceError> {
            self.entered.notify_one();
            self.release.notified().await;
            Ok(())
        }
        fn current_revision(&self) -> u64 {
            5
        }
        fn get_source_id(&self) -> &str {
            "directory"
        }
    }

    #[tokio::test]
    async fn interrupted_enable_retains_atomic_pending_work_and_recovers_without_request_retry() {
        tokio::time::timeout(Duration::from_secs(5), async {
            let asm = Arc::new(crate::assembly::tests::new_assembly_for_tests(None).await);
            let source = Arc::new(GatedReload {
                entered: tokio::sync::Notify::new(),
                release: tokio::sync::Notify::new(),
            });
            asm.ts_mgr.update_services(vec![source.clone()]);
            set(asm.state_db.as_ref(), &asm.ts_mgr, "directory", false)
                .await
                .unwrap();
            let task_asm = asm.clone();
            let request = tokio::spawn(async move {
                set(
                    task_asm.state_db.as_ref(),
                    &task_asm.ts_mgr,
                    "directory",
                    true,
                )
                .await
            });
            source.entered.notified().await;
            request.abort();
            assert!(request.await.unwrap_err().is_cancelled());
            let desired = read(asm.state_db.as_ref(), "directory").await.unwrap();
            assert!(desired.enabled && desired.pending);
            assert!(!asm.ts_mgr.source_enabled("directory"));
            source.release.notify_one();
            process(&asm).await.unwrap();
            let completed = read(asm.state_db.as_ref(), "directory").await.unwrap();
            assert_eq!(completed.revision, desired.revision);
            assert!(completed.enabled && !completed.pending);
            assert!(asm.ts_mgr.source_enabled("directory"));
        })
        .await
        .expect("interrupted enable recovery deadlocked");
    }

    #[async_trait::async_trait]
    impl TrustedServiceInterface for GatedSource {
        async fn get_attributes_for_actor(
            &self,
            _: &[(String, String)],
        ) -> Result<Vec<libeval::attribute::Attribute>, ServiceError> {
            self.entered.notify_one();
            self.release.notified().await;
            Ok(vec![])
        }
        async fn flush(&self) -> Result<(), ServiceError> {
            Ok(())
        }
        fn current_revision(&self) -> u64 {
            5
        }
        fn get_source_id(&self) -> &str {
            "hr"
        }
    }

    #[tokio::test]
    async fn in_flight_provider_reply_cannot_publish_permission_after_disable() {
        tokio::time::timeout(Duration::from_secs(5), async {
            use crate::visareq_worker::{VisaDecision, process_visa_request_for_test};
            let (asm, node, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
            let source = Arc::new(GatedSource {
                entered: tokio::sync::Notify::new(),
                release: tokio::sync::Notify::new(),
            });
            asm.ts_mgr.update_services(vec![source.clone()]);
            let task_asm = asm.clone();
            let request = tokio::spawn(async move {
                process_visa_request_for_test(task_asm, node, packet()).await
            });
            source.entered.notified().await;
            set(asm.state_db.as_ref(), &asm.ts_mgr, "hr", false)
                .await
                .unwrap();
            source.release.notify_one();
            assert!(matches!(request.await.unwrap(), VisaDecision::Deny(_)));
            assert!(asm.visa_mgr.list_visa_metadata().await.is_empty());
        })
        .await
        .expect("in-flight toggle test deadlocked");
    }
}
