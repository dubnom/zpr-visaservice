use super::*;
use crate::trusted_services::test_support::test_mapper;
use std::time::SystemTime;

const RED: &str = r#"{"user.sub":{"alice":{"color":["red"]}}}"#;
const BLUE: &str = r#"{"user.sub":{"alice":{"color":["blue"]}}}"#;

struct Fixture {
    dir: tempfile::TempDir,
    store: FileAttributeStore,
}

impl Fixture {
    fn new() -> Self {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().join("attributes.json");
        fs::write(&path, RED).unwrap();
        let store = FileAttributeStore::new(
            "directory".into(),
            test_mapper(),
            Duration::from_secs(600),
            &path,
        )
        .unwrap();
        Self { dir, store }
    }

    fn write(&self, json: &str) {
        fs::write(&self.store.fp, json).unwrap();
    }

    fn set_remaining(&self, remaining: Option<Duration>) -> Arc<Snapshot> {
        let old = self.store.snapshot.load_full();
        let expires_at = remaining.map_or_else(
            || Instant::now() - Duration::from_secs(1),
            |remaining| Instant::now() + remaining,
        );
        let snapshot = Arc::new(Snapshot {
            attributes: old.attributes.clone(),
            expires_at,
            revision: old.revision,
        });
        self.store.snapshot.store(snapshot.clone());
        snapshot
    }

    async fn lookup(&self) -> Result<Vec<Attribute>, ServiceError> {
        self.store
            .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
            .await
    }
}

fn assert_color(attrs: &[Attribute], color: &str) {
    assert_eq!(attrs.len(), 1);
    assert_eq!(attrs[0].get_key(), "user.color");
    assert_eq!(attrs[0].get_value(), [color]);
    assert_eq!(attrs[0].get_source(), "directory");
}

#[tokio::test]
async fn live_snapshot_ignores_backing_changes_until_reload_is_due() {
    let f = Fixture::new();
    let original = f.store.snapshot.load_full();
    f.write(BLUE);
    assert_color(&f.lookup().await.unwrap(), "red");
    assert!(Arc::ptr_eq(&original, &f.store.snapshot.load_full()));
}

#[tokio::test]
async fn live_snapshot_survives_missing_file_without_implicit_revision_change() {
    let f = Fixture::new();
    let original = f.store.snapshot.load_full();
    fs::remove_file(&f.store.fp).unwrap();
    assert_color(&f.lookup().await.unwrap(), "red");
    assert_eq!(f.store.current_revision(), original.revision);
    assert!(Arc::ptr_eq(&original, &f.store.snapshot.load_full()));
}

#[tokio::test]
async fn near_expiry_reloads_before_old_snapshot_actually_expires() {
    let f = Fixture::new();
    let old = f.set_remaining(Some(Duration::from_secs(59)));
    assert!(old.remaining() > Duration::ZERO);
    f.write(BLUE);
    assert_color(&f.lookup().await.unwrap(), "blue");
    assert_ne!(f.store.current_revision(), old.revision);
}

#[tokio::test]
async fn expired_snapshot_reloads_instead_of_returning_expired_claims() {
    let f = Fixture::new();
    let old = f.set_remaining(None);
    f.write(BLUE);
    let attrs = f.lookup().await.unwrap();
    assert_color(&attrs, "blue");
    assert!(!attrs[0].is_expired());
    assert_ne!(f.store.current_revision(), old.revision);
}

#[tokio::test]
async fn near_expiry_reload_failure_never_falls_back_to_still_unexpired_data() {
    let f = Fixture::new();
    let old = f.set_remaining(Some(Duration::from_secs(59)));
    f.write("{invalid");
    for _ in 0..2 {
        let error = f.lookup().await.unwrap_err().to_string();
        assert!(
            error.contains("directory") && error.contains("attributes.json"),
            "{error}"
        );
        assert!(Arc::ptr_eq(&old, &f.store.snapshot.load_full()));
        assert_eq!(f.store.current_revision(), old.revision);
    }
    f.write(BLUE);
    assert_color(&f.lookup().await.unwrap(), "blue");
}

#[tokio::test]
async fn queued_automatic_reload_callers_share_one_published_snapshot() {
    let f = Fixture::new();
    let old = f.set_remaining(None);
    f.write(BLUE);
    let guard = f.store.refresh_lock.lock().await;
    let mut pending = Box::pin(futures::future::join_all(
        (0..16).map(|_| f.store.live_snapshot()),
    ));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    drop(guard);
    let snapshots = pending
        .await
        .into_iter()
        .map(Result::unwrap)
        .collect::<Vec<_>>();
    assert_ne!(snapshots[0].revision, old.revision);
    for snapshot in &snapshots {
        assert!(Arc::ptr_eq(snapshot, &snapshots[0]));
    }
}

#[tokio::test]
async fn queued_explicit_flushes_each_publish_their_own_revision() {
    let f = Fixture::new();
    let original = f.store.current_revision();
    let guard = f.store.refresh_lock.lock().await;
    let mut first = Box::pin(f.store.flush());
    let mut second = Box::pin(f.store.flush());
    assert!(futures::poll!(first.as_mut()).is_pending());
    assert!(futures::poll!(second.as_mut()).is_pending());
    drop(guard);
    first.await.unwrap();
    let intermediate = f.store.snapshot.load_full();
    second.await.unwrap();
    assert_ne!(intermediate.revision, original);
    assert_ne!(f.store.current_revision(), intermediate.revision);
}

