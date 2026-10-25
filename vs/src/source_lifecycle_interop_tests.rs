use std::collections::BTreeMap;
use std::net::IpAddr;
use std::path::PathBuf;
use std::sync::Arc;

use libeval::attribute::{Attribute, key};
use libeval::route::LinkId;
use zpr::vsapi_types::{DenyCode, PacketDesc};

use crate::actor_mgr::ActorMgr;
use crate::admin_apikeys::{ApiKeyRecord, KeyStatus, Permission};
use crate::apikey::ApiKey;
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

async fn assembly(
    phase: &str,
) -> (
    Arc<Assembly>,
    tokio::sync::mpsc::Receiver<crate::event_mgr::VsEvent>,
) {
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
    let (mut asm, events) = crate::assembly::tests::new_assembly_with_event_rx(None).await;
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
    (Arc::new(asm), events)
}

#[derive(Debug, PartialEq, serde::Deserialize)]
#[serde(deny_unknown_fields)]
struct SourceControlReadback {
    enabled: bool,
    revision: String,
    pending: bool,
    declared: bool,
    effective_enabled: bool,
}

struct AdminClient {
    client: reqwest::Client,
    url: String,
    read_key: String,
    write_key: String,
    revoked_key: String,
}

impl AdminClient {
    async fn bridge(&self) -> Self {
        let base = required("ZPR_TEST_SOURCE_BRIDGE");
        let mut headers = reqwest::header::HeaderMap::new();
        headers.insert(
            "X-Source-Fixture",
            required("ZPR_TEST_SOURCE_BRIDGE_TOKEN").parse().unwrap(),
        );
        headers.insert(
            reqwest::header::COOKIE,
            "browser-fixture=not-an-admin-credential".parse().unwrap(),
        );
        headers.insert(
            reqwest::header::AUTHORIZATION,
            "Bearer not-an-admin-credential".parse().unwrap(),
        );
        let client = reqwest::Client::builder()
            .default_headers(headers)
            .no_proxy()
            .redirect(reqwest::redirect::Policy::none())
            .timeout(std::time::Duration::from_secs(3))
            .build()
            .unwrap();
        let bridge = Self {
            client,
            url: format!(
                "{base}/api/trusted-sources/runtime/{SOURCE}/control?organization=foreign-fixture"
            ),
            read_key: "browser-cannot-select-admin-key".into(),
            write_key: "browser-cannot-select-admin-key".into(),
            revoked_key: String::new(),
        };
        let before = self.read().await;
        let ca = std::fs::read_to_string(required("ZPR_TEST_SOURCE_CA")).unwrap();
        let admin_url = self.url.split("/admin/").next().unwrap();
        for (key, trust, expected) in [
            (
                "not-an-api-key",
                ca.clone(),
                reqwest::StatusCode::BAD_GATEWAY,
            ),
            (
                self.write_key.as_str(),
                rcgen::generate_simple_self_signed(vec!["localhost".into()])
                    .unwrap()
                    .cert
                    .pem(),
                reqwest::StatusCode::BAD_GATEWAY,
            ),
            (self.read_key.as_str(), ca.clone(), reqwest::StatusCode::OK),
            (self.write_key.as_str(), ca.clone(), reqwest::StatusCode::OK),
        ] {
            let configured = bridge
                .client
                .post(format!("{base}/fixture/config"))
                .json(&serde_json::json!({"url": admin_url, "key": key, "ca": trust}))
                .send()
                .await
                .unwrap();
            assert_eq!(configured.status(), reqwest::StatusCode::NO_CONTENT);
            let response = bridge
                .client
                .get(&bridge.url)
                .header("X-API-Key", &bridge.read_key)
                .send()
                .await
                .unwrap();
            assert_eq!(response.status(), expected);
            assert_eq!(response.headers().get("Cache-Control").unwrap(), "no-store");
            let body = response.text().await.unwrap();
            assert!(
                serde_json::from_str::<serde_json::Value>(&body)
                    .unwrap()
                    .is_object()
            );
            assert!(
                !body.contains(&self.write_key) && !body.contains(&self.read_key),
                "bridge leaked server-owned credentials"
            );
            if key == self.read_key {
                let response = bridge
                    .client
                    .put(&bridge.url)
                    .header("X-API-Key", &bridge.write_key)
                    .json(&serde_json::json!({"enabled": !before.enabled}))
                    .send()
                    .await
                    .unwrap();
                assert_eq!(response.status(), reqwest::StatusCode::FORBIDDEN);
            }
            assert_eq!(
                self.read().await,
                before,
                "bridge failure changed durable controls"
            );
        }
        for body in [
            r#"{}"#,
            r#"{"enabled":null}"#,
            r#"{"enabled":false,"extra":true}"#,
            r#"{"enabled":false,"enabled":true}"#,
        ] {
            let response = bridge
                .client
                .put(&bridge.url)
                .header("X-API-Key", &bridge.write_key)
                .header("Content-Type", "application/json")
                .body(body)
                .send()
                .await
                .unwrap();
            assert_eq!(response.status(), reqwest::StatusCode::BAD_REQUEST);
            assert_eq!(
                self.read().await,
                before,
                "malformed bridge mutation changed durable controls"
            );
        }
        let response = bridge
            .client
            .get(format!(
                "{base}/api/trusted-sources/runtime/unknown-fixture/control"
            ))
            .send()
            .await
            .unwrap();
        assert_eq!(response.status(), reqwest::StatusCode::NOT_FOUND);
        let response = bridge
            .client
            .get(format!("{base}/api/trusted-sources/runtime"))
            .header("X-API-Key", &bridge.read_key)
            .send()
            .await
            .unwrap();
        assert_eq!(response.status(), reqwest::StatusCode::OK);
        let inventory: Vec<serde_json::Value> = response.json().await.unwrap();
        assert_eq!(inventory.len(), 1);
        assert_eq!(inventory[0]["name"], SOURCE);
        assert_eq!(inventory[0]["enabled"], before.enabled);
        assert_eq!(bridge.read().await, before);
        println!(
            "SOURCE_LIFECYCLE control-bridge: real Admin TLS transport, server-owned credentials, inventory/readback, explicit credential/CA failures and strict mutations passed"
        );
        bridge
    }

