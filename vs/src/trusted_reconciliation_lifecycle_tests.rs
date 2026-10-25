use super::*;
use crate::db::FakeDb;
use crate::error::ServiceError;
use crate::trusted_services::TrustedServiceInterface;
use libeval::attribute::Attribute;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};

#[tokio::test]
async fn pristine_status_is_idle_without_invented_progress_or_error() {
    let db = FakeDb::new();
    let status = status(&db, "directory").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (0, 0, "idle")
    );
    assert_eq!(status.source_id, "directory");
    assert!(status.error.is_none());
}

#[tokio::test]
async fn encoded_source_names_are_isolated_without_normalization() {
    let db = FakeDb::new();
    let names = [
        "a/b",
        "a%2Fb",
        "a:b",
        "a b",
        "A",
        "a",
        "caf\u{e9}",
        "cafe\u{301}",
    ];
    for (index, name) in names.iter().enumerate() {
        for _ in 0..=index {
            enqueue(&db, name).await.unwrap();
        }
    }
    let keys = db.scan_match_all(format!("{PREFIX}*")).await.unwrap();
    assert_eq!(keys.len(), names.len());
    for (index, name) in names.iter().enumerate() {
        assert_eq!(status(&db, name).await.unwrap().requested, index as u64 + 1);
    }
}

#[tokio::test]
async fn duplicate_notifications_increment_generation_without_completion() {
    let db = FakeDb::new();
    for generation in 1..=20 {
        assert_eq!(enqueue(&db, "directory").await.unwrap(), generation);
    }
    let status = status(&db, "directory").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (20, 0, "pending")
    );
}

#[tokio::test]
async fn failure_keeps_prior_completed_generation_and_pending_work() {
    let db = FakeDb::new();
    enqueue(&db, "directory").await.unwrap();
    finish(&db, "directory", 1, true).await.unwrap();
    enqueue(&db, "directory").await.unwrap();
    for _ in 0..2 {
        finish(&db, "directory", 2, false).await.unwrap();
    }
    let status = status(&db, "directory").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (2, 1, "failed")
    );
    assert!(status.error.unwrap().contains("retry pending"));
}

#[tokio::test]
async fn successful_retry_clears_error_without_extra_request() {
    let db = FakeDb::new();
    enqueue(&db, "directory").await.unwrap();
    finish(&db, "directory", 1, false).await.unwrap();
    finish(&db, "directory", 1, true).await.unwrap();
    let status = status(&db, "directory").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (1, 1, "idle")
    );
    assert!(status.error.is_none());
}

#[tokio::test]
async fn completion_of_captured_generation_keeps_newer_request_pending() {
    let db = FakeDb::new();
    let captured = enqueue(&db, "directory").await.unwrap();
    enqueue(&db, "directory").await.unwrap();
    finish(&db, "directory", captured, true).await.unwrap();
    let status = status(&db, "directory").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (2, 1, "pending")
    );
}

#[tokio::test]
async fn corrupt_requested_generations_are_explicit_errors_not_idle_defaults() {
    for value in ["-1", "x", "", "18446744073709551616"] {
        let db = FakeDb::new();
        db.set(&request_key("directory"), value).await.unwrap();
        assert!(status(&db, "directory").await.is_err(), "{value}");
    }
}

#[tokio::test]
async fn corrupt_completed_generations_are_explicit_errors_not_success() {
    for value in ["-1", "x", "", "18446744073709551616"] {
        let db = FakeDb::new();
        enqueue(&db, "directory").await.unwrap();
        db.hset(&progress_key("directory"), "completed", value)
            .await
            .unwrap();
        assert!(status(&db, "directory").await.is_err(), "{value}");
    }
}

struct Source {
    db: Arc<dyn DbConnection>,
    flushes: AtomicU64,
    lookups: AtomicU64,
    revision: AtomicU64,
    flush_failed: AtomicBool,
    lookup_failed: AtomicBool,
    enqueue_during_flush: AtomicBool,
}

