use super::*;
use crate::trusted_services::{SOURCE_FRESHNESS_KEY, TrustedServiceInterface};
use libeval::attribute::{AttributeSource, key};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime};

struct Source {
    name: &'static str,
    attrs: Mutex<Vec<Attribute>>,
    calls: Mutex<Vec<Vec<(String, String)>>>,
    fail: AtomicBool,
    revision: AtomicU64,
}

impl Source {
    fn new(name: &'static str, revision: u64, attrs: Vec<Attribute>) -> Arc<Self> {
        Arc::new(Self {
            name,
            attrs: Mutex::new(attrs),
            calls: Mutex::new(vec![]),
            fail: AtomicBool::new(false),
            revision: AtomicU64::new(revision),
        })
    }
    fn calls(&self) -> usize {
        self.calls.lock().unwrap().len()
    }
}

#[async_trait::async_trait]
impl TrustedServiceInterface for Source {
    async fn get_attributes_for_actor(
        &self,
        identities: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        self.calls.lock().unwrap().push(identities.to_vec());
        if self.fail.load(Ordering::Relaxed) {
            Err(ServiceError::AttributesIndeterminate(format!(
                "{}: unavailable",
                self.name
            )))
        } else {
            Ok(self.attrs.lock().unwrap().clone())
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

fn deadline(seconds: u64) -> SystemTime {
    SystemTime::now() + Duration::from_secs(seconds)
}

fn claim(source: &str, name: &str, value: &str, expires: SystemTime) -> Attribute {
    AttributeSource::new(source)
        .builder(name)
        .expires(expires)
        .value(value)
}

fn actor() -> Actor {
    let mut actor = Actor::new();
    actor
        .add_attribute(claim(
            "authenticated",
            key::CN,
            "device.example",
            deadline(3600),
        ))
        .unwrap();
    actor
        .add_attribute(Attribute::builder(key::ZPR_ADDR).value("fd5a:5052::123"))
        .unwrap();
    actor
}

async fn refresh(manager: &TrustedServicesMgr, actor: &mut Actor) -> RefreshOutcome {
    refresh_expired_attributes(manager, &[key::CN], actor).await
}

fn commit(manager: &TrustedServicesMgr, actor: &Actor, outcome: &RefreshOutcome) {
    outcome.commit_revisions(manager, actor.get_zpr_addr().unwrap());
}

fn key_for(name: &str, source: &str) -> String {
    format!("{name}__source__{source}")
}

struct PersistenceFixture {
    asm: Assembly,
    db: Arc<crate::db::FakeDb>,
    source: Arc<Source>,
    actor: Actor,
}

#[tokio::test]
async fn signed_bas_claims_survive_directory_refresh_without_extending_authentication() {
    use libeval::actor::{DeviceAuthentication, DeviceAuthority};

    let mut f = PersistenceFixture::new().await;
    let expiration = deadline(300);
    let identity = DeviceAuthentication::new(
        DeviceAuthority::AuthenticationService {
            service_id: "velocity-enrollment-bas".into(),
            issuer: "zpr/bas".into(),
        },
        "device.example".into(),
        uuid::Uuid::new_v4().to_string(),
        expiration,
    )
    .unwrap();
    f.actor.set_device_authentication(identity.clone()).unwrap();
    let claims = crate::trusted_services::qualify_source_attributes(
        vec![claim(
            "velocity-enrollment-bas",
            "device.enrollment.asset_id",
            "device.example",
            expiration,
        )],
        "velocity-enrollment-bas",
        true,
    );
    for attribute in &claims {
        f.actor.add_attribute(attribute.clone()).unwrap();
    }
    crate::db::ActorRepo::new(f.db.clone())
        .update_authenticated_actor(&f.actor)
        .await
        .unwrap();

    for remove_directory in [false, true] {
        if remove_directory {
            f.asm.ts_mgr.update_services(vec![]);
        }
        let mut reloaded = f.stored().await;
        refresh_and_persist_actor(&f.asm, &mut reloaded)
            .await
            .unwrap();
        for attribute in &claims {
            let retained = reloaded
                .get_attribute(attribute.get_key())
                .unwrap_or_else(|| {
                    panic!("authentication claim was pruned: {}", attribute.get_key())
                });
            assert_eq!(retained.get_value(), attribute.get_value());
            assert_eq!(retained.get_source(), "velocity-enrollment-bas");
            assert_eq!(retained.get_expires(), expiration);
        }
        assert_eq!(reloaded.device_authentication(), Some(&identity));
        if remove_directory {
            assert!(reloaded.get_attribute("user.old").is_none());
        }
    }
}

#[tokio::test]
async fn bas_source_stamp_without_matching_authentication_is_still_pruned() {
    use libeval::actor::{DeviceAuthentication, DeviceAuthority};

    for authenticated_source in [None, Some("another-bas")] {
        let mut actor = actor();
        if let Some(source) = authenticated_source {
            actor
                .set_device_authentication(
                    DeviceAuthentication::new(
                        DeviceAuthority::AuthenticationService {
                            service_id: source.into(),
                            issuer: "zpr/bas".into(),
                        },
                        "device.example".into(),
                        uuid::Uuid::new_v4().to_string(),
                        deadline(300),
                    )
                    .unwrap(),
                )
                .unwrap();
        }

        for attribute in crate::trusted_services::qualify_source_attributes(
            vec![claim("bas", "device.asset", "unmatched", deadline(300))],
            "bas",
            true,
        ) {
            actor.add_attribute(attribute).unwrap();
        }
        let manager = TrustedServicesMgr::new();
        let outcome = refresh(&manager, &mut actor).await;
        assert!(outcome.changed);
        assert!(actor.get_attribute("device.asset").is_none());
        assert!(
            actor
                .get_attribute(&key_for("device.asset", "bas"))
                .is_none()
        );
    }
}

#[tokio::test]
async fn expired_bas_authentication_or_claims_are_indeterminate_not_known_absence() {
    use libeval::actor::{DeviceAuthentication, DeviceAuthority};

    for session_expired in [false, true] {
        let mut actor = actor();
        let past = SystemTime::now() - Duration::from_secs(1);
        actor
            .set_device_authentication(
                DeviceAuthentication::new(
                    DeviceAuthority::AuthenticationService {
                        service_id: "bas".into(),
                        issuer: "zpr/bas".into(),
                    },
                    "device.example".into(),
                    uuid::Uuid::new_v4().to_string(),
                    if session_expired { past } else { deadline(300) },
                )
                .unwrap(),
            )
            .unwrap();
        for attribute in crate::trusted_services::qualify_source_attributes(
            vec![claim(
                "bas",
                "device.asset",
                "verified",
                if session_expired { deadline(300) } else { past },
            )],
            "bas",
            true,
        ) {
            actor.add_attribute(attribute).unwrap();
        }
        let outcome = refresh(&TrustedServicesMgr::new(), &mut actor).await;
        assert!(!outcome.indeterminate.is_empty());
        assert!(!outcome.changed);
        assert!(actor.get_attribute("device.asset").is_some());
        assert!(
            actor
                .get_attribute(&key_for("device.asset", "bas"))
                .is_some()
        );
    }
}

#[tokio::test]
async fn session_guard_refresh_conflict_does_not_certify_stale_observations() {
    use libeval::actor::{DeviceAuthentication, DeviceAuthority};

    let mut f = PersistenceFixture::new().await;
    let identity = DeviceAuthentication::new(
        DeviceAuthority::Bootstrap {
            visa_service: "vs.zpr/test".into(),
        },
        "verified-device".into(),
        uuid::Uuid::new_v4().to_string(),
        deadline(600),
    )
    .unwrap();
    f.actor.set_device_authentication(identity).unwrap();
    crate::db::ActorRepo::new(f.db.clone())
        .update_authenticated_actor(&f.actor)
        .await
        .unwrap();
    let replacement = DeviceAuthentication::new(
        DeviceAuthority::Bootstrap {
            visa_service: "vs.zpr/test".into(),
        },
        "verified-device".into(),
        uuid::Uuid::new_v4().to_string(),
        deadline(600),
    )
    .unwrap();
    let gate = f.db.hold_pipelines_for_test().await;
    let mut stale = f.actor.clone();
    let refresh = tokio::task::unconstrained(refresh_and_persist_actor(&f.asm, &mut stale));
    tokio::pin!(refresh);
    assert!(futures::poll!(&mut refresh).is_pending());
    crate::db::DbConnection::hset(
        f.db.as_ref(),
        "actor:fd5a-5052--123",
        "device_authentication",
        &serde_json::to_string(&replacement).unwrap(),
    )
    .await
    .unwrap();
    drop(gate);
    assert!(matches!(
        refresh.await,
        Err(ServiceError::Store(crate::error::StoreError::Conflict(_)))
    ));
    f.assert_retry_pending();
    let stored = f.stored().await;
    assert_eq!(stored.device_authentication(), Some(&replacement));
    assert!(stored.get_attribute("user.new").is_none());
    assert!(stored.get_attribute("user.old").is_some());
}

#[tokio::test]
async fn device_authentication_survives_provider_refresh_pruning_and_reload() {
    use libeval::actor::{DeviceAuthentication, DeviceAuthority};

    let mut f = PersistenceFixture::new().await;
    let identity = DeviceAuthentication::new(
        DeviceAuthority::AuthenticationService {
            service_id: "bas".into(),
            issuer: "zpr/bas".into(),
        },
        "verified-device".into(),
        uuid::Uuid::new_v4().to_string(),
        deadline(600),
    )
    .unwrap();
    f.actor.set_device_authentication(identity.clone()).unwrap();
    crate::db::ActorRepo::new(f.db.clone())
        .update_authenticated_actor(&f.actor)
        .await
        .unwrap();
    f.source.attrs.lock().unwrap().push(claim(
        "company",
        "user.device_authentication",
        "untrusted-human-subject",
        deadline(300),
    ));
    refresh_and_persist_actor(&f.asm, &mut f.actor)
        .await
        .unwrap();
    assert_eq!(f.actor.device_authentication(), Some(&identity));
    assert_eq!(f.stored().await.device_authentication(), Some(&identity));
    f.source.attrs.lock().unwrap().clear();
    f.source.flush().await.unwrap();
    let mut reloaded = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut reloaded)
        .await
        .unwrap();
    assert!(
        reloaded
            .get_attribute("user.device_authentication")
            .is_none()
    );
    assert_eq!(reloaded.device_authentication(), Some(&identity));
    assert_eq!(f.stored().await.device_authentication(), Some(&identity));
}

impl PersistenceFixture {
    async fn new() -> Self {
        let mut asm = crate::assembly::tests::new_assembly_for_tests(None).await;
        let db = Arc::new(crate::db::FakeDb::new());
        asm.actor_mgr = Arc::new(crate::actor_mgr::ActorMgr::new(
            crate::db::ActorRepo::new(db.clone()),
            crate::db::NodeRepo::new(db.clone()),
            asm.counters.clone(),
        ));
        let source = Source::new(
            "company",
            1,
            vec![claim("company", "user.old", "old", deadline(600))],
        );
        asm.ts_mgr.update_services(vec![source.clone()]);
        let mut actor =
            crate::test_helpers::make_adapter_actor_defexp("fd5a:5052::123", "device.example");
        asm.actor_mgr
            .hack_add_adapter_no_node(&actor)
            .await
            .unwrap();
        refresh_and_persist_actor(&asm, &mut actor).await.unwrap();
        actor.add_identity_key(usize::MAX, "user.old").unwrap();
        asm.actor_mgr.update_actor(&actor).await.unwrap();
        *source.attrs.lock().unwrap() = vec![claim("company", "user.new", "new", deadline(300))];
        source.flush().await.unwrap();
        Self {
            asm,
            db,
            source,
            actor,
        }
    }

    fn address(&self) -> IpAddr {
        *self.actor.get_zpr_addr().unwrap()
    }

    async fn stored(&self) -> Actor {
        self.asm
            .actor_mgr
            .get_actor_by_zpr_addr(&self.address())
            .await
            .unwrap()
            .unwrap()
    }

    fn assert_retry_pending(&self) {
        assert_eq!(
            self.asm.ts_mgr.stale_sources_for_actor(&self.address()),
            vec![("company".into(), 2)]
        );
    }

    fn reject_write(&self) {
        self.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    }

    fn repair_write(&self) {
        self.db.set_set_ex_fault(crate::db::FaultMode::None);
        self.db.set_del_fault(crate::db::FaultMode::None);
    }
}

#[tokio::test]
async fn removed_source_named_like_builtin_issuer_preserves_builtin_claims() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    actor
        .add_attribute(claim("zpr", "device.zpr.role", "adapter", deadline(600)))
        .unwrap();
    for name in ["user.old", "user.old__source__zpr"] {
        actor
            .add_attribute(claim("zpr", name, "old", deadline(600)))
            .unwrap();
    }
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.changed);
    assert!(actor.get_attribute("user.old").is_none());
    assert!(actor.get_attribute("user.old__source__zpr").is_none());
    assert!(actor.get_zpr_addr().is_some());
    assert!(actor.get_attribute("device.zpr.role").is_some());
}

