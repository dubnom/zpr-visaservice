use super::*;
use crate::trusted_services::file_attribute_store::FileAttributeStore;
use crate::trusted_services::test_support::test_mapper;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::time::Duration;
use tokio::sync::Notify;

struct ControlledService {
    name: &'static str,
    failed: AtomicBool,
    revision: AtomicU64,
    entered: Notify,
    release: Notify,
}

impl ControlledService {
    fn new(name: &'static str) -> Arc<Self> {
        Arc::new(Self {
            name,
            failed: AtomicBool::new(false),
            revision: AtomicU64::new(1),
            entered: Notify::new(),
            release: Notify::new(),
        })
    }
}

#[async_trait::async_trait]
impl TrustedServiceInterface for ControlledService {
    async fn get_attributes_for_actor(
        &self,
        _: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        self.entered.notify_one();
        self.release.notified().await;
        if self.failed.load(Ordering::Relaxed) {
            Err(ServiceError::AttributesIndeterminate(format!(
                "{}: unavailable",
                self.name
            )))
        } else {
            Ok(vec![])
        }
    }
    async fn flush(&self) -> Result<(), ServiceError> {
        self.revision.fetch_add(1, Ordering::Relaxed);
        Ok(())
    }
    fn current_revision(&self) -> u64 {
        self.revision.load(Ordering::Relaxed)
    }
    fn get_source_id(&self) -> &str {
        self.name
    }
}

async fn replaced_lookup(targeted: bool, replacement_present: bool, old_failed: bool) {
    tokio::time::timeout(Duration::from_secs(3), async {
        let manager = Arc::new(TrustedServicesMgr::new());
        let old = ControlledService::new("directory");
        old.failed.store(old_failed, Ordering::Relaxed);
        manager.update_services(vec![old.clone()]);
        let task_manager = manager.clone();
        let pending = tokio::spawn(async move {
            if targeted {
                task_manager
                    .get_attributes_from_source_for_actor("directory", &[])
                    .await
                    .into_iter()
                    .next()
                    .unwrap()
            } else {
                task_manager
                    .get_attributes_for_actor(&[])
                    .await
                    .into_iter()
                    .next()
                    .unwrap()
                    .1
            }
        });
        old.entered.notified().await;
        let replacement = ControlledService::new("directory");
        replacement.failed.store(!old_failed, Ordering::Relaxed);
        if replacement_present {
            manager.update_services(vec![replacement.clone()]);
            let unverified = manager.statuses();
            assert_eq!(unverified[0].health, "unverified");
            assert!(unverified[0].last_lookup_ms.is_none());
            assert!(unverified[0].last_success_ms.is_none());
            replacement.release.notify_one();
            let result = manager
                .get_attributes_from_source_for_actor("directory", &[])
                .await;
            assert_eq!(result[0].is_ok(), old_failed);
        } else {
            manager.update_services(vec![]);
        }
        let before = serde_json::to_value(manager.statuses()).unwrap();
        old.release.notify_one();
        assert_eq!(pending.await.unwrap().is_err(), old_failed);
        assert_eq!(
            serde_json::to_value(manager.statuses()).unwrap(),
            before,
            "retired lookup changed current service health"
        );
        if !replacement_present {
            manager.update_services(vec![replacement]);
            assert_eq!(manager.statuses()[0].health, "unverified");
        }
    })
    .await
    .expect("controlled lookup deadlocked");
}

#[tokio::test]
async fn retired_full_success_cannot_overwrite_replacement_failure() {
    replaced_lookup(false, true, false).await;
}

#[tokio::test]
async fn retired_full_failure_cannot_overwrite_replacement_success() {
    replaced_lookup(false, true, true).await;
}

#[tokio::test]
async fn retired_targeted_lookup_cannot_change_replacement_health() {
    for failed in [false, true] {
        replaced_lookup(true, true, failed).await;
    }
}

#[tokio::test]
async fn retired_lookup_cannot_repopulate_removed_source_health() {
    for targeted in [false, true] {
        replaced_lookup(targeted, false, false).await;
    }
}

