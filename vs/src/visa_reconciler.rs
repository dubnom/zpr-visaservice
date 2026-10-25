//! Visa reconciliation: re-evaluate live visas against a policy snapshot and queue
//! revocations for the ones that no longer hold.
//!
//! Split out of `event_mgr` because this is visa lifecycle policy, not event
//! coordination -- the operation spans actors, policy, topology, and visa
//! persistence. `event_mgr` still owns the *ordering* of a reconcile pass (refresh
//! attributes, then sweep); this module owns the sweep itself.

use std::sync::Arc;

use tracing::{debug, info, warn};

use crate::assembly::Assembly;
use crate::logging::targets::VISA;
use crate::policy_mgr::PolicySnapshot;
use crate::visa_mgr::VisaRecheck;

/// Why a visa sweep is running. The two reasons need different handling because
/// an attribute change does not move the policy generation, so the
/// `checked_vinst` gates that make the policy sweep idempotent (ie, do not do
/// anything if the `vinst` has not changed) would make an attribute sweep a
/// complete no-op.
#[derive(Clone, Copy, PartialEq, Eq, Debug)]
pub(crate) enum SweepReason {
    /// A new policy was installed.
    PolicyUpdate,
    /// A trusted service's attribute data changed.
    AttributeChange,
}

/// Sweep every live visa and re-evaluate it against the given policy snapshot.
/// Denied visas (and visas whose route moved) are marked `PendingRevoke` on all
/// their nodes so VSS housekeeping revokes them.
///
/// Visas whose actors can't be resolved are skipped (not revoked). A verdict is
/// only applied while `target_vinst` is still the live policy generation —
/// otherwise a newer policy sweep is coming and it will decide when it runs.
///
/// [SweepReason::PolicyUpdate] skips any visa already checked at/after
/// `target_vinst`, bumps `checked_vinst` on allow (canceling any older queued
/// revoke), and denies through the vinst-gated verdict recorder.
///
/// [SweepReason::AttributeChange] cannot use those gates: the policy generation has
/// not moved, so every visa looks "current". It checks every visa, revokes through
/// the ungated [VisaMgr::mark_visa_revoked], and does nothing at all on allow —
/// a revoke queued by an earlier attribute sweep stays queued, and the node simply
/// re-requests to get a fresh decision.
/// Returns false if any visa could not be checked or its verdict recorded, so
/// durable attribute reconciliation can retry without changing policy semantics.
pub(crate) async fn revalidate_visas(
    asm: &Arc<Assembly>,
    psnap: &PolicySnapshot,
    reason: SweepReason,
) -> bool {
    let target_vinst = psnap.vinst();
    let snapshot = asm.visa_mgr.list_visa_metadata().await;

    // Some numbers for logging purposes.
    let total = snapshot.len();
    let mut allowed = 0u32;
    let mut revoked = 0u32;
    let mut skipped_stale = 0u32;
    let mut skipped_unresolved = 0u32;
    let mut skipped_current = 0u32;
    let mut failed = 0u32;

    for (visa_id, metadata) in snapshot {
        // Only a policy sweep can skip on generation: an attribute change leaves the
        // generation alone, so this would skip everything.
        if reason == SweepReason::PolicyUpdate && metadata.checked_vinst >= target_vinst {
            skipped_current += 1;
            continue;
        }
        let expires = match asm.visa_mgr.get_visa_with_metadata_by_id(visa_id).await {
            Ok(Some(record)) => record.visa.expires,
            Ok(None) => continue,
            Err(error) => {
                failed += 1;
                warn!(target: VISA, "visa sweep: failed to read visa {visa_id} validity: {error}");
                continue;
            }
        };
        match asm
            .visa_mgr
            .recheck_visa_allowed(asm, &metadata, psnap, expires)
            .await
        {
            Ok(VisaRecheck::SkipUnresolvedActor) => {
                skipped_unresolved += 1;
                debug!(target: VISA, "visa sweep: visa {visa_id} actor unresolved, skipping");
            }
            Ok(VisaRecheck::AllowSameRoute) => {
                // An attribute sweep records nothing on allow: it must not bump
                // checked_vinst (that would make a later policy sweep skip the visa)
                // and it deliberately leaves any queued revoke in place.
                if reason == SweepReason::AttributeChange {
                    allowed += 1;
                    continue;
                }
                // Allowed. Only apply while target_vinst is still the live policy;
                // otherwise an older sweep could cancel a newer revoke verdict.
                if asm.policy_mgr.get_current_snapshot().vinst() != target_vinst {
                    skipped_stale += 1;
                    continue;
                }
                match asm
                    .visa_mgr
                    .record_allow_verdict(visa_id, target_vinst)
                    .await
                {
                    Ok(_) => allowed += 1,
                    Err(e) => {
                        failed += 1;
                        warn!(target: VISA, "visa sweep: failed to record allow for visa {visa_id}: {e}")
                    }
                }
            }
            Ok(VisaRecheck::Revoke) => {
                // Denied, or allowed but rerouted. Only apply while target_vinst
                // is still the live policy; otherwise a newer sweep is coming and
                // will make the decision.
                if asm.policy_mgr.get_current_snapshot().vinst() != target_vinst {
                    skipped_stale += 1;
                    continue;
                }
                // The verdict recorder is vinst-gated, so an attribute sweep has to
                // revoke through the ungated path.
                let res = match reason {
                    SweepReason::PolicyUpdate => {
                        asm.visa_mgr
                            .record_deny_verdict(visa_id, target_vinst)
                            .await
                    }
                    SweepReason::AttributeChange => asm.visa_mgr.mark_visa_revoked(visa_id).await,
                };
                match res {
                    Ok(_) => revoked += 1,
                    Err(e) => {
                        failed += 1;
                        warn!(target: VISA, "visa sweep: failed to record deny for visa {visa_id}: {e}")
                    }
                }
            }
            Err(e) => {
                failed += 1;
                warn!(target: VISA, "visa sweep: error re-checking visa {visa_id}: {e}");
            }
        }
    }

    // Note that this sweep just manipulates the desired state of the visas. The
    // actual revocations happen asynchronously in VSS housekeeping.
    info!(
        target: VISA,
        "visa sweep reason={reason:?} vinst={target_vinst}: total={total} allowed={allowed} revoked={revoked} skipped_stale={skipped_stale} skipped_unresolved={skipped_unresolved} skipped_current={skipped_current}"
    );
    failed == 0 && skipped_unresolved == 0 && skipped_stale == 0
}

