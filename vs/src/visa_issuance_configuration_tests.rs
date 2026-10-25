use super::*;
use crate::db::{DbConnection, FakeDb, VisaRepo};
use libeval::eval_result::Direction;

struct Fixture {
    asm: Arc<Assembly>,
    db: Arc<FakeDb>,
    repo: VisaRepo,
    node: IpAddr,
}

impl Fixture {
    async fn new() -> Self {
        let (asm, node, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
        let mut asm = Arc::try_unwrap(asm).ok().unwrap();
        let db = Arc::new(FakeDb::new());
        let repo = VisaRepo::new(db.clone(), 1).await.unwrap();
        asm.visa_mgr = crate::visa_mgr::VisaMgr::new(repo.clone());
        Self {
            asm: Arc::new(asm),
            db,
            repo,
            node,
        }
    }

    fn job(&self) -> VisaRequestJob {
        VisaRequestJob::new(
            self.node,
            PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80).unwrap(),
        )
        .0
    }

    async fn assert_store_reached(&self) {
        assert_eq!(
            self.db.get("visa:next_visa_id").await.unwrap().as_deref(),
            Some("2")
        );
        assert!(self.asm.visa_mgr.list_visa_metadata().await.is_empty());
    }

    async fn assert_revoked_everywhere(&self, created_vinst: u64) {
        let metadata = self.asm.visa_mgr.list_visa_metadata().await;
        assert_eq!(metadata.len(), 1);
        let (id, md) = &metadata[0];
        assert_eq!(md.created_vinst, created_vinst);
        assert_eq!(md.checked_vinst, created_vinst);
        for node in [self.node, "fd5a:5052:3000::2".parse().unwrap()] {
            assert_eq!(
                self.asm
                    .visa_mgr
                    .get_pending_revoke_visa_ids_for_node(&node)
                    .await
                    .unwrap(),
                [*id],
            );
            assert!(
                self.asm
                    .visa_mgr
                    .get_pending_visa_ids_for_node(&node)
                    .await
                    .unwrap()
                    .is_empty()
            );
        }
        let stored = self.db.get(&format!("visa:{id}")).await.unwrap().unwrap();
        let record: serde_json::Value = serde_json::from_str(&stored).unwrap();
        let nodes = record["node_states"].as_object().unwrap();
        assert_eq!(nodes.len(), 2);
        for node in nodes.values() {
            assert_eq!(node["state"], "PendingRevoke");
        }
        let restored =
            crate::visa_mgr::VisaMgr::new(VisaRepo::new(self.db.clone(), 1).await.unwrap());
        for node in [self.node, "fd5a:5052:3000::2".parse().unwrap()] {
            assert_eq!(
                restored
                    .get_pending_revoke_visa_ids_for_node(&node)
                    .await
                    .unwrap(),
                [*id]
            );
            assert!(
                restored
                    .get_pending_visa_ids_for_node(&node)
                    .await
                    .unwrap()
                    .is_empty()
            );
        }
    }
}

#[tokio::test]
async fn evaluated_allow_cannot_rebase_onto_a_later_toggle_generation() {
    for reenable in [false, true] {
        let f = Fixture::new().await;
        let configuration = f.asm.ts_mgr.configuration();
        let next_visa_before = f.db.get("visa:next_visa_id").await.unwrap();
        let policy = f.asm.policy_mgr.get_current();
        crate::trusted_services::control::set(f.asm.state_db.as_ref(), &f.asm.ts_mgr, "hr", false)
            .await
            .unwrap();
        if reenable {
            crate::trusted_services::control::set(
                f.asm.state_db.as_ref(),
                &f.asm.ts_mgr,
                "hr",
                true,
            )
            .await
            .unwrap();
        }
        let source = crate::test_helpers::make_adapter_actor_defexp("fd5a:5052:4000::a", "src");
        let destination =
            crate::test_helpers::make_adapter_actor_defexp("fd5a:5052:4000::b", "dst");
        let result = visa_from_allow_for_configuration(
            f.asm.clone(),
            &f.job(),
            &[Hit::new_no_signal(0, Direction::Forward)],
            (&policy, &configuration),
            Some(Route::new_direct(f.node.into())),
            &source,
            &destination,
        )
        .await
        .unwrap();
        assert!(matches!(result, VisaDecision::Deny(_)));
        assert!(f.asm.visa_mgr.list_visa_metadata().await.is_empty());
        assert_eq!(
            f.db.get("visa:next_visa_id").await.unwrap(),
            next_visa_before
        );
    }
}