fn file_store(dir: &std::path::Path, name: &str, color: &str) -> Arc<FileAttributeStore> {
    let path = dir.join(format!("{name}.json"));
    std::fs::write(&path, format!(
        r#"{{"user.sub":{{"alice":{{"color":["{color}"],"roles":["reviewer","author"],"lazy":[]}}}}}}"#
    )).unwrap();
    Arc::new(
        FileAttributeStore::new(name.into(), test_mapper(), Duration::from_secs(600), &path)
            .unwrap(),
    )
}

fn identities() -> [(String, String); 1] {
    [("user.sub".into(), "alice".into())]
}

#[tokio::test]
async fn targeted_flush_only_invalidates_and_reloads_selected_source() {
    let dir = tempfile::tempdir().unwrap();
    let company = file_store(dir.path(), "company", "red");
    let lab = file_store(dir.path(), "lab", "blue");
    let manager = TrustedServicesMgr::new();
    manager.update_services(vec![company.clone(), lab.clone()]);
    let address = "fd5a:5052::123".parse().unwrap();
    manager.record_revision(&address, "company", company.current_revision());
    manager.record_revision(&address, "lab", lab.current_revision());
    let lab_revision = lab.current_revision();
    std::fs::write(
        dir.path().join("company.json"),
        r#"{"user.sub":{"alice":{"color":["green"]}}}"#,
    )
    .unwrap();
    manager.flush_one("company").await.unwrap();
    assert_eq!(
        manager.stale_sources_for_actor(&address),
        vec![("company".into(), company.current_revision())]
    );
    assert_eq!(lab.current_revision(), lab_revision);
    for (name, result) in manager.get_attributes_for_actor(&identities()).await {
        let attrs = result.unwrap();
        assert!(attrs.iter().any(
            |attr| attr.get_key() == format!("user.color__source__{name}")
                && attr.value_has(if name == "company" { "green" } else { "blue" })
        ));
    }
}

#[tokio::test]
async fn flush_all_continues_after_failure_and_preserves_failed_snapshot() {
    let dir = tempfile::tempdir().unwrap();
    let company = file_store(dir.path(), "company", "red");
    let lab = file_store(dir.path(), "lab", "blue");
    let manager = TrustedServicesMgr::new();
    manager.update_services(vec![company.clone(), lab.clone()]);
    let before = [company.current_revision(), lab.current_revision()];
    std::fs::write(dir.path().join("company.json"), "broken").unwrap();
    std::fs::write(
        dir.path().join("lab.json"),
        r#"{"user.sub":{"alice":{"color":["green"]}}}"#,
    )
    .unwrap();
    let results = manager.flush_all().await;
    assert_eq!(results.len(), 2);
    assert!(results[0].is_err());
    assert!(results[1].is_ok());
    assert_eq!(company.current_revision(), before[0]);
    assert!(lab.current_revision() > before[1]);
    let attrs = manager.get_attributes_for_actor(&identities()).await;
    assert!(
        attrs[0]
            .1
            .as_ref()
            .unwrap()
            .iter()
            .any(|attr| attr.value_has("red"))
    );
    assert!(
        attrs[1]
            .1
            .as_ref()
            .unwrap()
            .iter()
            .any(|attr| attr.value_has("green"))
    );
}

#[tokio::test]
async fn unknown_source_operations_do_not_change_health_or_revisions() {
    let dir = tempfile::tempdir().unwrap();
    let company = file_store(dir.path(), "company", "red");
    let manager = TrustedServicesMgr::new();
    manager.update_services(vec![company.clone()]);
    let revision = company.current_revision();
    let status = serde_json::to_value(manager.statuses()).unwrap();
    assert!(matches!(
        manager.flush_one("missing").await,
        Err(ServiceError::TrustedServiceNotFound(_))
    ));
    let results = manager
        .get_attributes_from_source_for_actor("missing", &identities())
        .await;
    assert_eq!(results.len(), 1);
    assert!(matches!(
        &results[0],
        Err(ServiceError::TrustedServiceNotFound(_))
    ));
    assert_eq!(company.current_revision(), revision);
    assert_eq!(serde_json::to_value(manager.statuses()).unwrap(), status);
}