#[tokio::test]
async fn mismatched_source_suffix_does_not_establish_ownership() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    actor
        .add_attribute(claim(
            "authenticated",
            "user.old__source__company",
            "old",
            deadline(600),
        ))
        .unwrap();
    let outcome = refresh(&manager, &mut actor).await;
    assert!(!outcome.changed);
    assert!(actor.get_attribute("user.old__source__company").is_some());
}

#[tokio::test]
async fn rejected_removal_write_preserves_revision_until_cleanup_is_committed() {
    let mut f = PersistenceFixture::new().await;
    f.asm.ts_mgr.update_services(vec![]);
    f.reject_write();
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .is_err()
    );
    f.source.revision.store(1, Ordering::Relaxed);
    f.asm.ts_mgr.update_services(vec![f.source.clone()]);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
    assert!(f.stored().await.get_attribute("user.old").is_some());
}

#[tokio::test]
async fn removed_empty_source_forgets_record_without_unnecessary_actor_write() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let source = Source::new("company", 1, vec![]);
    manager.update_services(vec![source.clone()]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    manager.update_services(vec![]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(!outcome.changed);
    commit(&manager, &actor, &outcome);
    manager.update_services(vec![source]);
    assert_eq!(
        manager.stale_sources_for_actor(actor.get_zpr_addr().unwrap()),
        vec![("company".into(), 1)]
    );
}

#[tokio::test]
async fn persisted_removal_makes_following_refresh_a_noop() {
    let mut f = PersistenceFixture::new().await;
    f.asm.ts_mgr.update_services(vec![]);
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .unwrap()
    );
    let mut stored = f.stored().await;
    f.reject_write();
    assert!(
        !refresh_and_persist_actor(&f.asm, &mut stored)
            .await
            .unwrap()
    );
}

