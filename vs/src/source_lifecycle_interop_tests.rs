use std::collections::BTreeMap;
use std::net::IpAddr;
use std::path::PathBuf;
use std::sync::Arc;

use libeval::attribute::{Attribute, key};
use libeval::route::LinkId;
use zpr::vsapi_types::{DenyCode, PacketDesc};

use crate::actor_mgr::ActorMgr;
use crate::assembly::Assembly;
use crate::config::TrustedServiceHttpConfig;
use crate::db::{ActorRepo, DbConnection, LinkRepo, NodeRepo, PolicyRepo, RedisDb, VisaRepo};
use crate::policy_mgr::PolicyMgr;
use crate::test_helpers::{FakeResolver, make_adapter_actor_defexp, make_node_actor_defexp};
use crate::topology_mgr::TopologyMgr;
use crate::trusted_services::{TrustedServicesMgr, control};
use crate::visa_mgr::VisaMgr;
use crate::visareq_worker::{VisaDecision, process_visa_request_for_test};

const SOURCE: &str = "company";
const CLIENT: &str = "fd5a:5052:4000::a";
const SERVER: &str = "fd5a:5052:4000::b";
const UUIDS: [&str; 2] = [
    "b6a21ab4-5554-4cde-ae65-41a406a3c07f",
    "b6a21ab4-5554-4cde-ae65-41a406a3c07e",
];

fn required(name: &str) -> String {
    std::env::var(name).unwrap_or_else(|_| panic!("missing fixture input {name}"))
}

fn packet(port: u16) -> PacketDesc {
    PacketDesc::new_tcp(CLIENT, SERVER, 1234, port).unwrap()
}

async fn assembly(phase: &str) -> Arc<Assembly> {
    let client = redis::Client::open(required("ZPR_TEST_SOURCE_REDIS")).unwrap();
    let db: Arc<dyn DbConnection> =
        Arc::new(RedisDb::new(client.get_connection_manager().await.unwrap()));
    let sources = Arc::new(TrustedServicesMgr::new());
    // Same ordering as production startup: durable off must precede policy stores.
    control::restore(db.as_ref(), &sources).await.unwrap();
    let http = BTreeMap::from([(
        SOURCE.into(),
        TrustedServiceHttpConfig {
            url: required("ZPR_TEST_SOURCE_URL"),
            ca_cert: PathBuf::from(required("ZPR_TEST_SOURCE_CA")),
            client_cert: PathBuf::from(required("ZPR_TEST_SOURCE_CERT")),
            client_key: PathBuf::from(required("ZPR_TEST_SOURCE_KEY")),
            token_verification_key_file: None,
        },
    )]);
    let repo = PolicyRepo::new(db.clone());
    let policy = if phase == "initial" {
        assert!(!db.exists("source-lifecycle:visa").await.unwrap());
        PolicyMgr::new_with_initial_policy_and_http(
            std::fs::read(required("ZPR_TEST_SOURCE_BUNDLE")).unwrap(),
            repo,
            Arc::new(FakeResolver::ip_only()),
            sources.clone(),
            PathBuf::from("."),
            http,
        )
        .await
        .unwrap()
    } else {
        PolicyMgr::new_from_state_with_http(
            repo,
            Arc::new(FakeResolver::ip_only()),
            sources.clone(),
            PathBuf::from("."),
            http,
        )
        .await
        .unwrap()
    };
    assert_eq!(
        sources.statuses().len(),
        1,
        "shared declarations must deduplicate"
    );
    assert_eq!(sources.statuses()[0].name, SOURCE);
    assert_eq!(
        policy.get_current_container().as_bytes(),
        std::fs::read(required("ZPR_TEST_SOURCE_BUNDLE")).unwrap(),
        "restart must restore the actual signed compiler artifact"
    );
    let mut asm = crate::assembly::tests::new_assembly_for_tests(None).await;
    asm.policy_mgr = policy;
    asm.actor_mgr = Arc::new(ActorMgr::new(
        ActorRepo::new(db.clone()),
        NodeRepo::new(db.clone()),
        asm.counters.clone(),
    ));
    asm.visa_mgr = VisaMgr::new(VisaRepo::new(db.clone(), 1).await.unwrap());
    asm.topo_mgr = TopologyMgr::new(LinkRepo::new(db.clone()));
    asm.state_db = db;
    asm.ts_mgr = sources;
    if phase != "initial" {
        asm.actor_mgr.refresh_state().await.unwrap();
    }
    for (index, node) in ["fd5a:5052:3000::1", "fd5a:5052:3000::2"]
        .iter()
        .enumerate()
    {
        let address: IpAddr = node.parse().unwrap();
        asm.actor_mgr
            .add_node(
                &make_node_actor_defexp(node, &format!("fixture-node-{index}"), "127.0.0.1:1"),
                phase != "initial",
            )
            .await
            .unwrap();
        asm.topo_mgr.add_node(address).unwrap();
        if phase == "initial" {
            // Explicit synthetic test identities, NOT a resolver from device CN
            // to a human subject and NOT an approved subject-link implementation.
            let mut actor = make_adapter_actor_defexp([CLIENT, SERVER][index], UUIDS[index]);
            actor
                .add_attribute(
                    Attribute::builder(key::DEVICE_AUTHORITY).value("fixture-authenticated"),
                )
                .unwrap();
            if index == 1 {
                actor
                    .add_attribute(Attribute::builder(key::SERVICES).value("web.svc.zpr"))
                    .unwrap();
            }
            asm.actor_mgr
                .add_adapter_via_node(&actor, &address)
                .await
                .unwrap();
        }
    }
    asm.topo_mgr
        .add_link(
            "fd5a:5052:3000::1".parse().unwrap(),
            "fd5a:5052:3000::2".parse().unwrap(),
            LinkId("fixture-link".into()),
            vec![],
            1,
        )
        .unwrap();
    for (address, node) in [(CLIENT, "fd5a:5052:3000::1"), (SERVER, "fd5a:5052:3000::2")] {
        assert_eq!(
            asm.actor_mgr
                .get_docking_node_for_adapter(&address.parse().unwrap()),
            Some(node.parse().unwrap()),
            "fixture restart must restore docking, not produce a false policy denial"
        );
    }
    Arc::new(asm)
}

