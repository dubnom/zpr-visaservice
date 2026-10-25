use crate::test_helpers::make_policy_with_com_conditions;
use crate::visareq_worker::compute_expiration;
use libeval::actor::Actor;
use libeval::attribute::{AttributeSource, key};
use libeval::eval_result::{Direction, Hit};
use std::time::{Duration, SystemTime};

fn put(actor: &mut Actor, source: &str, key: &str, expires: SystemTime) {
    actor
        .add_attribute(
            AttributeSource::new(source)
                .builder(key)
                .expires(expires)
                .value("valid"),
        )
        .unwrap();
}

fn expiration(
    src: &Actor,
    dst: &Actor,
    client: &[&str],
    service: &[&str],
    direction: Direction,
) -> SystemTime {
    compute_expiration(
        src,
        dst,
        &make_policy_with_com_conditions(client, service),
        &Hit::new_no_signal(0, direction),
    )
    .unwrap()
}

#[test]
fn every_competing_gate_can_be_the_exact_minimum() {
    let gates = [
        (true, "user.condition"),
        (false, "device.condition"),
        (true, "user.identity"),
        (false, "device.identity"),
        (true, key::USER_AUTHORITY),
        (false, key::DEVICE_AUTHORITY),
        (false, key::SERVICES),
        (true, "zpr.source_freshness__source__company"),
        (false, "zpr.source_freshness__source__company"),
    ];
    let now = SystemTime::now();
    for winner in 0..gates.len() {
        let mut src = Actor::new();
        let mut dst = Actor::new();
        for (index, (source, key)) in gates.iter().enumerate() {
            let actor = if *source { &mut src } else { &mut dst };
            put(
                actor,
                "company",
                key,
                now + Duration::from_secs(if index == winner { 120 } else { 600 }),
            );
        }
        src.add_identity_key(0, "user.identity").unwrap();
        dst.add_identity_key(0, "device.identity").unwrap();
        assert_eq!(
            expiration(
                &src,
                &dst,
                &["user.condition"],
                &["device.condition"],
                Direction::Forward
            ),
            now + Duration::from_secs(120),
            "gate {winner}"
        );
    }
}

#[test]
fn reverse_hit_uses_client_and_service_keys_only_on_correct_endpoint() {
    let now = SystemTime::now();
    for direction in [Direction::Forward, Direction::Reverse] {
        let mut client = Actor::new();
        let mut provider = Actor::new();
        put(
            &mut client,
            "company",
            "user.client",
            now + Duration::from_secs(240),
        );
        put(
            &mut provider,
            "company",
            "user.provider",
            now + Duration::from_secs(180),
        );
        put(
            &mut provider,
            "company",
            key::SERVICES,
            now + Duration::from_secs(300),
        );
        put(
            &mut provider,
            "company",
            "user.client",
            now + Duration::from_secs(1),
        );
        put(
            &mut client,
            "company",
            "user.provider",
            now + Duration::from_secs(1),
        );
        put(
            &mut client,
            "company",
            key::SERVICES,
            now + Duration::from_secs(1),
        );
        let (src, dst) = if direction == Direction::Forward {
            (&client, &provider)
        } else {
            (&provider, &client)
        };
        assert_eq!(
            expiration(src, dst, &["user.client"], &["user.provider"], direction),
            now + Duration::from_secs(180)
        );
    }
}

#[test]
fn only_correctly_source_qualified_freshness_keys_bound_absence() {
    let now = SystemTime::now();
    let mut src = Actor::new();
    put(
        &mut src,
        "company",
        "zpr.source_freshness__source__lab",
        now + Duration::from_secs(1),
    );
    put(
        &mut src,
        "company",
        "user.zpr.source_freshness",
        now + Duration::from_secs(1),
    );
    put(
        &mut src,
        "company",
        "zpr.source_freshness__source__company",
        now + Duration::from_secs(120),
    );
    assert_eq!(
        expiration(
            &src,
            &Actor::new(),
            &["user.absent"],
            &[],
            Direction::Forward
        ),
        now + Duration::from_secs(120)
    );
}