#[tokio::test]
async fn source_change_during_visa_store_denies_and_persists_revoke_for_whole_path() {
    let f = Fixture::new().await;
    let vinst = f.asm.policy_mgr.get_current().vinst();
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.assert_store_reached().await;
    f.asm.ts_mgr.update_services(vec![]);
    drop(guard);
    assert!(matches!(pending.await.unwrap(), VisaDecision::Deny(_)));
    f.assert_revoked_everywhere(vinst).await;
}

#[tokio::test]
async fn policy_change_during_visa_store_cannot_return_old_generation_allow() {
    let f = Fixture::new().await;
    let vinst = f.asm.policy_mgr.get_current().vinst();
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.assert_store_reached().await;
    f.asm
        .policy_mgr
        .update_policy_from_container_bytes(crate::assembly::tests::make_policy(
            "2026-10-24T00:00:00Z",
            3,
            None,
        ))
        .await
        .unwrap();
    drop(guard);
    assert!(matches!(pending.await.unwrap(), VisaDecision::Deny(_)));
    f.assert_revoked_everywhere(vinst).await;
}

#[tokio::test]
async fn unchanged_configuration_allows_pending_visa_store_to_complete() {
    let f = Fixture::new().await;
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.assert_store_reached().await;
    drop(guard);
    let VisaDecision::Allow(visa, _) = pending.await.unwrap() else {
        panic!("unchanged configuration must allow");
    };
    assert!(
        f.asm
            .visa_mgr
            .get_pending_revoke_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .is_empty()
    );
    assert!(
        f.asm
            .visa_mgr
            .get_pending_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .contains(&visa.issuer_id)
    );
}

#[tokio::test]
async fn rejected_store_returns_original_error_without_claiming_successful_revoke() {
    let f = Fixture::new().await;
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.assert_store_reached().await;
    f.asm.ts_mgr.update_services(vec![]);
    f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    drop(guard);
    assert!(matches!(pending.await, Err(ServiceError::Store(_))));
    assert!(f.asm.visa_mgr.list_visa_metadata().await.is_empty());
    assert!(
        f.asm
            .visa_mgr
            .get_pending_revoke_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .is_empty()
    );
}

#[tokio::test]
async fn failed_revoke_write_is_an_explicit_error_not_a_successful_denial() {
    let f = Fixture::new().await;
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.assert_store_reached().await;
    // Queue behind the store, but before the subsequent revocation write.
    let mut stop_revoke = Box::pin(f.repo.hold_writes_for_test());
    assert!(futures::poll!(stop_revoke.as_mut()).is_pending());
    f.asm.ts_mgr.update_services(vec![]);
    drop(guard);
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let revoke_guard = stop_revoke.await;
    assert_eq!(f.asm.visa_mgr.list_visa_metadata().await.len(), 1);
    f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    drop(revoke_guard);
    assert!(matches!(pending.await, Err(ServiceError::Store(_))));
    assert!(
        f.asm
            .visa_mgr
            .get_pending_revoke_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .is_empty()
    );
    assert!(
        f.asm
            .visa_mgr
            .get_pending_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .is_empty()
    );
    let record: serde_json::Value =
        serde_json::from_str(&f.db.get("visa:2").await.unwrap().unwrap()).unwrap();
    assert!(
        record["node_states"]
            .as_object()
            .unwrap()
            .values()
            .all(|node| node["state"] == "Staged")
    );
    let restored = crate::visa_mgr::VisaMgr::new(VisaRepo::new(f.db.clone(), 1).await.unwrap());
    assert!(
        restored
            .get_pending_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .is_empty()
    );
    // Recovery needs another successful write; the error is not durable revoke evidence.
    f.db.set_set_ex_fault(crate::db::FaultMode::None);
    f.asm.visa_mgr.mark_visa_revoked(2).await.unwrap();
    f.assert_revoked_everywhere(f.asm.policy_mgr.get_current().vinst())
        .await;
}

