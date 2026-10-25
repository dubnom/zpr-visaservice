use super::*;
use crate::db::{FakeDb, PolicyRepo};
use crate::test_helpers::{
    FakeResolver, make_trusted_service_policy, make_trusted_service_policy_with_identity,
};
use libeval::attribute::Attribute;
use tempfile::TempDir;

struct Fixture {
    dir: TempDir,
    db: Arc<FakeDb>,
    sources: Arc<TrustedServicesMgr>,
    policy: PolicyMgr,
    original: Vec<u8>,
}

fn candidate(api: &str, ttl: Option<u32>, mappings: &[&str]) -> Vec<u8> {
    make_trusted_service_policy("directory", api, ttl, mappings)
}

fn address() -> IpAddr {
    "fd5a:5052::123".parse().unwrap()
}

async fn fixture() -> Fixture {
    let dir = tempfile::tempdir().unwrap();
    std::fs::write(
        dir.path().join("directory.json"),
        r#"{"user.sub":{"alice":{"department":["engineering"],"badge":["cleared"]}}}"#,
    )
    .unwrap();
    let db = Arc::new(FakeDb::new());
    let sources = Arc::new(TrustedServicesMgr::new());
    let original = candidate("file", Some(600), &["department -> user.department"]);
    let policy = PolicyMgr::new_with_initial_policy(
        original.clone(),
        PolicyRepo::new(db.clone()),
        Arc::new(FakeResolver::ip_only()),
        sources.clone(),
        dir.path().into(),
    )
    .await
    .unwrap();
    let revisions = sources.stale_sources_for_actor(&address());
    assert_eq!(revisions.len(), 1);
    sources.record_revision(&address(), "directory", revisions[0].1);
    Fixture {
        dir,
        db,
        sources,
        policy,
        original,
    }
}

async fn lookup(sources: &TrustedServicesMgr) -> Vec<Attribute> {
    let results = sources
        .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
        .await;
    assert_eq!(results.len(), 1);
    let (name, result) = results.into_iter().next().unwrap();
    assert_eq!(name, "directory");
    result.unwrap()
}

async fn assert_persisted(f: &Fixture, bytes: &[u8], vinst: u64) {
    assert_eq!(f.policy.get_current_container().as_bytes(), bytes);
    assert_eq!(f.policy.get_current_snapshot().vinst(), vinst);
    let repo = PolicyRepo::new(f.db.clone());
    let stored = repo
        .get_current_loaded_policy(&config::POLICY_MIN_VERSION)
        .await
        .unwrap();
    assert_eq!(stored.container().as_bytes(), bytes);
    assert_eq!(
        repo.get_current_identifier().await.unwrap().unwrap().vinst,
        vinst
    );
}

async fn assert_rejected(f: &Fixture, bytes: Vec<u8>, detail: &str) {
    let held = f.policy.get_current_snapshot();
    let before = lookup(&f.sources).await;
    let status = serde_json::to_value(f.sources.statuses()).unwrap();
    let error = f
        .policy
        .update_policy_from_container_bytes(bytes)
        .await
        .unwrap_err();
    assert!(error.to_string().contains(detail), "{error}");
    assert_persisted(f, &f.original, 1).await;
    assert_eq!(held.vinst(), 1);
    assert_eq!(held.container().as_bytes(), f.original.as_slice());
    assert!(f.sources.stale_sources_for_actor(&address()).is_empty());
    assert_eq!(serde_json::to_value(f.sources.statuses()).unwrap(), status);
    let after = lookup(&f.sources).await;
    assert_eq!(before.len(), after.len());
    for attr in before {
        let current = after
            .iter()
            .find(|next| next.get_key() == attr.get_key())
            .unwrap();
        assert_eq!(current.get_source(), attr.get_source());
        assert_eq!(current.get_value(), attr.get_value());
    }
}

#[tokio::test]
async fn unsupported_api_update_is_nonpublishing() {
    let f = fixture().await;
    assert_rejected(&f, candidate("ldap", Some(600), &[]), "unsupported").await;
}

