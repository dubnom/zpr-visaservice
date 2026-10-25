use super::*;
use crate::db::{ActorRepo, DbConnection, FakeDb, FaultMode, NodeRepo};
use crate::test_helpers::{make_adapter_actor_defexp, make_node_actor_defexp};
use crate::trusted_services::TrustedServiceInterface;
use std::sync::atomic::{AtomicU64, Ordering};

struct Source(AtomicU64);

#[async_trait::async_trait]
impl TrustedServiceInterface for Source {
    async fn get_attributes_for_actor(
        &self,
        _: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        self.0.fetch_add(1, Ordering::Relaxed);
        Ok(vec![])
    }
    async fn flush(&self) -> Result<(), ServiceError> {
        Ok(())
    }
    fn current_revision(&self) -> u64 {
        7
    }
    fn get_source_id(&self) -> &str {
        "directory"
    }
}

struct Fixture {
    asm: Arc<Assembly>,
    db: Arc<FakeDb>,
    source: Arc<Source>,
    node: IpAddr,
    adapter: IpAddr,
}

impl Fixture {
    async fn new() -> Self {
        let mut asm = crate::assembly::tests::new_assembly_for_tests(None).await;
        let db = Arc::new(FakeDb::new());
        asm.actor_mgr = Arc::new(crate::actor_mgr::ActorMgr::new(
            ActorRepo::new(db.clone()),
            NodeRepo::new(Arc::new(FakeDb::new())),
            asm.counters.clone(),
        ));
        let source = Arc::new(Source(AtomicU64::new(0)));
        asm.ts_mgr.update_services(vec![source.clone()]);
        let node = asm.net_mgr.get_next_zpr_addr(&Role::Node).unwrap();
        let adapter = asm.net_mgr.get_next_zpr_addr(&Role::Adapter).unwrap();
        let node_actor = make_node_actor_defexp(&node.to_string(), "node", "10.0.0.1:1234");
        asm.actor_mgr.add_node(&node_actor, false).await.unwrap();
        let mut actor = make_adapter_actor_defexp(&adapter.to_string(), "adapter");
        actor
            .add_attribute(Attribute::builder(key::SERVICES).values(["svc"]))
            .unwrap();
        asm.actor_mgr
            .add_adapter_via_node(&actor, &node)
            .await
            .unwrap();
        for address in [node, adapter] {
            asm.ts_mgr.record_revision(&address, "directory", 7);
        }
        Self {
            asm: Arc::new(asm),
            db,
            source,
            node,
            adapter,
        }
    }

    async fn disconnect(&self, address: IpAddr) -> Result<(), ServiceError> {
        self.asm
            .cc
            .disconnect(self.asm.clone(), address, vsapi::DisconnectReason::Admin)
            .await
    }

    fn current(&self, address: IpAddr) -> bool {
        self.asm.ts_mgr.stale_sources_for_actor(&address).is_empty()
    }

    fn assert_reserved(&self, address: IpAddr) {
        assert!(
            self.asm.net_mgr.take_zpr_addr(&address).is_err(),
            "address with unconfirmed cleanup was recycled"
        );
    }

    fn assert_released(&self, address: IpAddr) {
        self.asm
            .net_mgr
            .take_zpr_addr(&address)
            .expect("cleaned address must be reusable");
    }

    fn reject(&self) {
        self.db.set_set_ex_fault(FaultMode::Reject);
    }
    fn repair(&self) {
        self.db.set_set_ex_fault(FaultMode::None);
        self.db.set_del_fault(FaultMode::None);
    }
}

#[tokio::test]
async fn successful_adapter_disconnect_purges_revision_before_reuse() {
    let f = Fixture::new().await;
    f.disconnect(f.adapter).await.unwrap();
    assert!(!f.current(f.adapter));
    assert!(
        f.asm
            .actor_mgr
            .get_actor_by_zpr_addr(&f.adapter)
            .await
            .unwrap()
            .is_none()
    );
    f.assert_released(f.adapter);
}

#[tokio::test]
async fn rejected_adapter_removal_keeps_address_reserved_and_revision_owned() {
    let f = Fixture::new().await;
    f.reject();
    f.disconnect(f.adapter).await.unwrap();
    assert!(f.current(f.adapter));
    assert!(
        f.asm
            .actor_mgr
            .get_actor_by_zpr_addr(&f.adapter)
            .await
            .unwrap()
            .is_some()
    );
    f.assert_reserved(f.adapter);
}

#[tokio::test]
async fn rejected_cascade_removal_cannot_recycle_adapter_or_node() {
    let f = Fixture::new().await;
    f.reject();
    f.disconnect(f.node).await.unwrap();
    for address in [f.adapter, f.node] {
        assert!(f.current(address));
        f.assert_reserved(address);
        assert!(
            f.asm
                .actor_mgr
                .get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .is_some()
        );
    }
}

#[tokio::test]
async fn repaired_adapter_disconnect_purges_records_and_releases_retained_address() {
    let f = Fixture::new().await;
    f.reject();
    f.disconnect(f.adapter).await.unwrap();
    f.repair();
    f.disconnect(f.adapter).await.unwrap();
    assert!(!f.current(f.adapter));
    f.assert_released(f.adapter);
}