async fn stage_without_activation(f: &Fixture) {
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.assert_store_reached().await;
    let mut activation = Box::pin(f.repo.hold_writes_for_test());
    assert!(futures::poll!(activation.as_mut()).is_pending());
    drop(guard);
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let activation_guard = activation.await;
    assert_eq!(f.asm.visa_mgr.list_visa_metadata().await.len(), 1);
    drop(pending);
    drop(activation_guard);
}

async fn assert_staged(f: &Fixture) {
    for node in [f.node, "fd5a:5052:3000::2".parse().unwrap()] {
        assert!(
            f.asm
                .visa_mgr
                .get_pending_visa_ids_for_node(&node)
                .await
                .unwrap()
                .is_empty()
        );
        assert!(
            f.asm
                .visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap()
                .is_empty()
        );
        assert_eq!(
            f.repo.get_node_visa_state(2, &node).unwrap(),
            crate::db::NodeVisaState::Staged
        );
    }
}

#[tokio::test]
async fn canceled_issuance_remains_staged_at_every_node_across_repository_restart() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    assert_staged(&f).await;
    let restored = VisaRepo::new(f.db.clone(), 1).await.unwrap();
    for node in [f.node, "fd5a:5052:3000::2".parse().unwrap()] {
        assert_eq!(
            restored.get_node_visa_state(2, &node).unwrap(),
            crate::db::NodeVisaState::Staged
        );
        assert!(
            restored
                .get_visa_ids_for_node_by_state(&node, crate::db::NodeVisaState::PendingInstall)
                .unwrap()
                .is_empty()
        );
    }
}

#[tokio::test]
async fn staged_activation_publishes_whole_path_once_and_preserves_expiration() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    let original = f
        .asm
        .visa_mgr
        .get_visa_with_metadata_by_id(2)
        .await
        .unwrap()
        .unwrap();
    assert!(f.repo.activate_staged_visa(2).await.unwrap());
    assert!(!f.repo.activate_staged_visa(2).await.unwrap());
    for node in [f.node, "fd5a:5052:3000::2".parse().unwrap()] {
        assert_eq!(
            f.asm
                .visa_mgr
                .get_pending_visa_ids_for_node(&node)
                .await
                .unwrap(),
            [2]
        );
    }
    let activated = f
        .asm
        .visa_mgr
        .get_visa_with_metadata_by_id(2)
        .await
        .unwrap()
        .unwrap();
    assert_eq!(activated.visa.expires, original.visa.expires);
    assert_eq!(
        activated.metadata.created_vinst,
        original.metadata.created_vinst
    );
    let restored = VisaRepo::new(f.db.clone(), 1).await.unwrap();
    assert_eq!(
        restored.get_node_visa_state(2, &f.node).unwrap(),
        crate::db::NodeVisaState::PendingInstall
    );
}

#[tokio::test]
async fn policy_verdicts_cannot_promote_unactivated_staged_visa() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    let generation = f.asm.policy_mgr.get_current().vinst();
    assert!(!f.repo.record_deny_verdict(2, generation + 1).await.unwrap());
    assert!(
        !f.repo
            .record_allow_verdict(2, generation + 2)
            .await
            .unwrap()
    );
    assert_staged(&f).await;
}