    async fn start(asm: &Arc<Assembly>, tasks: &mut tokio::task::JoinSet<()>) -> Self {
        let mut keys = Vec::new();
        for (id, permission, status) in [
            (1, Permission::Read, KeyStatus::Active),
            (2, Permission::ReadWrite, KeyStatus::Active),
            (3, Permission::ReadWrite, KeyStatus::Revoked),
        ] {
            let mut secret = [0; 32];
            openssl::rand::rand_bytes(&mut secret).unwrap();
            let key = ApiKey::new(id, secret);
            asm.admin_api_keys.insert_for_test(
                key.key_id_hex(),
                ApiKeyRecord {
                    owner: "isolated-source-lifecycle".into(),
                    permission,
                    status,
                    created: "fixture".into(),
                    secret_hash: key.secret_hash().unwrap(),
                    description: "ephemeral fixture key".into(),
                },
            );
            keys.push(key.to_key_string());
        }
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let server_asm = asm.clone();
        tasks.spawn(async move {
            crate::admin_service::serve_source_lifecycle_fixture(
                listener,
                server_asm,
                &PathBuf::from(required("ZPR_TEST_SOURCE_KEY")),
                &PathBuf::from(required("ZPR_TEST_SOURCE_CERT")),
            )
            .await;
        });
        let client = reqwest::Client::builder()
            .tls_certs_only([reqwest::Certificate::from_pem(
                &std::fs::read(required("ZPR_TEST_SOURCE_CA")).unwrap(),
            )
            .unwrap()])
            .https_only(true)
            .no_proxy()
            .redirect(reqwest::redirect::Policy::none())
            .timeout(std::time::Duration::from_secs(3))
            .pool_max_idle_per_host(0)
            .build()
            .unwrap();
        let [read_key, write_key, revoked_key]: [String; 3] = keys.try_into().unwrap();
        Self {
            client,
            url: format!("https://{address}/admin/trusted-services/{SOURCE}/control"),
            read_key,
            write_key,
            revoked_key,
        }
    }

    async fn read(&self) -> SourceControlReadback {
        let response = self
            .client
            .get(&self.url)
            .header("X-API-Key", &self.read_key)
            .send()
            .await
            .unwrap();
        assert_eq!(response.status(), reqwest::StatusCode::OK);
        let status: SourceControlReadback = response.json().await.unwrap();
        assert!(status.declared);
        assert_eq!(status.effective_enabled, status.enabled);
        status
    }