#[tokio::test]
async fn missing_record_update_is_nonpublishing() {
    let f = fixture().await;
    assert_rejected(
        &f,
        candidate("file", None, &[]),
        "no trusted service record",
    )
    .await;
}

#[tokio::test]
async fn invalid_ttl_update_is_nonpublishing() {
    let f = fixture().await;
    for ttl in [0, 1, 60] {
        assert_rejected(&f, candidate("file", Some(ttl), &[]), "ttl").await;
    }
}

#[tokio::test]
async fn invalid_source_name_update_is_nonpublishing() {
    let f = fixture().await;
    for name in [
        "../outside".to_string(),
        "a".repeat(65),
        "bad__source__id".into(),
    ] {
        assert_rejected(
            &f,
            make_trusted_service_policy(&name, "file", Some(600), &[]),
            "id must",
        )
        .await;
    }
}

#[tokio::test]
async fn reserved_target_update_is_nonpublishing() {
    let f = fixture().await;
    assert_rejected(
        &f,
        candidate(
            "file",
            Some(600),
            &["department -> user.department__source__other"],
        ),
        "reserved",
    )
    .await;
}

#[tokio::test]
async fn conflicting_mapping_update_is_nonpublishing() {
    let f = fixture().await;
    for mappings in [
        vec![
            "department -> user.department",
            "department -> device.department",
        ],
        vec![
            "department -> user.department",
            "badge -> user.department{}",
        ],
    ] {
        assert_rejected(&f, candidate("file", Some(600), &mappings), "conflicting").await;
    }
}

#[tokio::test]
async fn missing_new_source_file_update_is_nonpublishing() {
    let f = fixture().await;
    assert_rejected(
        &f,
        make_trusted_service_policy("missing", "file", Some(600), &[]),
        "missing.json",
    )
    .await;
}

#[tokio::test]
async fn malformed_replacement_file_update_is_nonpublishing() {
    let f = fixture().await;
    for invalid in [
        "invalid JSON",
        r#"{"user.sub":{"alice":{"department":null}}}"#,
        r#"{"user.sub":{"alice":{},"alice":{}}}"#,
    ] {
        std::fs::write(f.dir.path().join("directory.json"), invalid).unwrap();
        assert_rejected(
            &f,
            candidate("file", Some(601), &["department -> user.department"]),
            "directory",
        )
        .await;
    }
}

#[tokio::test]
async fn missing_rest_configuration_update_is_nonpublishing() {
    let f = fixture().await;
    assert_rejected(
        &f,
        candidate("rest/1", Some(600), &["department -> user.department"]),
        "missing [trusted_service_http.directory]",
    )
    .await;
}

#[tokio::test]
async fn unchanged_declarations_reuse_store_without_reading_deleted_file() {
    let f = fixture().await;
    let held = f.policy.get_current_snapshot();
    std::fs::remove_file(f.dir.path().join("directory.json")).unwrap();
    f.policy
        .update_policy_from_container_bytes(f.original.clone())
        .await
        .unwrap();
    assert_persisted(&f, &f.original, 2).await;
    assert!(f.sources.stale_sources_for_actor(&address()).is_empty());
    assert!(
        lookup(&f.sources)
            .await
            .iter()
            .any(|attr| attr.value_has("engineering"))
    );
    assert_eq!(held.vinst(), 1);
}

#[tokio::test]
async fn mapping_change_rebuilds_store_and_keeps_held_snapshot_coherent() {
    let f = fixture().await;
    let held = f.policy.get_current_snapshot();
    let changed = candidate("file", Some(600), &["badge -> user.badge"]);
    f.policy
        .update_policy_from_container_bytes(changed.clone())
        .await
        .unwrap();
    assert_persisted(&f, &changed, 2).await;
    assert_eq!(f.sources.stale_sources_for_actor(&address()).len(), 1);
    let attrs = lookup(&f.sources).await;
    assert_eq!(attrs.len(), 2);
    assert!(
        attrs
            .iter()
            .any(|attr| attr.get_key() == "user.badge" && attr.value_has("cleared"))
    );
    assert!(
        !attrs
            .iter()
            .any(|attr| attr.get_key().starts_with("user.department"))
    );
    assert_eq!(held.vinst(), 1);
    assert_eq!(held.container().as_bytes(), f.original.as_slice());
    assert_eq!(
        held.policy()
            .trusted_service_by_id("directory")
            .unwrap()
            .returns_attrs[0]
            .service_attr_key,
        "department"
    );
}