#[test]
fn discrepant_legacy_and_qualified_markers_use_earlier_bound() {
    let now = SystemTime::now();
    for legacy_earlier in [true, false] {
        let mut actor = Actor::new();
        for (key, early) in [
            ("zpr.source_freshness", legacy_earlier),
            ("zpr.source_freshness__source__company", !legacy_earlier),
        ] {
            put(
                &mut actor,
                "company",
                key,
                now + Duration::from_secs(if early { 120 } else { 600 }),
            );
        }
        assert_eq!(
            expiration(&actor, &Actor::new(), &[], &[], Direction::Forward),
            now + Duration::from_secs(120)
        );
    }
}

#[test]
fn selected_policy_index_does_not_inherit_other_policy_deadlines() {
    let mut message = capnp::message::Builder::new_default();
    {
        let mut policy = message.init_root::<zpr::policy::v1::policy::Builder>();
        let mut policies = policy.reborrow().init_com_policies(2);
        for (index, name) in ["user.first", "user.second"].iter().enumerate() {
            policies
                .reborrow()
                .get(index as u32)
                .init_client_conds(1)
                .get(0)
                .set_key(name);
        }
    }
    let mut bytes = Vec::new();
    capnp::serialize::write_message(&mut bytes, &message).unwrap();
    let policy = libeval::policy::Policy::new_from_policy_bytes(bytes::Bytes::from(bytes)).unwrap();
    let now = SystemTime::now();
    let mut src = Actor::new();
    put(
        &mut src,
        "company",
        "user.first",
        now + Duration::from_secs(60),
    );
    put(
        &mut src,
        "company",
        "user.second",
        now + Duration::from_secs(300),
    );
    for (index, seconds) in [(0, 60), (1, 300)] {
        assert_eq!(
            compute_expiration(
                &src,
                &Actor::new(),
                &policy,
                &Hit::new_no_signal(index, Direction::Forward)
            )
            .unwrap(),
            now + Duration::from_secs(seconds)
        );
    }
}

#[test]
fn negative_condition_with_present_claim_still_contributes_its_deadline() {
    let mut message = capnp::message::Builder::new_default();
    {
        let mut policy = message.init_root::<zpr::policy::v1::policy::Builder>();
        let mut policies = policy.reborrow().init_com_policies(1);
        let mut conditions = policies.reborrow().get(0).init_client_conds(1);
        let mut condition = conditions.reborrow().get(0);
        condition.set_key("user.blocked");
        condition.set_op(zpr::policy::v1::AttrOp::Ne);
        condition.init_value(1).set(0, "true");
    }
    let mut bytes = Vec::new();
    capnp::serialize::write_message(&mut bytes, &message).unwrap();
    let policy = libeval::policy::Policy::new_from_policy_bytes(bytes::Bytes::from(bytes)).unwrap();
    let expires = SystemTime::now() + Duration::from_secs(120);
    let mut src = Actor::new();
    put(&mut src, "company", "user.blocked", expires);
    assert_eq!(
        compute_expiration(
            &src,
            &Actor::new(),
            &policy,
            &Hit::new_no_signal(0, Direction::Forward)
        )
        .unwrap(),
        expires
    );
}

#[test]
fn expired_supporting_bounds_are_never_clamped_into_the_future() {
    let expires = SystemTime::now() - Duration::from_secs(1);
    for key in [
        "user.condition",
        "zpr.source_freshness",
        key::DEVICE_AUTHORITY,
    ] {
        let mut actor = Actor::new();
        put(&mut actor, "company", key, expires);
        assert_eq!(
            expiration(
                &actor,
                &Actor::new(),
                &["user.condition"],
                &[],
                Direction::Forward
            ),
            expires
        );
    }
}