#[tokio::test]
async fn revoked_staged_visa_cannot_be_activated_or_revived_by_policy_allow() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    assert!(f.repo.mark_visa_revoked(2).await.unwrap());
    assert!(!f.repo.activate_staged_visa(2).await.unwrap());
    let generation = f.asm.policy_mgr.get_current().vinst();
    f.repo
        .record_allow_verdict(2, generation + 1)
        .await
        .unwrap();
    for node in [f.node, "fd5a:5052:3000::2".parse().unwrap()] {
        assert!(
            f.asm
                .visa_mgr
                .get_pending_visa_ids_for_node(&node)
                .await
                .unwrap()
                .is_empty()
        );
        assert_eq!(
            f.asm
                .visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap(),
            [2]
        );
    }
}

#[tokio::test]
async fn rejected_activation_removes_abandoned_issuance() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    assert!(f.repo.activate_staged_visa(2).await.is_err());
    assert_activation_removed(&f).await;
    f.db.set_set_ex_fault(crate::db::FaultMode::None);
    assert!(!f.repo.activate_staged_visa(2).await.unwrap());
}

#[tokio::test]
async fn missing_or_removed_staged_visa_cannot_be_activated() {
    let f = Fixture::new().await;
    assert!(!f.repo.activate_staged_visa(999).await.unwrap());
    stage_without_activation(&f).await;
    f.repo.remove_visa(2).await.unwrap();
    assert!(!f.repo.activate_staged_visa(2).await.unwrap());
}

#[tokio::test]
async fn expired_staged_visa_cannot_be_activated() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    tokio::time::pause();
    tokio::time::advance(Duration::from_secs(86_400)).await;
    assert!(!f.repo.activate_staged_visa(2).await.unwrap());
    assert!(
        f.asm
            .visa_mgr
            .get_pending_visa_ids_for_node(&f.node)
            .await
            .unwrap()
            .is_empty()
    );
}

#[tokio::test]
async fn ambiguous_activation_reply_reports_error_and_removes_durable_activation() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    f.db.set_set_ex_fault(crate::db::FaultMode::ApplyThenError);
    assert!(f.repo.activate_staged_visa(2).await.is_err());
    assert_activation_removed(&f).await;
}

#[tokio::test]
async fn activation_write_failure_in_request_never_returns_allow() {
    let f = Fixture::new().await;
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let mut activation = Box::pin(f.repo.hold_writes_for_test());
    assert!(futures::poll!(activation.as_mut()).is_pending());
    drop(guard);
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let activation_guard = activation.await;
    f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    drop(activation_guard);
    assert!(matches!(pending.await, Err(ServiceError::Store(_))));
    assert_activation_removed(&f).await;
}

async fn assert_activation_removed(f: &Fixture) {
    assert!(f.repo.list_visa_metadata().await.is_empty());
    assert!(f.db.get("visa:2").await.unwrap().is_none());
    let restored = VisaRepo::new(f.db.clone(), 1).await.unwrap();
    assert!(restored.list_visa_metadata().await.is_empty());
    for repo in [&f.repo, &restored] {
        assert!(
            repo.get_visa_ids_for_actors(&[
                "fd5a:5052:4000::a".parse().unwrap(),
                "fd5a:5052:4000::b".parse().unwrap(),
            ])
            .unwrap()
            .is_empty()
        );
        for node in [f.node, "fd5a:5052:3000::2".parse().unwrap()] {
            assert!(repo.get_all_visa_ids_for_node(&node).unwrap().is_empty());
            for state in [
                crate::db::NodeVisaState::Staged,
                crate::db::NodeVisaState::PendingInstall,
                crate::db::NodeVisaState::PendingRevoke,
            ] {
                assert!(
                    repo.get_visa_ids_for_node_by_state(&node, state)
                        .unwrap()
                        .is_empty()
                );
            }
        }
    }
    assert!(!f.repo.activate_staged_visa(2).await.unwrap());
    let generation = f.asm.policy_mgr.get_current().vinst();
    assert!(
        !f.repo
            .record_allow_verdict(2, generation + 1)
            .await
            .unwrap()
    );
}