#[tokio::test]
async fn restarted_manager_prunes_loaded_actor_without_revision_history() {
    let mut f = PersistenceFixture::new().await;
    f.asm.ts_mgr = Arc::new(TrustedServicesMgr::new());
    let mut loaded = f.stored().await;
    assert!(
        refresh_and_persist_actor(&f.asm, &mut loaded)
            .await
            .unwrap()
    );
    let stored = f.stored().await;
    assert!(
        !stored
            .attrs_iter()
            .any(|attr| attr.get_source() == "company")
    );
    assert_eq!(stored.get_cn(), Some("device.example"));
}

#[tokio::test]
async fn removed_required_source_claim_is_persisted_before_existing_visa_recheck() {
    use zpr::policy::v1 as wire;
    let (asm, node) = crate::test_helpers::build_sweep_asm(true).await;
    let mut message = capnp::message::Builder::new_default();
    {
        let mut policy = message.init_root::<wire::policy::Builder>();
        policy.set_created("2026-10-24T00:00:00Z");
        policy.set_version(2);
        let mut coms = policy.reborrow().init_com_policies(1);
        let mut com = coms.reborrow().get(0);
        com.set_service_id("web");
        com.set_allow(true);
        let mut scopes = com.reborrow().init_scope(1);
        let mut scope = scopes.reborrow().get(0);
        scope.set_protocol(6);
        scope.init_port().set_port_num(80);
        let mut conditions = com.init_client_conds(1);
        let mut condition = conditions.reborrow().get(0);
        condition.set_key("user.old__source__company");
        condition.set_op(wire::AttrOp::Eq);
        condition.init_value(1).set(0, "old");
    }
    let mut bytes = vec![];
    capnp::serialize::write_message(&mut bytes, &message).unwrap();
    asm.policy_mgr
        .update_policy_from_container_bytes(crate::test_helpers::make_container_bytes(
            crate::config::POLICY_MIN_COMPILER_MAJOR,
            crate::config::POLICY_MIN_COMPILER_MINOR,
            crate::config::POLICY_MIN_COMPILER_PATCH,
            &bytes,
        ))
        .await
        .unwrap();
    let destination: IpAddr = "fd5a:5052:4000::b".parse().unwrap();
    let mut dst = asm
        .actor_mgr
        .get_actor_by_zpr_addr(&destination)
        .await
        .unwrap()
        .unwrap();
    dst.add_attribute(Attribute::builder(key::SERVICES).value("web"))
        .unwrap();
    asm.actor_mgr.update_actor(&dst).await.unwrap();
    asm.ts_mgr.update_services(vec![Source::new(
        "company",
        1,
        vec![claim("company", "user.old", "old", deadline(600))],
    )]);
    let packet =
        zpr::vsapi_types::PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80)
            .unwrap();
    let crate::visareq_worker::VisaDecision::Allow(visa, _) =
        crate::visareq_worker::process_visa_request_for_test(asm.clone(), node, packet).await
    else {
        panic!("baseline must allow with required source claim");
    };
    assert!(
        asm.visa_mgr
            .get_pending_revoke_visa_ids_for_node(&node)
            .await
            .unwrap()
            .is_empty()
    );
    asm.ts_mgr.update_services(vec![]);
    assert!(crate::event_mgr::handle_trusted_service_change(&asm).await);
    for address in ["fd5a:5052:4000::a".parse().unwrap(), destination] {
        let stored = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap()
            .unwrap();
        assert!(
            !stored
                .attrs_iter()
                .any(|attr| attr.get_source() == "company")
        );
    }
    assert_eq!(
        asm.visa_mgr
            .get_pending_revoke_visa_ids_for_node(&node)
            .await
            .unwrap(),
        [visa.issuer_id]
    );
}

#[tokio::test]
async fn removed_source_prunes_unexpired_claims_authority_and_identity_keys() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let source = Source::new(
        "company",
        1,
        vec![
            claim("company", "user.old", "old", deadline(600)),
            claim("company", SOURCE_FRESHNESS_KEY, "observed", deadline(600)),
        ],
    );
    manager.update_services(vec![source.clone()]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    actor.add_identity_key(usize::MAX, "user.old").unwrap();
    manager.update_services(vec![]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.changed);
    assert!(outcome.indeterminate.is_empty());
    assert!(
        !actor
            .attrs_iter()
            .any(|attr| attr.get_source() == "company")
    );
    assert!(!actor.identity_keys_iter().any(|key| key == "user.old"));
    assert!(actor.get_attribute(key::CN).is_some());
    assert!(actor.get_zpr_addr().is_some());
    assert_eq!(source.calls(), 1);
}

#[tokio::test]
async fn removed_source_claims_are_cleaned_after_revision_memory_is_lost() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    manager.update_services(vec![Source::new(
        "company",
        1,
        vec![claim("company", "user.old", "old", deadline(600))],
    )]);
    refresh(&manager, &mut actor).await;
    let restarted = TrustedServicesMgr::new();
    let outcome = refresh(&restarted, &mut actor).await;
    assert!(outcome.changed);
    assert!(outcome.indeterminate.is_empty());
    assert!(
        !actor
            .attrs_iter()
            .any(|attr| attr.get_source() == "company")
    );
}

#[tokio::test]
async fn removed_identity_is_not_forwarded_to_remaining_provider() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    manager.update_services(vec![Source::new(
        "company",
        1,
        vec![claim("company", "user.old", "old", deadline(600))],
    )]);
    refresh(&manager, &mut actor).await;
    let remaining = Source::new("lab", 2, vec![]);
    manager.update_services(vec![remaining.clone()]);
    let outcome = refresh_expired_attributes(
        &manager,
        &[key::CN, "user.old", "user.old__source__company"],
        &mut actor,
    )
    .await;
    assert!(outcome.indeterminate.is_empty());
    let calls = remaining.calls.lock().unwrap();
    assert_eq!(
        calls.as_slice(),
        &[vec![(key::CN.into(), "device.example".into())]]
    );
}

#[tokio::test]
async fn removed_expired_identity_does_not_prevent_remaining_source_refresh() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    actor
        .add_attribute(claim(
            "company",
            "user.old__source__company",
            "old",
            SystemTime::now() - Duration::from_secs(1),
        ))
        .unwrap();
    let remaining = Source::new("lab", 2, vec![]);
    manager.update_services(vec![remaining.clone()]);
    let outcome = refresh_expired_attributes(
        &manager,
        &[key::CN, "user.old__source__company"],
        &mut actor,
    )
    .await;
    assert!(outcome.indeterminate.is_empty());
    assert_eq!(remaining.calls(), 1);
    assert!(actor.get_attribute("user.old__source__company").is_none());
}

#[tokio::test]
async fn removal_preserves_active_source_and_unrelated_authenticated_claims() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let company = Source::new(
        "company",
        1,
        vec![claim("company", "user.old", "old", deadline(600))],
    );
    let lab = Source::new(
        "lab",
        2,
        vec![claim("lab", "device.site", "hq", deadline(600))],
    );
    manager.update_services(vec![company, lab.clone()]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    let peer = actor
        .get_attribute("device.site__source__lab")
        .unwrap()
        .clone();
    manager.update_services(vec![lab.clone()]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.changed);
    assert!(outcome.indeterminate.is_empty());
    assert!(!actor.attrs_iter().any(|a| a.get_source() == "company"));
    let retained = actor.get_attribute("device.site__source__lab").unwrap();
    assert_eq!(retained.get_value(), peer.get_value());
    assert_eq!(retained.get_expires(), peer.get_expires());
    assert_eq!(lab.calls(), 1);
    assert!(actor.get_attribute(key::CN).is_some());
}