#[async_trait::async_trait]
impl TrustedServiceInterface for Source {
    async fn get_attributes_for_actor(
        &self,
        _: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        self.lookups.fetch_add(1, Ordering::Relaxed);
        if self.lookup_failed.load(Ordering::Relaxed) {
            Err(ServiceError::AttributesIndeterminate(
                "hr: unavailable".into(),
            ))
        } else {
            Ok(vec![])
        }
    }
    async fn flush(&self) -> Result<(), ServiceError> {
        self.flushes.fetch_add(1, Ordering::Relaxed);
        if self.flush_failed.load(Ordering::Relaxed) {
            return Err(ServiceError::AttributesIndeterminate(
                "hr: reload failed".into(),
            ));
        }
        self.revision.fetch_add(1, Ordering::Relaxed);
        if self.enqueue_during_flush.swap(false, Ordering::Relaxed) {
            enqueue(self.db.as_ref(), "hr").await.unwrap();
        }
        Ok(())
    }
    fn current_revision(&self) -> u64 {
        self.revision.load(Ordering::Relaxed)
    }
    fn get_source_id(&self) -> &str {
        "hr"
    }
}

async fn fixture() -> (Arc<Assembly>, std::net::IpAddr, Arc<Source>) {
    let (asm, node, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
    let source = Arc::new(Source {
        db: asm.state_db.clone(),
        flushes: AtomicU64::new(0),
        lookups: AtomicU64::new(0),
        revision: AtomicU64::new(50),
        flush_failed: AtomicBool::new(false),
        lookup_failed: AtomicBool::new(false),
        enqueue_during_flush: AtomicBool::new(false),
    });
    asm.ts_mgr.update_services(vec![source.clone()]);
    (asm, node, source)
}

async fn issue(asm: &Arc<Assembly>, node: std::net::IpAddr) -> u64 {
    let packet =
        zpr::vsapi_types::PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80)
            .unwrap();
    let crate::visareq_worker::VisaDecision::Allow(visa, _) =
        crate::visareq_worker::process_visa_request_for_test(asm.clone(), node, packet).await
    else {
        panic!("baseline visa must allow");
    };
    visa.issuer_id
}

#[tokio::test]
async fn idle_processing_does_not_flush_or_query_sources() {
    let (asm, _, source) = fixture().await;
    process(&asm).await.unwrap();
    assert_eq!(source.flushes.load(Ordering::Relaxed), 0);
    assert_eq!(source.lookups.load(Ordering::Relaxed), 0);
}

#[tokio::test]
async fn coalesced_requests_flush_once_and_no_live_visas_need_no_actor_lookup() {
    let (asm, _, source) = fixture().await;
    for _ in 0..5 {
        enqueue(asm.state_db.as_ref(), "hr").await.unwrap();
    }
    process(&asm).await.unwrap();
    let status = status(asm.state_db.as_ref(), "hr").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (5, 5, "idle")
    );
    assert_eq!(source.flushes.load(Ordering::Relaxed), 1);
    assert_eq!(source.lookups.load(Ordering::Relaxed), 0);
    process(&asm).await.unwrap();
    assert_eq!(source.flushes.load(Ordering::Relaxed), 1);
}

#[tokio::test]
async fn failed_flush_keeps_work_pending_and_skips_actor_phase_until_repair() {
    let (asm, node, source) = fixture().await;
    issue(&asm, node).await;
    let before = source.lookups.load(Ordering::Relaxed);
    source.flush_failed.store(true, Ordering::Relaxed);
    enqueue(asm.state_db.as_ref(), "hr").await.unwrap();
    process(&asm).await.unwrap();
    let failed = status(asm.state_db.as_ref(), "hr").await.unwrap();
    assert_eq!((failed.completed, failed.state), (0, "failed"));
    assert_eq!(source.lookups.load(Ordering::Relaxed), before);
    source.flush_failed.store(false, Ordering::Relaxed);
    process(&asm).await.unwrap();
    assert_eq!(
        status(asm.state_db.as_ref(), "hr").await.unwrap().state,
        "idle"
    );
    assert_eq!(source.flushes.load(Ordering::Relaxed), 2);
    assert!(source.lookups.load(Ordering::Relaxed) > before);
}