    async fn set(&self, enabled: bool) -> SourceControlReadback {
        let before = self.read().await;
        let response = self
            .client
            .put(&self.url)
            .header("X-API-Key", &self.write_key)
            .json(&serde_json::json!({"enabled": enabled}))
            .send()
            .await
            .unwrap();
        assert_eq!(response.status(), reqwest::StatusCode::ACCEPTED);
        let accepted: SourceControlReadback = response.json().await.unwrap();
        assert_eq!(accepted.enabled, enabled);
        assert_eq!(accepted.effective_enabled, enabled);
        assert!(accepted.declared && accepted.pending);
        uuid::Uuid::parse_str(&accepted.revision).unwrap();
        assert_ne!(before.revision, accepted.revision);
        accepted
    }

    async fn assert_rejected_mutations(&self) {
        let before = self.read().await;
        for key in [
            None,
            Some("not-an-api-key"),
            Some(self.revoked_key.as_str()),
        ] {
            let mut read = self.client.get(&self.url);
            let mut request = self
                .client
                .put(&self.url)
                .json(&serde_json::json!({"enabled": !before.enabled}));
            if let Some(key) = key {
                read = read.header("X-API-Key", key);
                request = request.header("X-API-Key", key);
            }
            assert_eq!(
                read.send().await.unwrap().status(),
                reqwest::StatusCode::UNAUTHORIZED
            );
            assert_eq!(
                request.send().await.unwrap().status(),
                reqwest::StatusCode::UNAUTHORIZED
            );
        }
        assert_eq!(
            self.client
                .put(&self.url)
                .header("X-API-Key", &self.read_key)
                .json(&serde_json::json!({"enabled": !before.enabled}))
                .send()
                .await
                .unwrap()
                .status(),
            reqwest::StatusCode::FORBIDDEN,
        );
        for body in [
            r#"{}"#,
            r#"{"enabled":"false"}"#,
            r#"{"enabled":false,"extra":true}"#,
            r#"{"enabled":false,"enabled":true}"#,
        ] {
            assert_eq!(
                self.client
                    .put(&self.url)
                    .header("X-API-Key", &self.write_key)
                    .header("Content-Type", "application/json")
                    .body(body)
                    .send()
                    .await
                    .unwrap()
                    .status(),
                reqwest::StatusCode::UNPROCESSABLE_ENTITY,
            );
        }
        assert_eq!(
            self.read().await,
            before,
            "rejected HTTP mutations changed durable control state"
        );
    }

    async fn assert_untrusted_tls_rejected(&self) {
        let unrelated = rcgen::generate_simple_self_signed(vec!["localhost".into()]).unwrap();
        let client = reqwest::Client::builder()
            .tls_certs_only([reqwest::Certificate::from_der(unrelated.cert.der()).unwrap()])
            .https_only(true)
            .no_proxy()
            .redirect(reqwest::redirect::Policy::none())
            .timeout(std::time::Duration::from_secs(3))
            .build()
            .unwrap();
        let error = client
            .get(&self.url)
            .header("X-API-Key", &self.write_key)
            .send()
            .await
            .expect_err("unrelated trust root accepted the Admin TLS certificate");
        assert!(
            error.is_connect() && !error.is_timeout(),
            "expected TLS connection rejection: {error}"
        );
    }