#[tokio::test]
async fn removed_source_pruning_rejection_leaves_stored_claims_and_retries() {
    let mut f = PersistenceFixture::new().await;
    f.asm.ts_mgr.update_services(vec![]);
    f.reject_write();
    assert!(matches!(
        refresh_and_persist_actor(&f.asm, &mut f.actor).await,
        Err(ServiceError::Store(_))
    ));
    assert!(f.stored().await.get_attribute("user.old").is_some());
    f.repair_write();
    let mut retry = f.stored().await;
    assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert!(
        !f.stored()
            .await
            .attrs_iter()
            .any(|a| a.get_source() == "company")
    );
}

#[tokio::test]
async fn successful_removal_forgets_revision_so_same_revision_readdition_refreshes() {
    let mut f = PersistenceFixture::new().await;
    f.asm.ts_mgr.update_services(vec![]);
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .unwrap()
    );
    f.source.revision.store(1, Ordering::Relaxed);
    f.asm.ts_mgr.update_services(vec![f.source.clone()]);
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("company".into(), 1)]
    );
    let before = f.source.calls();
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .unwrap()
    );
    assert_eq!(f.source.calls(), before + 1);
    assert!(f.stored().await.get_attribute("user.new").is_some());
}

#[tokio::test]
async fn removal_persists_even_when_authenticated_identity_is_expired() {
    let mut f = PersistenceFixture::new().await;
    f.actor
        .add_attribute(claim(
            "authenticated",
            key::CN,
            "device.example",
            SystemTime::now() - Duration::from_secs(1),
        ))
        .unwrap();
    // The assembly policy may not select CN; exercise the explicit identity path.
    f.asm.ts_mgr.update_services(vec![]);
    let outcome = refresh_expired_attributes(&f.asm.ts_mgr, &[key::CN], &mut f.actor).await;
    assert!(outcome.changed);
    assert!(!outcome.indeterminate.is_empty());
    assert!(!f.actor.attrs_iter().any(|a| a.get_source() == "company"));
}

#[tokio::test]
async fn rejected_attribute_write_preserves_exact_stored_claims() {
    use crate::db::DbConnection;
    let mut f = PersistenceFixture::new().await;
    let attrs = format!("actor:{}:attrs", f.address().to_string().replace(':', "-"));
    let before = f.db.hgetall(attrs.clone()).await.unwrap();
    f.reject_write();
    assert!(matches!(
        refresh_and_persist_actor(&f.asm, &mut f.actor).await,
        Err(ServiceError::Store(_))
    ));
    assert_eq!(f.db.hgetall(attrs).await.unwrap(), before);
    f.assert_retry_pending();
}

#[tokio::test]
async fn rejected_write_does_not_publish_local_refresh_as_persisted() {
    let mut f = PersistenceFixture::new().await;
    f.reject_write();
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .is_err()
    );
    assert!(f.actor.get_attribute("user.new").is_some());
    assert!(f.actor.get_attribute("user.old").is_none());
    let stored = f.stored().await;
    assert!(stored.get_attribute("user.old").is_some());
    assert!(stored.get_attribute("user.new").is_none());
    f.assert_retry_pending();
}

#[tokio::test]
async fn rejected_write_retry_reloads_and_persists_authoritative_replacement() {
    let mut f = PersistenceFixture::new().await;
    f.reject_write();
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .is_err()
    );
    f.repair_write();
    let mut retry = f.stored().await;
    let before = f.source.calls();
    assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert_eq!(f.source.calls(), before + 1);
    let stored = f.stored().await;
    for name in ["user.old", "user.old__source__company"] {
        assert!(stored.get_attribute(name).is_none());
    }
    for name in ["user.new", "user.new__source__company", key::USER_AUTHORITY] {
        assert!(stored.get_attribute(name).is_some());
    }
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn repeated_rejected_writes_never_suppress_provider_retry() {
    let f = PersistenceFixture::new().await;
    f.reject_write();
    let before = f.source.calls();
    for _ in 0..3 {
        let mut retry = f.stored().await;
        assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.is_err());
        f.assert_retry_pending();
    }
    assert_eq!(f.source.calls(), before + 3);
}

#[tokio::test]
async fn rejected_pruning_write_does_not_commit_failure_sentinel() {
    let mut f = PersistenceFixture::new().await;
    f.source.fail.store(true, Ordering::Relaxed);
    f.reject_write();
    assert!(matches!(
        refresh_and_persist_actor(&f.asm, &mut f.actor).await,
        Err(ServiceError::Store(_))
    ));
    assert!(f.actor.get_attribute("user.old").is_none());
    assert!(f.stored().await.get_attribute("user.old").is_some());
    // Distinguishes an unchanged recorded revision of 1 from a committed sentinel.
    f.source.revision.store(1, Ordering::Relaxed);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn repaired_pruning_write_reports_indeterminate_and_keeps_retry_pending() {
    let mut f = PersistenceFixture::new().await;
    f.source.fail.store(true, Ordering::Relaxed);
    f.reject_write();
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .is_err()
    );
    f.repair_write();
    let mut retry = f.stored().await;
    assert!(matches!(
        refresh_and_persist_actor(&f.asm, &mut retry).await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.old").is_none());
    f.assert_retry_pending();
    f.source.revision.store(1, Ordering::Relaxed);
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("company".into(), 1)]
    );
}

#[tokio::test]
async fn mixed_source_write_failure_commits_neither_success_nor_failure_revision() {
    let mut f = PersistenceFixture::new().await;
    let peer = Source::new("lab", 8, vec![]);
    peer.fail.store(true, Ordering::Relaxed);
    f.asm
        .ts_mgr
        .update_services(vec![f.source.clone(), peer.clone()]);
    f.reject_write();
    assert!(matches!(
        refresh_and_persist_actor(&f.asm, &mut f.actor).await,
        Err(ServiceError::Store(_))
    ));
    let stale = f.asm.ts_mgr.stale_sources_for_actor(&f.address());
    assert_eq!(stale.len(), 2);
    assert!(stale.contains(&("company".into(), 2)));
    assert!(stale.contains(&("lab".into(), 8)));
    f.source.revision.store(1, Ordering::Relaxed);
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("lab".into(), 8)]
    );
    assert!(f.stored().await.get_attribute("user.old").is_some());
}

#[tokio::test]
async fn failed_write_preserves_stored_identity_bookkeeping_until_retry() {
    let mut f = PersistenceFixture::new().await;
    f.reject_write();
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .is_err()
    );
    assert!(!f.actor.identity_keys_iter().any(|key| key == "user.old"));
    assert!(
        f.stored()
            .await
            .identity_keys_iter()
            .any(|key| key == "user.old")
    );
    f.repair_write();
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert!(
        !f.stored()
            .await
            .identity_keys_iter()
            .any(|key| key == "user.old")
    );
}