#[tokio::test]
async fn failed_lookup_does_not_acknowledge_pass_even_when_revoke_is_queued() {
    let (asm, node, source) = fixture().await;
    let visa_id = issue(&asm, node).await;
    source.lookup_failed.store(true, Ordering::Relaxed);
    enqueue(asm.state_db.as_ref(), "hr").await.unwrap();
    process(&asm).await.unwrap();
    let status = status(asm.state_db.as_ref(), "hr").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (1, 0, "failed")
    );
    assert_eq!(
        asm.visa_mgr
            .get_pending_revoke_visa_ids_for_node(&node)
            .await
            .unwrap(),
        [visa_id]
    );
}

#[tokio::test]
async fn recovered_lookup_acknowledges_work_without_canceling_queued_revoke() {
    let (asm, node, source) = fixture().await;
    let visa_id = issue(&asm, node).await;
    let original = asm
        .visa_mgr
        .get_visa_with_metadata_by_id(visa_id)
        .await
        .unwrap()
        .unwrap()
        .visa
        .expires;
    source.lookup_failed.store(true, Ordering::Relaxed);
    enqueue(asm.state_db.as_ref(), "hr").await.unwrap();
    process(&asm).await.unwrap();
    source.lookup_failed.store(false, Ordering::Relaxed);
    process(&asm).await.unwrap();
    let status = status(asm.state_db.as_ref(), "hr").await.unwrap();
    assert_eq!(
        (status.requested, status.completed, status.state),
        (1, 1, "idle")
    );
    assert!(status.error.is_none());
    assert_eq!(
        asm.visa_mgr
            .get_pending_revoke_visa_ids_for_node(&node)
            .await
            .unwrap(),
        [visa_id]
    );
    assert_eq!(
        asm.visa_mgr
            .get_visa_with_metadata_by_id(visa_id)
            .await
            .unwrap()
            .unwrap()
            .visa
            .expires,
        original
    );
    for endpoint in ["fd5a:5052:4000::a", "fd5a:5052:4000::b"] {
        assert!(
            asm.ts_mgr
                .stale_sources_for_actor(&endpoint.parse().unwrap())
                .is_empty()
        );
    }
}

#[tokio::test]
async fn absent_source_is_retained_for_retry_without_affecting_healthy_source() {
    let (asm, _, source) = fixture().await;
    enqueue(asm.state_db.as_ref(), "missing").await.unwrap();
    enqueue(asm.state_db.as_ref(), "hr").await.unwrap();
    process(&asm).await.unwrap();
    assert_eq!(
        status(asm.state_db.as_ref(), "missing")
            .await
            .unwrap()
            .state,
        "failed"
    );
    assert_eq!(
        status(asm.state_db.as_ref(), "hr").await.unwrap().state,
        "idle"
    );
    assert_eq!(source.flushes.load(Ordering::Relaxed), 1);
}

#[tokio::test]
async fn request_arriving_during_flush_requires_another_successful_pass() {
    let (asm, _, source) = fixture().await;
    source.enqueue_during_flush.store(true, Ordering::Relaxed);
    enqueue(asm.state_db.as_ref(), "hr").await.unwrap();
    process(&asm).await.unwrap();
    let pending = status(asm.state_db.as_ref(), "hr").await.unwrap();
    assert_eq!(
        (pending.requested, pending.completed, pending.state),
        (2, 1, "pending")
    );
    process(&asm).await.unwrap();
    let done = status(asm.state_db.as_ref(), "hr").await.unwrap();
    assert_eq!((done.requested, done.completed, done.state), (2, 2, "idle"));
    assert_eq!(source.flushes.load(Ordering::Relaxed), 2);
}