    async fn wait_completed(&self, revision: &str) {
        tokio::time::timeout(std::time::Duration::from_secs(8), async {
            loop {
                let status = self.read().await;
                assert_eq!(status.revision, revision);
                if !status.pending {
                    return;
                }
                tokio::time::sleep(std::time::Duration::from_millis(10)).await;
            }
        })
        .await
        .expect("real event worker did not complete source reconciliation");
    }
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

async fn wait_revocation(asm: &Arc<Assembly>, id: u64) {
    tokio::time::timeout(std::time::Duration::from_secs(8), async {
        loop {
            let mut pending = true;
            for address in ["fd5a:5052:3000::1", "fd5a:5052:3000::2"] {
                pending &= asm
                    .visa_mgr
                    .get_pending_revoke_visa_ids_for_node(&address.parse().unwrap())
                    .await
                    .unwrap()
                    .contains(&id);
            }
            if pending {
                return;
            }
            tokio::time::sleep(std::time::Duration::from_millis(10)).await;
        }
    })
    .await
    .expect("real event worker did not queue both-node revocation");
    assert_revocation(asm, id).await;
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
        let (asm, events) = assembly(&phase).await;
        let mut tasks = tokio::task::JoinSet::new();
        let admin = AdminClient::start(&asm, &mut tasks).await;
        admin.assert_rejected_mutations().await;
        admin.assert_untrusted_tls_rejected().await;
        let admin = admin.bridge().await;
        let event_asm = asm.clone();
        tasks.spawn(crate::event_mgr::launch(event_asm, events));
        println!("SOURCE_LIFECYCLE admin-http: verified TLS, authenticated readback, rejected missing/invalid/revoked/read-only credentials and malformed mutations");
        match phase.as_str() {
            "initial" => {
                let default = admin.read().await;
                assert!(default.enabled && !default.pending && default.revision.is_empty());
                assert!(matches!(request(&asm, 81).await, VisaDecision::Deny(_)));
                let VisaDecision::Allow(visa, _) = request(&asm, 80).await else {
                    panic!("real compiled policy and LDAP observation must allow");
                };
                assert_observation(&asm, "person@example.test").await;
                assert_policy_result(&asm, true).await;
                asm.state_db.set("source-lifecycle:visa", &visa.issuer_id.to_string()).await.unwrap();
                let accepted = admin.set(false).await;
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(_)));
                wait_revocation(&asm, visa.issuer_id).await;
                let off = admin.read().await;
                assert_eq!(off, accepted);
                assert!(!off.enabled && off.pending);
                asm.state_db.set("source-lifecycle:off-revision", &off.revision).await.unwrap();
                println!("SOURCE_LIFECYCLE initial: compiled allow/deny, source off and durable queued revocation passed");
            }
            "restart-off" => {
                let off = admin.read().await;
                assert!(!off.enabled && off.pending);
                assert_eq!(Some(off.revision), asm.state_db.get("source-lifecycle:off-revision").await.unwrap());
                assert!(!asm.ts_mgr.statuses()[0].enabled);
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(_)));
                let id: u64 = asm.state_db.get("source-lifecycle:visa").await.unwrap().unwrap().parse().unwrap();
                assert_revocation(&asm, id).await;
                println!("SOURCE_LIFECYCLE restart-off: stored off and revocation survive a new VS process and database restart");
                let accepted = admin.set(true).await;
                admin.wait_completed(&accepted.revision).await;
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(_)), "re-enable must not resurrect cached allow");
                assert_observation(&asm, "contractor@example.test").await;
                assert_policy_result(&asm, false).await;
                assert_revocation(&asm, id).await;
                println!("SOURCE_LIFECYCLE changed: re-enabled fresh LDAP data denies without stale permission");
            }
            "provider-down" => {
                assert!(admin.read().await.enabled);
                let before = asm.visa_mgr.list_visa_metadata().await.len();
                assert!(matches!(request(&asm, 80).await, VisaDecision::Deny(DenyCode::NoReason)));
                assert_eq!(asm.ts_mgr.statuses()[0].health, "failed", "outage must remain indeterminate, not successful empty absence");
                assert_eq!(asm.visa_mgr.list_visa_metadata().await.len(), before);
                let id: u64 = asm.state_db.get("source-lifecycle:visa").await.unwrap().unwrap().parse().unwrap();
                assert_revocation(&asm, id).await;
                println!("SOURCE_LIFECYCLE provider-down: LDAP bind failure denies without a new visa or empty-observation fallback");
            }
            "recover" => {
                let accepted = admin.set(true).await;
                admin.wait_completed(&accepted.revision).await;
                let VisaDecision::Allow(visa, _) = request(&asm, 80).await else { panic!("fresh restored data must allow"); };
                assert_observation(&asm, "person@example.test").await;
                assert_policy_result(&asm, true).await;
                let id: u64 = asm.state_db.get("source-lifecycle:visa").await.unwrap().unwrap().parse().unwrap();
                assert_ne!(visa.issuer_id, id);
                assert_revocation(&asm, id).await;
                assert!(!admin.read().await.pending);
                println!("SOURCE_LIFECYCLE recover: fresh lookup permits a new visa; old queued revoke remains");
            }
            other => panic!("unknown fixture phase {other}"),
        }
        tasks.abort_all();
        while let Some(result) = tasks.join_next().await {
            assert!(result.unwrap_err().is_cancelled(), "fixture worker exited before shutdown");
        }
    }).await.expect("source lifecycle phase exceeded 30 seconds");
}