#[test]
fn removing_one_tied_observation_cannot_extend_the_other() {
    let expires = SystemTime::now() + Duration::from_secs(120);
    let mut src = Actor::new();
    let mut dst = Actor::new();
    put(
        &mut src,
        "company",
        "zpr.source_freshness__source__company",
        expires,
    );
    put(
        &mut dst,
        "lab",
        "zpr.source_freshness__source__lab",
        expires,
    );
    assert_eq!(
        expiration(&src, &dst, &[], &[], Direction::Forward),
        expires
    );
    src.remove_attribute("zpr.source_freshness__source__company");
    assert_eq!(
        expiration(&src, &dst, &[], &[], Direction::Forward),
        expires
    );
}

async fn issued() -> (
    std::sync::Arc<crate::assembly::Assembly>,
    std::net::IpAddr,
    zpr::vsapi_types::Visa,
) {
    let (asm, node, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
    let packet =
        zpr::vsapi_types::PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80)
            .unwrap();
    let crate::visareq_worker::VisaDecision::Allow(visa, _) =
        crate::visareq_worker::process_visa_request_for_test(asm.clone(), node, packet).await
    else {
        panic!("baseline issuance must allow");
    };
    (asm, node, visa)
}

#[tokio::test]
async fn recheck_compares_exact_deadlines_at_equality_and_one_nanosecond_either_side() {
    let (asm, _, visa) = issued().await;
    let metadata = asm
        .visa_mgr
        .get_visa_with_metadata_by_id(visa.issuer_id)
        .await
        .unwrap()
        .unwrap()
        .metadata;
    for endpoint in ["fd5a:5052:4000::a", "fd5a:5052:4000::b"] {
        let address = endpoint.parse().unwrap();
        for offset in [-1, 0, 1] {
            let expires = match offset {
                -1 => visa.expires - Duration::from_nanos(1),
                0 => visa.expires,
                _ => visa.expires + Duration::from_nanos(1),
            };
            let mut actor = asm
                .actor_mgr
                .get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .unwrap();
            put(
                &mut actor,
                "hr",
                "zpr.source_freshness__source__hr",
                expires,
            );
            asm.actor_mgr.update_actor(&actor).await.unwrap();
            let verdict = asm
                .visa_mgr
                .recheck_visa_allowed(
                    &asm,
                    &metadata,
                    &asm.policy_mgr.get_current_snapshot(),
                    visa.expires,
                )
                .await
                .unwrap();
            assert_eq!(
                verdict,
                if offset < 0 {
                    crate::visa_mgr::VisaRecheck::Revoke
                } else {
                    crate::visa_mgr::VisaRecheck::AllowSameRoute
                }
            );
        }
        let mut actor = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap()
            .unwrap();
        actor.remove_attribute("zpr.source_freshness__source__hr");
        asm.actor_mgr.update_actor(&actor).await.unwrap();
    }
}

#[tokio::test]
async fn repeated_attribute_sweeps_and_validity_recovery_never_cancel_queued_revoke() {
    use crate::visa_reconciler::{SweepReason, revalidate_visas};
    let (asm, node, visa) = issued().await;
    let address = "fd5a:5052:4000::a".parse().unwrap();
    let snapshot = asm.policy_mgr.get_current_snapshot();
    for seconds in [120, 120, 1800, 1800] {
        let mut actor = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap()
            .unwrap();
        put(
            &mut actor,
            "hr",
            "zpr.source_freshness__source__hr",
            SystemTime::now() + Duration::from_secs(seconds),
        );
        asm.actor_mgr.update_actor(&actor).await.unwrap();
        assert!(revalidate_visas(&asm, &snapshot, SweepReason::AttributeChange).await);
        assert_eq!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap(),
            [visa.issuer_id]
        );
        let stored = asm
            .visa_mgr
            .get_visa_with_metadata_by_id(visa.issuer_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(stored.visa.expires, visa.expires);
        assert_eq!(stored.metadata.checked_vinst, snapshot.vinst());
    }
}