fn node() -> IpAddr {
    "fd5a:5052:3000::1".parse().unwrap()
}

async fn request(asm: &Arc<Assembly>, port: u16) -> VisaDecision {
    process_visa_request_for_test(asm.clone(), node(), packet(port)).await
}

async fn assert_observation(asm: &Arc<Assembly>, expected: &str) {
    for address in [CLIENT, SERVER] {
        let actor = asm
            .actor_mgr
            .get_actor_by_zpr_addr(&address.parse().unwrap())
            .await
            .unwrap()
            .unwrap();
        let claim = actor.get_attribute("user.mail__source__company").unwrap();
        assert!(claim.value_has(expected), "{address}: {claim:?}");
        assert_eq!(claim.get_source(), SOURCE);
        assert!(
            actor.get_attribute("user.mail").is_some(),
            "single-source legacy alias is intentional"
        );
        assert!(actor.get_attribute("userPassword").is_none());
    }
}

async fn assert_policy_result(asm: &Arc<Assembly>, expected_allow: bool) {
    let source = asm
        .actor_mgr
        .get_actor_by_zpr_addr(&CLIENT.parse().unwrap())
        .await
        .unwrap()
        .unwrap();
    let destination = asm
        .actor_mgr
        .get_actor_by_zpr_addr(&SERVER.parse().unwrap())
        .await
        .unwrap()
        .unwrap();
    let result = libeval::eval::EvalContext::new(asm.policy_mgr.get_current())
        .eval_request(&source, &destination, &packet(80))
        .unwrap();
    assert_eq!(
        matches!(
            result,
            libeval::eval_result::PartialEvalResult::AllowWithoutRoute(_)
        ),
        expected_allow,
        "directory-data denial must be a real compiled-policy decision: {result:?}"
    );
}

async fn assert_revocation(asm: &Arc<Assembly>, id: u64) {
    for address in ["fd5a:5052:3000::1", "fd5a:5052:3000::2"] {
        let node = address.parse().unwrap();
        assert!(
            asm.visa_mgr
                .get_pending_revoke_visa_ids_for_node(&node)
                .await
                .unwrap()
                .contains(&id)
        );
        assert!(
            !asm.visa_mgr
                .get_pending_visa_ids_for_node(&node)
                .await
                .unwrap()
                .contains(&id)
        );
    }
}