#[tokio::test]
async fn standalone_delete_rejection_cannot_interrupt_batched_actor_update() {
    let mut f = PersistenceFixture::new().await;
    f.db.set_del_fault(crate::db::FaultMode::Reject);
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .unwrap()
    );
    let stored = f.stored().await;
    assert!(stored.get_attribute("user.new").is_some());
    assert!(stored.get_attribute("user.old").is_none());
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn standalone_delete_lost_reply_cannot_interrupt_batched_actor_update() {
    let mut f = PersistenceFixture::new().await;
    f.db.set_del_fault(crate::db::FaultMode::ApplyThenError);
    assert!(
        refresh_and_persist_actor(&f.asm, &mut f.actor)
            .await
            .unwrap()
    );
    assert!(f.stored().await.get_attribute("user.new").is_some());
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn canceled_pipeline_reply_retries_even_with_fresh_persisted_attributes() {
    let mut f = PersistenceFixture::new().await;
    let guard = f.db.hold_pipeline_replies_for_test().await;
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut f.actor,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    drop(pending);
    drop(guard);
    let mut retry = f.stored().await;
    assert!(retry.get_attribute("user.new").is_some());
    let before = f.source.calls();
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert_eq!(f.source.calls(), before + 1);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
    assert!(!refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert_eq!(f.source.calls(), before + 1);
}

#[tokio::test]
async fn sweep_counts_write_failure_separately_from_missing_actor_and_recovers() {
    let f = PersistenceFixture::new().await;
    let missing = "fd5a:5052::999".parse().unwrap();
    let addresses = HashSet::from([f.address(), missing]);
    f.reject_write();
    assert_eq!(refresh_actors(&f.asm, addresses.clone()).await, (0, 1, 1));
    f.assert_retry_pending();
    f.repair_write();
    assert_eq!(refresh_actors(&f.asm, addresses.clone()).await, (1, 1, 0));
    assert_eq!(refresh_actors(&f.asm, addresses).await, (0, 1, 0));
}

#[tokio::test]
async fn malformed_stored_actor_fails_load_without_provider_lookup_or_revision_commit() {
    use crate::db::DbConnection;
    let f = PersistenceFixture::new().await;
    let attrs = format!("actor:{}:attrs", f.address().to_string().replace(':', "-"));
    f.db.hset(&attrs, "user.old", "{bad json").await.unwrap();
    let before = f.source.calls();
    assert_eq!(
        refresh_actors(&f.asm, HashSet::from([f.address()])).await,
        (0, 0, 1)
    );
    assert_eq!(f.source.calls(), before);
    f.assert_retry_pending();
}

#[tokio::test]
async fn missing_actor_write_is_not_recreated_by_refresh_or_marked_current() {
    use crate::db::DbConnection;
    let mut f = PersistenceFixture::new().await;
    let base = format!("actor:{}", f.address().to_string().replace(':', "-"));
    f.db.del(&base).await.unwrap();
    assert!(matches!(
        refresh_and_persist_actor(&f.asm, &mut f.actor).await,
        Err(ServiceError::Store(_))
    ));
    assert!(!f.db.exists(&base).await.unwrap());
    f.assert_retry_pending();
}

#[tokio::test]
async fn addressless_actor_never_attempts_lookup_write_or_revision_commit() {
    let f = PersistenceFixture::new().await;
    let mut actor = Actor::new();
    actor
        .add_attribute(claim(
            "authenticated",
            key::CN,
            "device.example",
            deadline(600),
        ))
        .unwrap();
    f.reject_write();
    let before = f.source.calls();
    assert!(!refresh_and_persist_actor(&f.asm, &mut actor).await.unwrap());
    assert_eq!(f.source.calls(), before);
    f.assert_retry_pending();
}

#[tokio::test]
async fn changed_mapping_replaces_unexpired_keys_and_identity_bookkeeping() {
    let manager = TrustedServicesMgr::new();
    let old = Source::new(
        "company",
        1,
        vec![claim("company", "user.old", "old", deadline(600))],
    );
    manager.update_services(vec![old.clone()]);
    let mut actor = actor();
    let outcome = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &outcome);
    actor.add_identity_key(usize::MAX, "user.old").unwrap();
    let replacement = Source::new(
        "company",
        2,
        vec![claim("company", "user.new", "new", deadline(300))],
    );
    manager.update_services(vec![replacement.clone()]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.indeterminate.is_empty());
    assert!(actor.get_attribute("user.old").is_none());
    assert!(
        actor
            .get_attribute(&key_for("user.old", "company"))
            .is_none()
    );
    assert!(!actor.identity_keys_iter().any(|key| key == "user.old"));
    assert!(actor.get_attribute("user.new").unwrap().value_has("new"));
    assert_eq!(replacement.calls(), 1);
    commit(&manager, &actor, &outcome);
    assert!(!refresh(&manager, &mut actor).await.changed);
}

#[tokio::test]
async fn refreshed_multisource_configuration_prunes_cached_legacy_aliases() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    manager.update_services(vec![Source::new(
        "company",
        1,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            deadline(600),
        )],
    )]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    assert!(actor.get_attribute("user.department").is_some());
    manager.update_services(vec![
        Source::new(
            "company",
            2,
            vec![claim(
                "company",
                "user.department",
                "engineering",
                deadline(600),
            )],
        ),
        Source::new(
            "lab",
            3,
            vec![claim("lab", "user.department", "research", deadline(600))],
        ),
    ]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.indeterminate.is_empty());
    for name in ["user.department", key::USER_AUTHORITY] {
        assert!(actor.get_attribute(name).is_none(), "{name}");
        assert!(actor.get_attribute(&key_for(name, "company")).is_some());
        assert!(actor.get_attribute(&key_for(name, "lab")).is_some());
    }
    assert!(actor.get_attribute(key::CN).is_some());
    commit(&manager, &actor, &outcome);
    assert!(
        manager
            .stale_sources_for_actor(actor.get_zpr_addr().unwrap())
            .is_empty()
    );
}

#[tokio::test]
async fn refreshed_single_source_configuration_restores_legacy_aliases() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    manager.update_services(vec![
        Source::new(
            "company",
            1,
            vec![claim(
                "company",
                "user.department",
                "engineering",
                deadline(600),
            )],
        ),
        Source::new("lab", 2, vec![]),
    ]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    assert!(actor.get_attribute("user.department").is_none());
    manager.update_services(vec![Source::new(
        "company",
        3,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            deadline(300),
        )],
    )]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.indeterminate.is_empty());
    for key in ["user.department", key::USER_AUTHORITY] {
        let legacy = actor.get_attribute(key).unwrap();
        let qualified = actor.get_attribute(&key_for(key, "company")).unwrap();
        assert_eq!(legacy.get_value(), qualified.get_value());
        assert_eq!(legacy.get_expires(), qualified.get_expires());
    }
}

#[tokio::test]
async fn bounded_empty_replacement_keeps_only_freshness_not_user_authority() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    manager.update_services(vec![Source::new(
        "company",
        1,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            deadline(600),
        )],
    )]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    let expires = deadline(120);
    manager.update_services(vec![Source::new(
        "company",
        2,
        vec![
            AttributeSource::new("company")
                .builder(SOURCE_FRESHNESS_KEY)
                .expires(expires)
                .values(Vec::<String>::new()),
        ],
    )]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.indeterminate.is_empty());
    assert!(
        !actor
            .attrs_iter()
            .any(|attr| attr.get_key().starts_with("user."))
    );
    for key in [
        SOURCE_FRESHNESS_KEY.to_string(),
        key_for(SOURCE_FRESHNESS_KEY, "company"),
    ] {
        let marker = actor.get_attribute(&key).unwrap();
        assert_eq!(marker.get_expires(), expires);
        assert!(marker.get_value().is_empty());
    }
    commit(&manager, &actor, &outcome);
    assert!(!refresh(&manager, &mut actor).await.changed);
}