#[tokio::test]
async fn replacement_revisions_are_stale_and_address_reuse_forgets_all_sources() {
    let dir = tempfile::tempdir().unwrap();
    let company = file_store(dir.path(), "company", "red");
    let lab = file_store(dir.path(), "lab", "blue");
    let manager = TrustedServicesMgr::new();
    manager.update_services(vec![company.clone(), lab.clone()]);
    let address = "fd5a:5052::123".parse().unwrap();
    manager.record_revision(&address, "company", company.current_revision());
    manager.record_revision(&address, "lab", lab.current_revision());
    let replacement = file_store(dir.path(), "company", "green");
    manager.update_services(vec![replacement.clone(), lab.clone()]);
    assert_eq!(
        manager.stale_sources_for_actor(&address),
        vec![("company".into(), replacement.current_revision())]
    );
    manager.record_revision(&address, "company", replacement.current_revision());
    assert!(manager.stale_sources_for_actor(&address).is_empty());
    manager.forget_actor_revisions(&address);
    assert_eq!(manager.stale_sources_for_actor(&address).len(), 2);
}

#[tokio::test]
async fn alias_transitions_preserve_source_values_tags_and_deadlines() {
    let dir = tempfile::tempdir().unwrap();
    let company = file_store(dir.path(), "company", "red");
    let lab = file_store(dir.path(), "lab", "blue");
    let manager = TrustedServicesMgr::new();
    for multiple in [false, true, false] {
        if multiple {
            manager.update_services(vec![company.clone(), lab.clone()]);
        } else {
            manager.update_services(vec![company.clone()]);
        }
        for targeted in [false, true] {
            let attrs = if targeted {
                manager
                    .get_attributes_from_source_for_actor("company", &identities())
                    .await
                    .remove(0)
                    .unwrap()
            } else {
                manager
                    .get_attributes_for_actor(&identities())
                    .await
                    .remove(0)
                    .1
                    .unwrap()
            };
            assert_eq!(attrs.len(), if multiple { 3 } else { 6 });
            for key in ["user.color", "user.role", "user.zpr.tag.lazy"] {
                let qualified = attrs
                    .iter()
                    .find(|attr| attr.get_key() == format!("{key}__source__company"))
                    .unwrap();
                assert_eq!(qualified.get_source(), "company");
                let legacy = attrs.iter().find(|attr| attr.get_key() == key);
                assert_eq!(legacy.is_none(), multiple);
                if let Some(legacy) = legacy {
                    assert_eq!(legacy.get_value(), qualified.get_value());
                    assert_eq!(legacy.get_expires(), qualified.get_expires());
                }
                if key == "user.role" {
                    assert_eq!(qualified.get_value(), &["reviewer", "author"]);
                }
                if key == "user.zpr.tag.lazy" {
                    assert!(qualified.get_value().is_empty());
                }
            }
        }
    }
}

#[tokio::test]
async fn independent_source_health_distinguishes_empty_success_failure_and_recovery() {
    let manager = TrustedServicesMgr::new();
    let company = ControlledService::new("company");
    let lab = ControlledService::new("lab");
    manager.update_services(vec![lab.clone(), company.clone()]);
    let mut last_company_success = None;
    for failed in [false, true, false] {
        company.failed.store(failed, Ordering::Relaxed);
        company.release.notify_one();
        lab.release.notify_one();
        let results = manager.get_attributes_for_actor(&[]).await;
        assert!(results[0].1.as_ref().unwrap().is_empty());
        assert_eq!(results[1].1.is_err(), failed);
        let statuses = manager.statuses();
        assert_eq!(statuses[0].name, "company");
        assert_eq!(
            statuses[0].health,
            if failed { "failed" } else { "working" }
        );
        assert_eq!(statuses[1].health, "working");
        assert!(
            statuses
                .iter()
                .all(|status| status.last_success_ms.is_some())
        );
        if failed {
            assert_eq!(statuses[0].last_success_ms, last_company_success);
        } else {
            last_company_success = statuses[0].last_success_ms;
        }
    }
}