#[tokio::test]
#[ignore = "owned Go mTLS/LDAP/Redis fixture; run scripts/test-source-lifecycle.sh"]
async fn source_lifecycle_process_interop() {
    tracing_subscriber::fmt()
        .with_max_level(tracing::Level::WARN)
        .with_ansi(false)
        .with_test_writer()
        .try_init()
        .unwrap();
    tokio::time::timeout(std::time::Duration::from_secs(30), async {
        let phase = required("ZPR_TEST_SOURCE_PHASE");
        let asm = assembly(&phase).await;
        match phase.as_str() {
            "initial" => {
                assert!(control::read(asm.state_db.as_ref(), SOURCE).await.unwrap().enabled);
                assert!(matches!(request(&asm, 81).await, VisaDecision::Deny(_)));
                let VisaDecision::Allow(visa, _) = request(&asm, 80).await else {
                    panic!("real compiled policy and LDAP observation must allow");
                };
                assert_observation(&asm, "person@example.test").await;
                assert_policy_result(&asm, true).await;
                asm.state_db.set("source-lifecycle:visa", &visa.issuer_id.to_string()).await.unwrap();
                control::set(asm.state_db.as_ref(), &asm.ts_mgr, SOURCE, false).await.unwrap();
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(_)));
                control::process_pending(&asm).await;
                assert_revocation(&asm, visa.issuer_id).await;
                let off = control::read(asm.state_db.as_ref(), SOURCE).await.unwrap();
                assert!(!off.enabled && off.pending);
                asm.state_db.set("source-lifecycle:off-revision", &off.revision).await.unwrap();
                println!("SOURCE_LIFECYCLE initial: compiled allow/deny, source off and durable queued revocation passed");
            }
            "restart-off" => {
                let off = control::read(asm.state_db.as_ref(), SOURCE).await.unwrap();
                assert!(!off.enabled && off.pending);
                assert_eq!(Some(off.revision), asm.state_db.get("source-lifecycle:off-revision").await.unwrap());
                assert!(!asm.ts_mgr.statuses()[0].enabled);
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(_)));
                let id: u64 = asm.state_db.get("source-lifecycle:visa").await.unwrap().unwrap().parse().unwrap();
                assert_revocation(&asm, id).await;
                println!("SOURCE_LIFECYCLE restart-off: stored off and revocation survive a new VS process and database restart");
                control::set(asm.state_db.as_ref(), &asm.ts_mgr, SOURCE, true).await.unwrap();
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(_)), "re-enable must not resurrect cached allow");
                assert_observation(&asm, "contractor@example.test").await;
                assert_policy_result(&asm, false).await;
                assert_revocation(&asm, id).await;
                println!("SOURCE_LIFECYCLE changed: re-enabled fresh LDAP data denies without stale permission");
            }
            "provider-down" => {
                assert!(control::read(asm.state_db.as_ref(), SOURCE).await.unwrap().enabled);
                let before = asm.visa_mgr.list_visa_metadata().await.len();
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(DenyCode::NoReason)));
                assert_eq!(asm.ts_mgr.statuses()[0].health, "failed", "outage must remain indeterminate, not successful empty absence");
                assert_eq!(asm.visa_mgr.list_visa_metadata().await.len(), before);
                let id: u64 = asm.state_db.get("source-lifecycle:visa").await.unwrap().unwrap().parse().unwrap();
                assert_revocation(&asm, id).await;
                println!("SOURCE_LIFECYCLE provider-down: LDAP bind failure denies without a new visa or empty-observation fallback");
            }
            "recover" => {
                control::set(asm.state_db.as_ref(), &asm.ts_mgr, SOURCE, true).await.unwrap();
                control::process_pending(&asm).await;
                let VisaDecision::Allow(visa, _) = request(&asm, 80).await else { panic!("fresh restored data must allow"); };
                assert_observation(&asm, "person@example.test").await;
                assert_policy_result(&asm, true).await;
                let id: u64 = asm.state_db.get("source-lifecycle:visa").await.unwrap().unwrap().parse().unwrap();
                assert_ne!(visa.issuer_id, id);
                assert_revocation(&asm, id).await;
                assert!(!control::read(asm.state_db.as_ref(), SOURCE).await.unwrap().pending);
                println!("SOURCE_LIFECYCLE recover: fresh lookup permits a new visa; old queued revoke remains");
            }
            other => panic!("unknown fixture phase {other}"),
        }
    }).await.expect("source lifecycle phase exceeded 30 seconds");
}