#[tokio::test]
async fn queued_reader_rechecks_snapshot_after_successful_explicit_flush() {
    let f = Fixture::new();
    f.set_remaining(None);
    f.write(BLUE);
    let guard = f.store.refresh_lock.lock().await;
    let mut flush = Box::pin(f.store.flush());
    assert!(futures::poll!(flush.as_mut()).is_pending());
    let mut reader = Box::pin(f.store.live_snapshot());
    assert!(futures::poll!(reader.as_mut()).is_pending());
    drop(guard);
    flush.await.unwrap();
    let published = f.store.snapshot.load_full();
    fs::remove_file(&f.store.fp).unwrap();
    assert!(Arc::ptr_eq(&reader.await.unwrap(), &published));
    assert_color(&f.lookup().await.unwrap(), "blue");
}

#[tokio::test]
async fn queued_reader_can_recover_after_preceding_flush_failed() {
    let f = Fixture::new();
    let original = f.set_remaining(None);
    f.write("{invalid");
    let guard = f.store.refresh_lock.lock().await;
    let mut flush = Box::pin(f.store.flush());
    assert!(futures::poll!(flush.as_mut()).is_pending());
    let mut reader = Box::pin(f.store.live_snapshot());
    assert!(futures::poll!(reader.as_mut()).is_pending());
    drop(guard);
    assert!(flush.await.is_err());
    assert!(Arc::ptr_eq(&original, &f.store.snapshot.load_full()));
    f.write(BLUE);
    let repaired = reader.await.unwrap();
    assert_ne!(repaired.revision, original.revision);
    assert_color(&f.lookup().await.unwrap(), "blue");
}

#[tokio::test]
async fn held_snapshot_remains_immutable_after_replacement() {
    let f = Fixture::new();
    let original = f.store.snapshot.load_full();
    f.write(BLUE);
    f.store.flush().await.unwrap();
    assert_eq!(original.attributes.0["user.sub"]["alice"]["color"], ["red"]);
    assert_eq!(
        f.store.snapshot.load().attributes.0["user.sub"]["alice"]["color"],
        ["blue"]
    );
    assert!(!Arc::ptr_eq(&original, &f.store.snapshot.load_full()));
}

#[tokio::test]
async fn cache_hits_do_not_extend_snapshot_deadline() {
    let f = Fixture::new();
    let original = f.store.snapshot.load_full();
    for _ in 0..3 {
        let before = SystemTime::now() + original.remaining();
        let attrs = f.lookup().await.unwrap();
        let after = SystemTime::now() + original.remaining();
        // SystemTime/Instant samples need a small allowance for sampling order.
        let allowance = Duration::from_millis(10);
        assert!(attrs[0].get_expires() >= before - allowance);
        assert!(attrs[0].get_expires() <= after + allowance);
        assert_eq!(f.store.snapshot.load().expires_at, original.expires_at);
    }
}

#[tokio::test]
async fn automatic_reload_deadline_is_bounded_by_configured_ttl() {
    let f = Fixture::new();
    f.set_remaining(None);
    let before = Instant::now();
    let wall_before = SystemTime::now();
    let attrs = f.lookup().await.unwrap();
    let wall_after = SystemTime::now();
    let after = Instant::now();
    let snapshot = f.store.snapshot.load_full();
    assert!(snapshot.expires_at >= before + f.store.ttl);
    assert!(snapshot.expires_at <= after + f.store.ttl);
    assert!(attrs[0].get_expires() >= wall_before + f.store.ttl - Duration::from_millis(10));
    assert!(attrs[0].get_expires() <= wall_after + f.store.ttl);
}

#[tokio::test]
async fn semantic_cardinality_failure_is_lookup_failure_not_json_reload_failure() {
    let f = Fixture::new();
    let old = f.set_remaining(None);
    f.write(r#"{"user.sub":{"alice":{"color":[]}}}"#);
    assert!(matches!(
        f.lookup().await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    let malformed = f.store.snapshot.load_full();
    assert_ne!(malformed.revision, old.revision);
    f.write(BLUE);
    // A live decoded snapshot is authoritative until explicitly or automatically reloaded.
    assert!(f.lookup().await.is_err());
    assert!(Arc::ptr_eq(&malformed, &f.store.snapshot.load_full()));
    f.store.flush().await.unwrap();
    assert_color(&f.lookup().await.unwrap(), "blue");
}

#[tokio::test]
async fn empty_replacement_removes_old_subject_without_reusing_old_observation() {
    let f = Fixture::new();
    let old = f.set_remaining(None);
    f.write("{}");
    assert!(f.lookup().await.unwrap().is_empty());
    assert_ne!(f.store.current_revision(), old.revision);
    assert!(f.store.snapshot.load().attributes.0.is_empty());
}

#[tokio::test]
async fn atomic_file_replacement_is_loaded_as_complete_new_snapshot() {
    let f = Fixture::new();
    let original = f.store.snapshot.load_full();
    let replacement = f.dir.path().join("replacement.json");
    fs::write(&replacement, BLUE).unwrap();
    fs::rename(&replacement, &f.store.fp).unwrap();
    assert_color(&f.lookup().await.unwrap(), "red");
    f.store.flush().await.unwrap();
    assert_color(&f.lookup().await.unwrap(), "blue");
    assert_ne!(f.store.current_revision(), original.revision);
}