#[tokio::test]
async fn absent_actor_successful_cleanup_purges_orphan_revision_and_allocation() {
    let f = Fixture::new().await;
    let absent = f.asm.net_mgr.get_next_zpr_addr(&Role::Adapter).unwrap();
    f.asm.ts_mgr.record_revision(&absent, "directory", 7);
    f.disconnect(absent).await.unwrap();
    assert!(!f.current(absent));
    f.assert_released(absent);
}

#[tokio::test]
async fn absent_actor_failed_cleanup_keeps_orphan_address_quarantined() {
    let f = Fixture::new().await;
    let absent = f.asm.net_mgr.get_next_zpr_addr(&Role::Adapter).unwrap();
    f.asm.ts_mgr.record_revision(&absent, "directory", 7);
    f.reject();
    f.disconnect(absent).await.unwrap();
    assert!(f.current(absent));
    f.assert_reserved(absent);
}

#[tokio::test]
async fn malformed_actor_load_returns_error_without_releasing_address() {
    let f = Fixture::new().await;
    let attrs = format!("actor:{}:attrs", f.adapter.to_string().replace(':', "-"));
    f.db.hset(&attrs, key::CN, "{invalid").await.unwrap();
    assert!(f.disconnect(f.adapter).await.is_err());
    assert!(f.current(f.adapter));
    f.assert_reserved(f.adapter);
}

#[tokio::test]
async fn successful_node_disconnect_cleans_cascade_revisions_and_allocations() {
    let f = Fixture::new().await;
    f.disconnect(f.node).await.unwrap();
    for address in [f.node, f.adapter] {
        assert!(!f.current(address));
        assert!(
            f.asm
                .actor_mgr
                .get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .is_none()
        );
        f.assert_released(address);
    }
}

#[tokio::test]
async fn adapter_disconnect_leaves_peer_node_revision_and_allocation_unchanged() {
    let f = Fixture::new().await;
    f.disconnect(f.adapter).await.unwrap();
    assert!(f.current(f.node));
    f.assert_reserved(f.node);
    assert!(
        f.asm
            .actor_mgr
            .get_actor_by_zpr_addr(&f.node)
            .await
            .unwrap()
            .is_some()
    );
}

#[tokio::test]
async fn recycled_address_requires_lookup_for_new_actor_even_for_empty_observation() {
    let f = Fixture::new().await;
    f.disconnect(f.adapter).await.unwrap();
    f.assert_released(f.adapter);
    let mut actor = make_adapter_actor_defexp(&f.adapter.to_string(), "new-owner");
    f.asm
        .actor_mgr
        .add_adapter_via_node(&actor, &f.node)
        .await
        .unwrap();
    crate::actor_attributes::refresh_and_persist_actor(&f.asm, &mut actor)
        .await
        .unwrap();
    assert_eq!(f.source.0.load(Ordering::Relaxed), 1);
    assert!(f.current(f.adapter));
    crate::actor_attributes::refresh_and_persist_actor(&f.asm, &mut actor)
        .await
        .unwrap();
    assert_eq!(f.source.0.load(Ordering::Relaxed), 1);
}

#[tokio::test]
async fn repeated_disconnect_without_reassignment_remains_safe() {
    let f = Fixture::new().await;
    f.disconnect(f.adapter).await.unwrap();
    f.disconnect(f.adapter).await.unwrap();
    assert!(!f.current(f.adapter));
    f.assert_released(f.adapter);
    assert!(f.current(f.node));
}

#[tokio::test]
async fn late_cleanup_delete_rejection_keeps_allocation_and_revision() {
    let f = Fixture::new().await;
    f.db.set_del_fault(FaultMode::Reject);
    f.disconnect(f.adapter).await.unwrap();
    assert!(
        f.asm
            .actor_mgr
            .get_actor_by_zpr_addr(&f.adapter)
            .await
            .unwrap()
            .is_none()
    );
    assert!(f.current(f.adapter));
    f.assert_reserved(f.adapter);
}

#[tokio::test]
async fn ambiguous_cleanup_delete_reply_keeps_allocation_and_revision() {
    let f = Fixture::new().await;
    f.db.set_del_fault(FaultMode::ApplyThenError);
    f.disconnect(f.adapter).await.unwrap();
    assert!(f.current(f.adapter));
    f.assert_reserved(f.adapter);
}

#[tokio::test]
async fn repaired_late_cleanup_failure_releases_address_even_when_actor_is_absent() {
    let f = Fixture::new().await;
    f.db.set_del_fault(FaultMode::ApplyThenError);
    f.disconnect(f.adapter).await.unwrap();
    f.repair();
    f.disconnect(f.adapter).await.unwrap();
    assert!(!f.current(f.adapter));
    f.assert_released(f.adapter);
}

#[tokio::test]
async fn unmanaged_actor_cleanup_purges_revision_without_touching_managed_peer() {
    let f = Fixture::new().await;
    let address: IpAddr = "192.0.2.123".parse().unwrap();
    let actor = make_adapter_actor_defexp(&address.to_string(), "unmanaged");
    f.asm
        .actor_mgr
        .hack_add_adapter_no_node(&actor)
        .await
        .unwrap();
    f.asm.ts_mgr.record_revision(&address, "directory", 7);
    f.disconnect(address).await.unwrap();
    assert!(!f.current(address));
    assert!(f.current(f.adapter));
    f.assert_reserved(f.adapter);
}