#[tokio::test]
async fn shorter_source_deadline_does_not_change_peer_claim_or_authority_deadlines() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let company_exp = deadline(600);
    let lab_exp = deadline(900);
    let company = Source::new(
        "company",
        1,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            company_exp,
        )],
    );
    let lab = Source::new(
        "lab",
        2,
        vec![claim("lab", "user.department", "research", lab_exp)],
    );
    manager.update_services(vec![company.clone(), lab.clone()]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    let shorter = deadline(120);
    *company.attrs.lock().unwrap() =
        vec![claim("company", "user.department", "engineering", shorter)];
    company.flush().await.unwrap();
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.indeterminate.is_empty());
    for key in ["user.department", key::USER_AUTHORITY] {
        assert_eq!(
            actor
                .get_attribute(&key_for(key, "company"))
                .unwrap()
                .get_expires(),
            shorter
        );
        assert_eq!(
            actor
                .get_attribute(&key_for(key, "lab"))
                .unwrap()
                .get_expires(),
            lab_exp
        );
    }
    assert_eq!(lab.calls(), 1);
}

#[tokio::test]
async fn mixed_failure_strips_only_stale_failed_source_and_recovers() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let company = Source::new(
        "company",
        1,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            deadline(600),
        )],
    );
    let lab = Source::new(
        "lab",
        2,
        vec![claim("lab", "user.department", "research", deadline(600))],
    );
    manager.update_services(vec![company.clone(), lab.clone()]);
    let initial = refresh(&manager, &mut actor).await;
    commit(&manager, &actor, &initial);
    lab.fail.store(true, Ordering::Relaxed);
    for source in [&company, &lab] {
        source.flush().await.unwrap();
    }
    let outcome = refresh(&manager, &mut actor).await;
    assert_eq!(outcome.indeterminate, ["lab"]);
    assert!(!actor.attrs_iter().any(|attr| attr.get_source() == "lab"));
    assert!(
        actor
            .get_attribute(&key_for("user.department", "company"))
            .is_some()
    );
    commit(&manager, &actor, &outcome);
    assert_eq!(
        manager.stale_sources_for_actor(actor.get_zpr_addr().unwrap()),
        vec![("lab".into(), lab.current_revision())]
    );
    lab.fail.store(false, Ordering::Relaxed);
    let recovered = refresh(&manager, &mut actor).await;
    assert!(recovered.indeterminate.is_empty());
    assert_eq!(company.calls(), 2);
    assert_eq!(lab.calls(), 3);
    assert!(
        actor
            .get_attribute(&key_for(key::USER_AUTHORITY, "lab"))
            .is_some()
    );
}

#[tokio::test]
async fn ttl_failure_becomes_revision_retry_then_recovers_authoritatively() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let company = Source::new("company", 1, vec![]);
    manager.update_services(vec![company.clone()]);
    manager.record_revision(actor.get_zpr_addr().unwrap(), "company", 1);
    actor
        .add_attribute(claim(
            "company",
            "user.old",
            "old",
            SystemTime::now() - Duration::from_secs(1),
        ))
        .unwrap();
    company.fail.store(true, Ordering::Relaxed);
    let first = refresh(&manager, &mut actor).await;
    assert!(!first.indeterminate.is_empty());
    assert!(actor.get_attribute("user.old").is_some());
    commit(&manager, &actor, &first);
    let second = refresh(&manager, &mut actor).await;
    assert!(!second.indeterminate.is_empty());
    assert!(actor.get_attribute("user.old").is_none());
    commit(&manager, &actor, &second);
    company.fail.store(false, Ordering::Relaxed);
    *company.attrs.lock().unwrap() = vec![claim("company", "user.new", "new", deadline(300))];
    let third = refresh(&manager, &mut actor).await;
    assert!(third.indeterminate.is_empty());
    assert!(actor.get_attribute("user.new").is_some());
    assert_eq!(company.calls(), 3);
    commit(&manager, &actor, &third);
    assert!(!refresh(&manager, &mut actor).await.changed);
}

struct FlushDuringLookup(Arc<Source>);

#[tokio::test]
async fn source_replacement_during_actor_write_invalidates_old_revision_and_reloads() {
    let f = PersistenceFixture::new().await;
    let mut working = f.actor.clone();
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    assert!(f.stored().await.get_attribute("user.old").is_some());
    // Reusing the previously recorded revision must not certify the old write.
    let replacement = Source::new(
        "company",
        1,
        vec![claim("company", "user.current", "current", deadline(600))],
    );
    f.asm.ts_mgr.update_services(vec![replacement.clone()]);
    drop(guard);
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.new").is_some());
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("company".into(), 1)]
    );
    let mut retry = f.stored().await;
    assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert_eq!(replacement.calls(), 1);
    assert!(f.stored().await.get_attribute("user.current").is_some());
    assert!(f.stored().await.get_attribute("user.new").is_none());
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn policy_publication_during_actor_write_invalidates_all_actor_revisions_only() {
    let mut f = PersistenceFixture::new().await;
    let peer: IpAddr = "fd5a:5052::124".parse().unwrap();
    f.asm.ts_mgr.record_revision(&peer, "company", 2);
    f.asm.ts_mgr.record_revision(&f.address(), "other", 7);
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut f.actor,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    f.asm
        .policy_mgr
        .update_policy_from_container_bytes(crate::assembly::tests::make_policy(
            "2026-10-24T00:00:00Z",
            2,
            None,
        ))
        .await
        .unwrap();
    drop(guard);
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.new").is_some());
    assert!(f.asm.ts_mgr.stale_sources_for_actor(&peer).is_empty());
    let other = Source::new("other", 7, vec![]);
    f.asm
        .ts_mgr
        .update_services(vec![f.source.clone(), other.clone()]);
    let stale: std::collections::HashMap<_, _> = f
        .asm
        .ts_mgr
        .stale_sources_for_actor(&f.address())
        .into_iter()
        .collect();
    assert_eq!(stale.len(), 2);
    assert_eq!(stale.get("company"), Some(&2));
    assert_eq!(stale.get("other"), Some(&7));
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert_eq!(f.source.calls(), calls + 2);
    assert_eq!(other.calls(), 1);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn removal_during_actor_write_is_rejected_then_pruned_on_reload() {
    let mut f = PersistenceFixture::new().await;
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut f.actor,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    f.asm.ts_mgr.update_services(vec![]);
    drop(guard);
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.new").is_some());
    let mut retry = f.stored().await;
    assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert!(
        !f.stored()
            .await
            .attrs_iter()
            .any(|a| a.get_source() == "company")
    );
    assert_eq!(f.source.calls(), calls + 1);
}

#[tokio::test]
async fn same_store_republication_during_actor_write_requires_another_lookup() {
    let mut f = PersistenceFixture::new().await;
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut f.actor,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    f.asm.ts_mgr.update_services(vec![]);
    f.asm.ts_mgr.update_services(vec![f.source.clone()]);
    drop(guard);
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert_eq!(f.source.calls(), calls + 2);
}

#[tokio::test]
async fn rejected_actor_write_preserves_store_error_and_invalidates_prior_revision() {
    let mut f = PersistenceFixture::new().await;
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut f.actor,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    let replacement = Source::new("company", 1, vec![]);
    f.asm.ts_mgr.update_services(vec![replacement.clone()]);
    f.db.set_set_ex_fault(crate::db::FaultMode::Reject);
    drop(guard);
    assert!(matches!(pending.await, Err(ServiceError::Store(_))));
    assert!(f.stored().await.get_attribute("user.old").is_some());
    assert!(f.stored().await.get_attribute("user.new").is_none());
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("company".into(), 1)]
    );
    f.repair_write();
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert_eq!(replacement.calls(), 1);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn unchanged_publication_during_actor_write_commits_revision_once() {
    let f = PersistenceFixture::new().await;
    let mut working = f.actor.clone();
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    assert!(f.stored().await.get_attribute("user.new").is_none());
    drop(guard);
    assert!(pending.await.unwrap());
    assert!(f.stored().await.get_attribute("user.new").is_some());
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
    let mut retry = f.stored().await;
    assert!(!refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert_eq!(f.source.calls(), calls + 1);
}

#[tokio::test]
async fn canceled_actor_write_before_mutation_invalidates_only_affected_actor() {
    let f = PersistenceFixture::new().await;
    let peer: IpAddr = "fd5a:5052::124".parse().unwrap();
    f.asm.ts_mgr.record_revision(&peer, "company", 1);
    let mut working = f.actor.clone();
    let guard = f.db.hold_pipelines_for_test().await;
    let calls = f.source.calls();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert_eq!(f.source.calls(), calls + 1);
    let replacement = Source::new("company", 1, vec![]);
    f.asm.ts_mgr.update_services(vec![replacement.clone()]);
    drop(pending);
    drop(guard);
    assert!(f.stored().await.get_attribute("user.old").is_some());
    assert!(f.stored().await.get_attribute("user.new").is_none());
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("company".into(), 1)]
    );
    assert!(f.asm.ts_mgr.stale_sources_for_actor(&peer).is_empty());
    let mut retry = f.stored().await;
    assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert_eq!(replacement.calls(), 1);
    assert!(f.stored().await.get_attribute("user.old").is_none());
}