#[tokio::test]
async fn ttl_change_rebuilds_store_with_new_snapshot_deadline() {
    let f = fixture().await;
    let started = std::time::SystemTime::now();
    let changed = candidate("file", Some(120), &["department -> user.department"]);
    f.policy
        .update_policy_from_container_bytes(changed.clone())
        .await
        .unwrap();
    assert_persisted(&f, &changed, 2).await;
    assert_eq!(f.sources.stale_sources_for_actor(&address()).len(), 1);
    for attr in lookup(&f.sources).await {
        assert!(attr.get_expires() >= started + std::time::Duration::from_secs(120));
        assert!(
            attr.get_expires()
                <= std::time::SystemTime::now() + std::time::Duration::from_secs(120)
        );
    }
}

#[tokio::test]
async fn identity_declaration_change_rebuilds_and_resolves_new_lookup_key() {
    let f = fixture().await;
    let changed = make_trusted_service_policy_with_identity(
        "directory",
        "file",
        Some(600),
        &["department -> user.department"],
        &["department"],
    );
    f.policy
        .update_policy_from_container_bytes(changed.clone())
        .await
        .unwrap();
    assert_persisted(&f, &changed, 2).await;
    assert_eq!(f.sources.stale_sources_for_actor(&address()).len(), 1);
    assert!(
        f.policy
            .get_current()
            .lookup_identity_keys()
            .contains(&"user.department")
    );
    assert!(
        lookup(&f.sources)
            .await
            .iter()
            .any(|attr| attr.value_has("engineering"))
    );
}

#[tokio::test]
async fn restart_restores_accepted_container_generation_and_fresh_source_revision() {
    let f = fixture().await;
    let changed = candidate("file", Some(1200), &["badge -> user.badge"]);
    f.policy
        .update_policy_from_container_bytes(changed.clone())
        .await
        .unwrap();
    let before = f.sources.stale_sources_for_actor(&address())[0].1;
    let restarted_sources = Arc::new(TrustedServicesMgr::new());
    let restarted = PolicyMgr::new_from_state(
        PolicyRepo::new(f.db.clone()),
        Arc::new(FakeResolver::ip_only()),
        restarted_sources.clone(),
        f.dir.path().into(),
    )
    .await
    .unwrap();
    assert_eq!(
        restarted.get_current_container().as_bytes(),
        changed.as_slice()
    );
    assert_eq!(restarted.get_current_snapshot().vinst(), 2);
    let after = restarted_sources.stale_sources_for_actor(&address());
    assert_eq!(after.len(), 1);
    assert_ne!(before, after[0].1);
    assert_eq!(restarted_sources.statuses()[0].health, "unverified");
    assert!(
        lookup(&restarted_sources)
            .await
            .iter()
            .any(|attr| attr.get_key() == "user.badge")
    );
}

#[tokio::test]
async fn jointly_submitted_accepted_and_rejected_updates_advance_only_once() {
    let f = fixture().await;
    let accepted = candidate("file", Some(1200), &["badge -> user.badge"]);
    let rejected = candidate("unsupported", Some(600), &[]);
    let (good, bad) = tokio::join!(
        f.policy
            .update_policy_from_container_bytes(accepted.clone()),
        f.policy.update_policy_from_container_bytes(rejected),
    );
    assert!(good.is_ok());
    assert!(bad.is_err());
    assert_persisted(&f, &accepted, 2).await;
    assert!(
        lookup(&f.sources)
            .await
            .iter()
            .any(|attr| attr.get_key() == "user.badge")
    );
}