#[cfg(test)]
mod tests {
    use super::*;

    use crate::assembly::tests::{make_policy, new_assembly_for_tests};
    use crate::test_helpers::{build_sweep_asm, create_sweep_visa};
    use std::net::IpAddr;

    #[tokio::test]
    async fn recheck_enforces_updated_grant_deadlines_without_extending_visas() {
        use libeval::attribute::{Attribute, AttributeSource, key};
        use std::time::{Duration, SystemTime};

        for source_endpoint in [true, false] {
            for changed_key in [
                "user.blocked__source__hr",
                key::CN,
                key::DEVICE_AUTHORITY,
                key::SERVICES,
                "zpr.source_freshness__source__hr",
                "user.unrelated__source__hr",
            ] {
                if source_endpoint && changed_key == key::SERVICES {
                    continue;
                }
                for shorter in [true, false] {
                    let (asm, node, _service) =
                        crate::test_helpers::build_ttl_refresh_asm(source_endpoint).await;
                    let addr: IpAddr = if source_endpoint {
                        "fd5a:5052:4000::a"
                    } else {
                        "fd5a:5052:4000::b"
                    }
                    .parse()
                    .unwrap();
                    let mut actor = asm
                        .actor_mgr
                        .get_actor_by_zpr_addr(&addr)
                        .await
                        .unwrap()
                        .unwrap();
                    actor.add_identity_key(0, key::CN).unwrap();
                    actor
                        .add_attribute(
                            Attribute::builder(key::DEVICE_AUTHORITY)
                                .expires_in(Duration::from_secs(3600))
                                .value("test-authority"),
                        )
                        .unwrap();
                    asm.actor_mgr.update_actor(&actor).await.unwrap();
                    let packet = zpr::vsapi_types::PacketDesc::new_tcp(
                        "fd5a:5052:4000::a",
                        "fd5a:5052:4000::b",
                        1234,
                        80,
                    )
                    .unwrap();
                    let decision = crate::visareq_worker::process_visa_request_for_test(
                        asm.clone(),
                        node,
                        packet,
                    )
                    .await;
                    let crate::visareq_worker::VisaDecision::Allow(visa, _) = decision else {
                        panic!("baseline request must allow");
                    };
                    let original_expiration = visa.expires;
                    let mut actor = asm
                        .actor_mgr
                        .get_actor_by_zpr_addr(&addr)
                        .await
                        .unwrap()
                        .unwrap();
                    let expires = if shorter {
                        SystemTime::now() + Duration::from_secs(120)
                    } else {
                        original_expiration + Duration::from_secs(600)
                    };
                    assert!(original_expiration > SystemTime::now() + Duration::from_secs(120));
                    let attr = match changed_key {
                        key::CN | key::DEVICE_AUTHORITY | key::SERVICES => {
                            let old = actor.get_attribute(changed_key).unwrap();
                            Attribute::builder(changed_key)
                                .expires(expires)
                                .values(old.get_value())
                        }
                        "zpr.source_freshness__source__hr" => {
                            AttributeSource::new(crate::test_helpers::TS_SOURCE)
                                .builder(changed_key)
                                .expires(expires)
                                .values(Vec::<String>::new())
                        }
                        _ => AttributeSource::new(crate::test_helpers::TS_SOURCE)
                            .builder(changed_key)
                            .expires(expires)
                            .value("false"),
                    };
                    actor.add_attribute(attr).unwrap();
                    asm.actor_mgr.update_actor(&actor).await.unwrap();

                    let psnap = asm.policy_mgr.get_current_snapshot();
                    assert!(revalidate_visas(&asm, &psnap, SweepReason::AttributeChange).await);
                    let revoked = asm
                        .visa_mgr
                        .get_pending_revoke_visa_ids_for_node(&node)
                        .await
                        .unwrap();
                    if shorter && changed_key != "user.unrelated__source__hr" {
                        assert_eq!(
                            revoked,
                            vec![visa.issuer_id],
                            "{source_endpoint}/{changed_key}"
                        );
                    } else {
                        assert!(revoked.is_empty(), "{source_endpoint}/{changed_key}");
                    }
                    let stored = asm
                        .visa_mgr
                        .get_visa_with_metadata_by_id(visa.issuer_id)
                        .await
                        .unwrap()
                        .unwrap();
                    assert_eq!(stored.visa.expires, original_expiration);
                }
            }
        }
    }