#[tokio::test]
async fn canceled_actor_write_after_mutation_requires_replacement_lookup() {
    let f = PersistenceFixture::new().await;
    let mut working = f.actor.clone();
    let guard = f.db.hold_pipeline_replies_for_test().await;
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert!(f.stored().await.get_attribute("user.new").is_some());
    assert!(f.stored().await.get_attribute("user.old").is_none());
    let replacement = Source::new(
        "company",
        1,
        vec![claim("company", "user.current", "current", deadline(600))],
    );
    f.asm.ts_mgr.update_services(vec![replacement.clone()]);
    drop(pending);
    drop(guard);
    assert_eq!(
        f.asm.ts_mgr.stale_sources_for_actor(&f.address()),
        vec![("company".into(), 1)]
    );
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert_eq!(replacement.calls(), 1);
    let stored = f.stored().await;
    assert!(stored.get_attribute("user.new").is_none());
    assert!(stored.get_attribute("user.current").is_some());
    assert!(!stored.identity_keys_iter().any(|key| key == "user.old"));
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn canceled_actor_write_without_publication_invalidates_previously_current_sources() {
    let f = PersistenceFixture::new().await;
    let other = Source::new("other", 7, vec![]);
    f.asm
        .ts_mgr
        .update_services(vec![f.source.clone(), other.clone()]);
    f.asm.ts_mgr.record_revision(&f.address(), "other", 7);
    let calls = f.source.calls();
    let mut working = f.actor.clone();
    let guard = f.db.hold_pipeline_replies_for_test().await;
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert!(
        f.stored()
            .await
            .get_attribute("user.new__source__company")
            .is_some()
    );
    assert_eq!(other.calls(), 0);
    drop(pending);
    drop(guard);
    assert_eq!(f.asm.ts_mgr.stale_sources_for_actor(&f.address()).len(), 2);
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert_eq!(f.source.calls(), calls + 2);
    assert_eq!(other.calls(), 1);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn canceled_actor_write_after_source_removal_is_pruned_on_reload() {
    let f = PersistenceFixture::new().await;
    let calls = f.source.calls();
    let mut working = f.actor.clone();
    let guard = f.db.hold_pipeline_replies_for_test().await;
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    assert!(f.stored().await.get_attribute("user.new").is_some());
    f.asm.ts_mgr.update_services(vec![]);
    drop(pending);
    drop(guard);
    let mut retry = f.stored().await;
    assert!(refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap());
    assert!(
        !f.stored()
            .await
            .attrs_iter()
            .any(|a| a.get_source() == "company")
    );
    assert_eq!(f.source.calls(), calls + 1);
}

#[tokio::test]
async fn canceled_lookup_before_actor_write_preserves_prior_revision() {
    let f = PersistenceFixture::new().await;
    let blocked = BlockOnce::new(f.source.clone());
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    let mut working = f.actor.clone();
    let mut pending = Box::pin(tokio::task::unconstrained(refresh_and_persist_actor(
        &f.asm,
        &mut working,
    )));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    drop(pending);
    f.asm
        .ts_mgr
        .update_services(vec![Source::new("company", 1, vec![])]);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
    assert!(f.stored().await.get_attribute("user.old").is_some());
    assert!(f.stored().await.get_attribute("user.new").is_none());
}

struct BlockOnce {
    source: Arc<Source>,
    block: AtomicBool,
    release: tokio::sync::Notify,
}

impl BlockOnce {
    fn new(source: Arc<Source>) -> Arc<Self> {
        Arc::new(Self {
            source,
            block: AtomicBool::new(true),
            release: tokio::sync::Notify::new(),
        })
    }
}

#[async_trait::async_trait]
impl TrustedServiceInterface for BlockOnce {
    async fn get_attributes_for_actor(
        &self,
        identities: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        let result = self.source.get_attributes_for_actor(identities).await;
        if self.block.swap(false, Ordering::Relaxed) {
            self.release.notified().await;
        }
        result
    }
    async fn flush(&self) -> Result<(), ServiceError> {
        self.source.flush().await
    }
    fn current_revision(&self) -> u64 {
        self.source.current_revision()
    }
    fn get_source_id(&self) -> &str {
        self.source.get_source_id()
    }
}

#[tokio::test]
async fn replaced_inflight_refresh_does_not_persist_retired_claims_or_revision() {
    let mut f = PersistenceFixture::new().await;
    let blocked = BlockOnce::new(f.source.clone());
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    let mut pending = Box::pin(refresh_and_persist_actor(&f.asm, &mut f.actor));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let replacement = Source::new(
        "company",
        3,
        vec![claim("company", "user.current", "current", deadline(600))],
    );
    f.asm.ts_mgr.update_services(vec![replacement.clone()]);
    blocked.release.notify_one();
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.old").is_some());
    assert!(f.stored().await.get_attribute("user.new").is_none());
    f.asm
        .ts_mgr
        .update_services(vec![Source::new("company", 1, vec![])]);
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
    f.asm.ts_mgr.update_services(vec![replacement]);
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert!(f.stored().await.get_attribute("user.current").is_some());
}

#[tokio::test]
async fn removed_inflight_refresh_cannot_restore_retired_claims() {
    let mut f = PersistenceFixture::new().await;
    let blocked = BlockOnce::new(f.source.clone());
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    let mut pending = Box::pin(refresh_and_persist_actor(&f.asm, &mut f.actor));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.asm.ts_mgr.update_services(vec![]);
    blocked.release.notify_one();
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.new").is_none());
    let mut retry = f.stored().await;
    refresh_and_persist_actor(&f.asm, &mut retry).await.unwrap();
    assert!(
        !f.stored()
            .await
            .attrs_iter()
            .any(|a| a.get_source() == "company")
    );
}

#[tokio::test]
async fn policy_update_during_lookup_rejects_old_refresh_before_persistence() {
    let mut f = PersistenceFixture::new().await;
    let blocked = BlockOnce::new(f.source.clone());
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    let mut pending = Box::pin(refresh_and_persist_actor(&f.asm, &mut f.actor));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    let candidate = crate::assembly::tests::make_policy("2026-10-24T00:00:00Z", 2, None);
    f.asm
        .policy_mgr
        .update_policy_from_container_bytes(candidate)
        .await
        .unwrap();
    blocked.release.notify_one();
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.old").is_some());
    assert!(f.stored().await.get_attribute("user.new").is_none());
}

#[tokio::test]
async fn unchanged_configuration_allows_blocked_refresh_to_finish_normally() {
    let mut f = PersistenceFixture::new().await;
    let blocked = BlockOnce::new(f.source.clone());
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    let mut pending = Box::pin(refresh_and_persist_actor(&f.asm, &mut f.actor));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    blocked.release.notify_one();
    assert!(pending.await.unwrap());
    assert!(f.stored().await.get_attribute("user.new").is_some());
    assert!(
        f.asm
            .ts_mgr
            .stale_sources_for_actor(&f.address())
            .is_empty()
    );
}

#[tokio::test]
async fn source_republication_during_refresh_is_detected_even_with_same_store() {
    let mut f = PersistenceFixture::new().await;
    let blocked = BlockOnce::new(f.source.clone());
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    let mut pending = Box::pin(refresh_and_persist_actor(&f.asm, &mut f.actor));
    assert!(futures::poll!(pending.as_mut()).is_pending());
    f.asm.ts_mgr.update_services(vec![]);
    f.asm.ts_mgr.update_services(vec![blocked.clone()]);
    blocked.release.notify_one();
    assert!(matches!(
        pending.await,
        Err(ServiceError::AttributesIndeterminate(_))
    ));
    assert!(f.stored().await.get_attribute("user.new").is_none());
}

#[tokio::test]
async fn visa_request_crossing_source_replacement_denies_without_issuing() {
    let (asm, node, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
    let blocked = BlockOnce::new(Source::new("hr", 2, vec![]));
    asm.ts_mgr.update_services(vec![blocked.clone()]);
    let packet =
        zpr::vsapi_types::PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80)
            .unwrap();
    let mut request = Box::pin(tokio::task::unconstrained(
        crate::visareq_worker::process_visa_request_for_test(
            asm.clone(),
            node,
            zpr::vsapi_types::PacketDesc::new_tcp(
                "fd5a:5052:4000::a",
                "fd5a:5052:4000::b",
                1234,
                80,
            )
            .unwrap(),
        ),
    ));
    assert!(futures::poll!(request.as_mut()).is_pending());
    assert_eq!(blocked.source.calls(), 1);
    asm.ts_mgr
        .update_services(vec![Source::new("hr", 3, vec![])]);
    blocked.release.notify_one();
    assert!(matches!(
        request.await,
        crate::visareq_worker::VisaDecision::Deny(_)
    ));
    assert!(asm.visa_mgr.list_visa_metadata().await.is_empty());
    assert!(matches!(
        crate::visareq_worker::process_visa_request_for_test(asm.clone(), node, packet).await,
        crate::visareq_worker::VisaDecision::Allow(_, _)
    ));
}

#[async_trait::async_trait]
impl TrustedServiceInterface for FlushDuringLookup {
    async fn get_attributes_for_actor(
        &self,
        ids: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        let result = self.0.get_attributes_for_actor(ids).await;
        self.0.flush().await?;
        result
    }
    async fn flush(&self) -> Result<(), ServiceError> {
        self.0.flush().await
    }
    fn current_revision(&self) -> u64 {
        self.0.current_revision()
    }
    fn get_source_id(&self) -> &str {
        self.0.get_source_id()
    }
}

#[tokio::test]
async fn flush_during_lookup_cannot_mark_newer_revision_current() {
    let manager = TrustedServicesMgr::new();
    let mut actor = actor();
    let source = Source::new(
        "company",
        10,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            deadline(600),
        )],
    );
    manager.update_services(vec![Arc::new(FlushDuringLookup(source.clone()))]);
    let outcome = refresh(&manager, &mut actor).await;
    assert!(outcome.indeterminate.is_empty());
    assert_eq!(outcome.revisions, [("company".into(), 10)]);
    commit(&manager, &actor, &outcome);
    assert_eq!(
        manager.stale_sources_for_actor(actor.get_zpr_addr().unwrap()),
        vec![("company".into(), 11)]
    );
}