#[tokio::test]
async fn rejected_activation_and_rejected_cleanup_preserve_staged_record_for_repair() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    f.db.set_del_fault(crate::db::FaultMode::Reject);
    assert!(f.repo.activate_staged_visa(2).await.is_err());
    assert_staged(&f).await;
    let restored = VisaRepo::new(f.db.clone(), 1).await.unwrap();
    assert_eq!(
        restored.get_node_visa_state(2, &f.node).unwrap(),
        crate::db::NodeVisaState::Staged
    );
    f.db.set_set_ex_fault(crate::db::FaultMode::None);
    f.db.set_del_fault(crate::db::FaultMode::None);
    assert!(f.repo.activate_staged_visa(2).await.unwrap());
}

#[tokio::test]
async fn ambiguous_activation_and_rejected_cleanup_remain_explicitly_repairable() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    f.db.set_set_ex_fault(crate::db::FaultMode::ApplyThenError);
    f.db.set_del_fault(crate::db::FaultMode::Reject);
    assert!(f.repo.activate_staged_visa(2).await.is_err());
    assert_staged(&f).await;
    let restored = VisaRepo::new(f.db.clone(), 1).await.unwrap();
    for node in [f.node, "fd5a:5052:3000::2".parse().unwrap()] {
        assert_eq!(
            restored.get_node_visa_state(2, &node).unwrap(),
            crate::db::NodeVisaState::PendingInstall
        );
    }
    f.db.set_set_ex_fault(crate::db::FaultMode::None);
    f.db.set_del_fault(crate::db::FaultMode::None);
    f.repo.mark_visa_revoked(2).await.unwrap();
    f.assert_revoked_everywhere(f.asm.policy_mgr.get_current().vinst())
        .await;
}

#[tokio::test]
async fn ambiguous_activation_and_ambiguous_cleanup_do_not_claim_memory_removal() {
    let f = Fixture::new().await;
    stage_without_activation(&f).await;
    f.db.set_set_ex_fault(crate::db::FaultMode::ApplyThenError);
    f.db.set_del_fault(crate::db::FaultMode::ApplyThenError);
    assert!(f.repo.activate_staged_visa(2).await.is_err());
    assert_staged(&f).await;
    assert!(f.db.get("visa:2").await.unwrap().is_none());
    assert!(
        VisaRepo::new(f.db.clone(), 1)
            .await
            .unwrap()
            .list_visa_metadata()
            .await
            .is_empty()
    );
    f.db.set_del_fault(crate::db::FaultMode::None);
    f.repo.remove_visa(2).await.unwrap();
    assert_activation_removed(&f).await;
}

#[tokio::test]
async fn ambiguous_activation_in_request_returns_error_and_cleans_up_before_restart() {
    let f = Fixture::new().await;
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let mut activation = Box::pin(f.repo.hold_writes_for_test());
    assert!(futures::poll!(activation.as_mut()).is_pending());
    drop(guard);
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let activation_guard = activation.await;
    f.db.set_set_ex_fault(crate::db::FaultMode::ApplyThenError);
    drop(activation_guard);
    assert!(matches!(pending.await, Err(ServiceError::Store(_))));
    assert_activation_removed(&f).await;
}

#[tokio::test]
async fn configuration_change_while_activation_is_queued_still_denies_and_revokes() {
    let f = Fixture::new().await;
    let generation = f.asm.policy_mgr.get_current().vinst();
    let guard = f.repo.hold_writes_for_test().await;
    let job = f.job();
    let mut pending = Box::pin(tokio::task::unconstrained(process_visa_request(
        f.asm.clone(),
        &job,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let mut activation = Box::pin(f.repo.hold_writes_for_test());
    assert!(futures::poll!(activation.as_mut()).is_pending());
    drop(guard);
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let activation_guard = activation.await;
    assert_staged(&f).await;
    f.asm.ts_mgr.update_services(vec![]);
    drop(activation_guard);
    assert!(matches!(pending.await.unwrap(), VisaDecision::Deny(_)));
    f.assert_revoked_everywhere(generation).await;
}