    #[tokio::test]
    async fn source_validity_shortening_revokes_but_longer_validity_does_not() {
        for seconds in [120, 1200] {
            let (asm, node, _service) = crate::test_helpers::build_ttl_refresh_asm(true).await;
            let peer: IpAddr = "fd5a:5052:3000::2".parse().unwrap();
            let route = asm.topo_mgr.get_best_route(&node, &peer).unwrap();
            let packet = zpr::vsapi_types::PacketDesc::new_tcp(
                "fd5a:5052:4000::a",
                "fd5a:5052:4000::b",
                1234,
                80,
            )
            .unwrap();
            let psnap = asm.policy_mgr.get_current_snapshot();
            let visa = asm
                .visa_mgr
                .create_visa(
                    &asm,
                    &node,
                    &packet,
                    &libeval::eval_result::Hit::new_no_signal(
                        0,
                        libeval::eval_result::Direction::Forward,
                    ),
                    &route,
                    "",
                    2,
                    psnap.vinst(),
                    std::time::SystemTime::now() + std::time::Duration::from_secs(600),
                )
                .await
                .unwrap();
            assert!(matches!(
                asm.visa_mgr
                    .recheck_visa_allowed(&asm, &visa.metadata, &psnap, visa.visa.expires)
                    .await
                    .unwrap(),
                crate::visa_mgr::VisaRecheck::AllowSameRoute
            ));
            let addr: IpAddr = "fd5a:5052:4000::a".parse().unwrap();
            let mut actor = asm
                .actor_mgr
                .get_actor_by_zpr_addr(&addr)
                .await
                .unwrap()
                .unwrap();
            actor
                .add_attribute(
                    libeval::attribute::AttributeSource::new(crate::test_helpers::TS_SOURCE)
                        .builder("zpr.source_freshness__source__hr")
                        .expires_in(std::time::Duration::from_secs(seconds))
                        .values(Vec::<String>::new()),
                )
                .unwrap();
            asm.actor_mgr.update_actor(&actor).await.unwrap();
            assert!(revalidate_visas(&asm, &psnap, SweepReason::AttributeChange).await);
            let revoked = asm
                .visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap();
            if seconds == 120 {
                assert_eq!(revoked, vec![visa.visa.issuer_id]);
            } else {
                assert!(revoked.is_empty());
            }
        }
    }