#[tokio::test]
async fn discarded_refresh_leaves_persisted_copy_and_revision_stale() {
    let manager = TrustedServicesMgr::new();
    let persisted = actor();
    let mut working = persisted.clone();
    let source = Source::new(
        "company",
        1,
        vec![claim(
            "company",
            "user.department",
            "engineering",
            deadline(600),
        )],
    );
    manager.update_services(vec![source.clone()]);
    let outcome = refresh(&manager, &mut working).await;
    assert!(outcome.changed);
    assert!(persisted.get_attribute("user.department").is_none());
    assert!(
        !manager
            .stale_sources_for_actor(persisted.get_zpr_addr().unwrap())
            .is_empty()
    );
    let mut retry = persisted.clone();
    let retried = refresh(&manager, &mut retry).await;
    assert_eq!(source.calls(), 2);
    assert_eq!(retried.revisions, outcome.revisions);
    commit(&manager, &retry, &retried);
    assert!(
        manager
            .stale_sources_for_actor(retry.get_zpr_addr().unwrap())
            .is_empty()
    );
}

#[tokio::test]
async fn both_endpoint_refreshes_persist_claim_removal_and_shortened_deadlines() {
    let (asm, _) = crate::test_helpers::build_sweep_asm(true).await;
    let source = Source::new(
        "company",
        1,
        vec![claim("company", "user.old", "old", deadline(600))],
    );
    asm.ts_mgr.update_services(vec![source.clone()]);
    let addresses: [IpAddr; 2] = [
        "fd5a:5052:4000::a".parse().unwrap(),
        "fd5a:5052:4000::b".parse().unwrap(),
    ];
    for address in addresses {
        let mut actor = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap()
            .unwrap();
        refresh_and_persist_actor(&asm, &mut actor).await.unwrap();
    }
    let shorter = deadline(120);
    *source.attrs.lock().unwrap() = vec![claim("company", "user.new", "new", shorter)];
    source.flush().await.unwrap();
    for address in addresses {
        let mut actor = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap()
            .unwrap();
        assert!(refresh_and_persist_actor(&asm, &mut actor).await.unwrap());
        let stored = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address)
            .await
            .unwrap()
            .unwrap();
        assert!(stored.get_attribute("user.old").is_none());
        assert!(
            stored
                .get_attribute(&key_for("user.old", "company"))
                .is_none()
        );
        for key in ["user.new", key::USER_AUTHORITY] {
            assert_eq!(stored.get_attribute(key).unwrap().get_expires(), shorter);
            assert_eq!(
                stored
                    .get_attribute(&key_for(key, "company"))
                    .unwrap()
                    .get_expires(),
                shorter
            );
        }
        assert!(asm.ts_mgr.stale_sources_for_actor(&address).is_empty());
    }
}