    #[tokio::test]
    async fn indeterminate_refresh_revokes_even_when_absence_policy_would_allow() {
        for (expired_source, expired_identity) in
            [(true, false), (false, false), (true, true), (false, true)]
        {
            let (asm, node, service) =
                crate::test_helpers::build_ttl_refresh_asm(expired_source).await;
            let peer: IpAddr = "fd5a:5052:3000::2".parse().unwrap();
            let route = asm.topo_mgr.get_best_route(&node, &peer).unwrap();
            let packet = zpr::vsapi_types::PacketDesc::new_tcp(
                "fd5a:5052:4000::a",
                "fd5a:5052:4000::b",
                1234,
                80,
            )
            .unwrap();
            let psnap = asm.policy_mgr.get_current_snapshot();
            let visa = asm
                .visa_mgr
                .create_visa(
                    &asm,
                    &node,
                    &packet,
                    &libeval::eval_result::Hit::new_no_signal(
                        0,
                        libeval::eval_result::Direction::Forward,
                    ),
                    &route,
                    "",
                    2,
                    psnap.vinst(),
                    std::time::SystemTime::now() + std::time::Duration::from_secs(600),
                )
                .await
                .unwrap();
            // With the source available, a missing "blocked" key genuinely permits
            // this flow and the recorded route is unchanged.
            assert!(matches!(
                asm.visa_mgr
                    .recheck_visa_allowed(&asm, &visa.metadata, &psnap, visa.visa.expires)
                    .await
                    .unwrap(),
                crate::visa_mgr::VisaRecheck::AllowSameRoute
            ));
            // Re-expire the source after the successful baseline recheck.
            let address = if expired_source {
                "fd5a:5052:4000::a"
            } else {
                "fd5a:5052:4000::b"
            };
            let addr: IpAddr = address.parse().unwrap();
            let mut actor = asm
                .actor_mgr
                .get_actor_by_zpr_addr(&addr)
                .await
                .unwrap()
                .unwrap();
            let expires = std::time::SystemTime::now() - std::time::Duration::from_secs(1);
            let expired_attr = if expired_identity {
                libeval::attribute::Attribute::builder(libeval::attribute::key::CN)
                    .expires(expires)
                    .value(actor.get_cn().unwrap())
            } else {
                libeval::attribute::AttributeSource::new(crate::test_helpers::TS_SOURCE)
                    .builder(format!(
                        "{}__source__{}",
                        crate::test_helpers::TS_KEY,
                        crate::test_helpers::TS_SOURCE
                    ))
                    .expires(expires)
                    .value("engineering")
            };
            actor.add_attribute(expired_attr).unwrap();
            asm.actor_mgr.update_actor(&actor).await.unwrap();
            service
                .fail
                .store(!expired_identity, std::sync::atomic::Ordering::SeqCst);

            assert!(!crate::event_mgr::handle_trusted_service_change(&asm).await);
            assert_eq!(
                asm.visa_mgr
                    .get_pending_revoke_visa_ids_for_node(&node)
                    .await
                    .unwrap(),
                vec![visa.visa.issuer_id]
            );
        }
    }

    /// A denied visa is marked PendingRevoke on its holder node (target policy
    /// still live). Uses the no-route assembly so eval falls out as a deny.
    #[tokio::test]
    async fn test_sweep_denied_visa_marked_pending_revoke() {
        let (asm, node_a) = build_sweep_asm(false).await;
        let id = create_sweep_visa(&asm, &node_a, 0).await;

        let psnap = asm.policy_mgr.get_current_snapshot();
        assert!(
            psnap.vinst() > 0,
            "target must exceed the visa's checked_vinst"
        );
        revalidate_visas(&asm, &psnap, SweepReason::PolicyUpdate).await;

        let revoke_ids = asm
            .visa_mgr
            .get_pending_revoke_visa_ids_for_node(&node_a)
            .await
            .unwrap();
        assert_eq!(revoke_ids, vec![id]);
    }

    /// A visa already checked at/after target_vinst is skipped before re-eval:
    /// even on the deny (no-route) assembly it is NOT marked PendingRevoke.
    #[tokio::test]
    async fn test_sweep_skips_already_checked() {
        let (asm, node_a) = build_sweep_asm(false).await;
        let target = asm.policy_mgr.get_current_snapshot().vinst();
        // checked_vinst == target, so the guard skips before recheck.
        let id = create_sweep_visa(&asm, &node_a, target).await;

        let psnap = asm.policy_mgr.get_current_snapshot();
        revalidate_visas(&asm, &psnap, SweepReason::PolicyUpdate).await;

        assert!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node_a)
                .await
                .unwrap()
                .is_empty(),
            "already-checked visa must not be re-evaluated/revoked"
        );
        let pending = asm
            .visa_mgr
            .get_pending_visa_ids_for_node(&node_a)
            .await
            .unwrap();
        assert_eq!(pending, vec![id]);
    }

    /// An unresolvable actor leaves the visa untouched (skipped, not revoked).
    #[tokio::test]
    async fn test_sweep_unresolved_actor_leaves_visa_untouched() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let node: IpAddr = "fd5a:5052:3000::1".parse().unwrap();
        // Five-tuple addrs (::4000::a/b) have no actors in the DB → unresolved.
        let id = create_sweep_visa(&asm, &node, 0).await;

        let psnap = asm.policy_mgr.get_current_snapshot();
        revalidate_visas(&asm, &psnap, SweepReason::PolicyUpdate).await;

        // Still held as PendingInstall (untouched); nothing marked for revoke.
        let pending = asm
            .visa_mgr
            .get_pending_visa_ids_for_node(&node)
            .await
            .unwrap();
        assert_eq!(pending, vec![id]);
        assert!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap()
                .is_empty()
        );
    }

    /// A deny whose target vinst is no longer the live policy generation is
    /// skipped (a newer sweep is coming): no revoke is marked.
    #[tokio::test]
    async fn test_sweep_denied_but_stale_target_skips() {
        let (asm, node_a) = build_sweep_asm(false).await;
        let id = create_sweep_visa(&asm, &node_a, 0).await;

        // Snapshot the current (stale) generation, then bump the live policy so
        // get_current_snapshot().vinst() moves past the snapshot's vinst.
        let stale_psnap = asm.policy_mgr.get_current_snapshot();
        asm.policy_mgr
            .update_policy_from_container_bytes(make_policy("2024-01-02T00:00:00Z", 2, Some("m")))
            .await
            .unwrap();
        assert!(asm.policy_mgr.get_current_snapshot().vinst() > stale_psnap.vinst());

        revalidate_visas(&asm, &stale_psnap, SweepReason::PolicyUpdate).await;

        // No revoke marked; the visa stays PendingInstall.
        assert!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node_a)
                .await
                .unwrap()
                .is_empty()
        );
        let pending = asm
            .visa_mgr
            .get_pending_visa_ids_for_node(&node_a)
            .await
            .unwrap();
        assert_eq!(pending, vec![id]);
    }

    /// The core case: an attribute sweep re-checks (and revokes) a visa whose
    /// `checked_vinst` already equals the live generation -- exactly what the policy
    /// sweep skips, at both the sweep and the store layer.
    #[tokio::test]
    async fn test_attr_sweep_revokes_visa_at_current_vinst() {
        let (asm, node_a) = build_sweep_asm(false).await;
        let target = asm.policy_mgr.get_current_snapshot().vinst();
        let id = create_sweep_visa(&asm, &node_a, target).await;

        let psnap = asm.policy_mgr.get_current_snapshot();
        revalidate_visas(&asm, &psnap, SweepReason::AttributeChange).await;

        let revoke_ids = asm
            .visa_mgr
            .get_pending_revoke_visa_ids_for_node(&node_a)
            .await
            .unwrap();
        assert_eq!(revoke_ids, vec![id]);

        // The attribute sweep must not touch the policy generation it was checked at.
        let md = asm.visa_mgr.list_visa_metadata().await;
        assert_eq!(md.len(), 1);
        assert_eq!(md[0].1.checked_vinst, target);
    }

    /// An attribute sweep holding a snapshot older than the live policy applies nothing:
    /// a policy sweep is already coming and will decide.
    #[tokio::test]
    async fn test_attr_sweep_stale_target_skips() {
        let (asm, node_a) = build_sweep_asm(false).await;
        let id = create_sweep_visa(&asm, &node_a, 0).await;

        let stale_psnap = asm.policy_mgr.get_current_snapshot();
        asm.policy_mgr
            .update_policy_from_container_bytes(make_policy("2024-01-02T00:00:00Z", 2, Some("m")))
            .await
            .unwrap();

        revalidate_visas(&asm, &stale_psnap, SweepReason::AttributeChange).await;

        assert!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node_a)
                .await
                .unwrap()
                .is_empty()
        );
        assert_eq!(
            asm.visa_mgr
                .get_pending_visa_ids_for_node(&node_a)
                .await
                .unwrap(),
            vec![id]
        );
    }

    /// An unresolvable actor is skipped by the attribute sweep too, never revoked.
    #[tokio::test]
    async fn test_attr_sweep_unresolved_actor_leaves_visa_untouched() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let node: IpAddr = "fd5a:5052:3000::1".parse().unwrap();
        let id = create_sweep_visa(&asm, &node, 0).await;

        let psnap = asm.policy_mgr.get_current_snapshot();
        revalidate_visas(&asm, &psnap, SweepReason::AttributeChange).await;

        assert_eq!(
            asm.visa_mgr
                .get_pending_visa_ids_for_node(&node)
                .await
                .unwrap(),
            vec![id]
        );
        assert!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap()
                .is_empty()
        );
    }
}
