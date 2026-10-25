use super::*;
use crate::subject_snapshot::{DOMAIN, Envelope};
use openssl::{pkey::PKey, sign::Signer};
use rcgen::{
    BasicConstraints, CertificateParams, CertifiedIssuer, ExtendedKeyUsagePurpose, IsCa, KeyPair,
    KeyUsagePurpose,
};
use std::sync::{Arc, Mutex};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio_rustls::rustls::{
    RootCertStore, ServerConfig, pki_types::PrivateKeyDer, server::WebPkiClientVerifier,
};

type Key = PKey<openssl::pkey::Private>;

fn sign(key: &Key, recipient: &str, sequence: &str, issued: u64, expires: u64) -> Vec<u8> {
    let raw = serde_json::to_vec(&serde_json::json!({
        "version": 1, "organization_id": "org", "vs_id": recipient,
        "epoch": "epoch-1", "sequence": sequence, "issued_at": issued.to_string(),
        "expires_at": expires.to_string(), "authorities": [], "sources": [],
        "bindings": [], "tombstones": []
    }))
    .unwrap();
    let mut signed = DOMAIN.to_vec();
    signed.extend_from_slice(&raw);
    let signature = Signer::new_without_digest(key)
        .unwrap()
        .sign_oneshot_to_vec(&signed)
        .unwrap();
    serde_json::to_vec(&Envelope {
        version: 1,
        key_id: "signer".into(),
        payload: URL_SAFE_NO_PAD.encode(raw),
        signature: URL_SAFE_NO_PAD.encode(signature),
    })
    .unwrap()
}

fn digest_of(envelope: &[u8]) -> String {
    let parsed: Envelope = serde_json::from_slice(envelope).unwrap();
    hex::encode(openssl::sha::sha256(
        &URL_SAFE_NO_PAD.decode(parsed.payload).unwrap(),
    ))
}

fn pins_json(key: &Key) -> String {
    serde_json::json!({"version": 1, "key_id": "signer", "pins": [
        {"key_id": "signer", "public_key": URL_SAFE_NO_PAD.encode(key.raw_public_key().unwrap())}
    ]})
    .to_string()
}

fn issuer() -> CertifiedIssuer<'static, KeyPair> {
    let mut params = CertificateParams::new(Vec::<String>::new()).unwrap();
    params.is_ca = IsCa::Ca(BasicConstraints::Unconstrained);
    params.key_usages = vec![KeyUsagePurpose::KeyCertSign, KeyUsagePurpose::CrlSign];
    CertifiedIssuer::self_signed(params, KeyPair::generate().unwrap()).unwrap()
}

fn client_identity(ca: &CertifiedIssuer<'static, KeyPair>) -> (String, String) {
    let key = KeyPair::generate().unwrap();
    let mut params = CertificateParams::new(Vec::<String>::new()).unwrap();
    params.extended_key_usages = vec![ExtendedKeyUsagePurpose::ClientAuth];
    let cert = params.signed_by(&key, ca).unwrap();
    (cert.pem(), key.serialize_pem())
}

#[derive(Default)]
struct Delivery {
    current: Option<Vec<u8>>,
    status: Option<u16>,
    receipt_status: Option<u16>,
    receipt_scope: Option<&'static str>,
    paths: Vec<String>,
    receipts: Vec<(String, String)>,
}

struct Fixture {
    _temp: tempfile::TempDir,
    config: FetchConfig,
    delivery: Arc<Mutex<Delivery>>,
    key: Key,
}

async fn handle<S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin>(
    mut stream: S,
    delivery: Arc<Mutex<Delivery>>,
) {
    let mut request = Vec::new();
    let mut buffer = [0; 4096];
    let (head, body) = loop {
        let Ok(count) = stream.read(&mut buffer).await else {
            return;
        };
        if count == 0 {
            return;
        }
        request.extend_from_slice(&buffer[..count]);
        if let Some(end) = request.windows(4).position(|w| w == b"\r\n\r\n") {
            let head = String::from_utf8_lossy(&request[..end]).to_string();
            let length: usize = head
                .lines()
                .find_map(|l| {
                    l.to_ascii_lowercase()
                        .strip_prefix("content-length:")
                        .map(|v| v.trim().parse().unwrap())
                })
                .unwrap_or(0);
            if request.len() >= end + 4 + length {
                break (head, request[end + 4..end + 4 + length].to_vec());
            }
        }
    };
    let line = head.lines().next().unwrap().to_owned();
    let (status, response) = {
        let mut d = delivery.lock().unwrap();
        d.paths.push(line.clone());
        if line == "GET /v1/subject-link-snapshots/epoch-1/current HTTP/1.1" {
            match (d.status, &d.current) {
                (Some(status), _) => (status, Vec::new()),
                (None, Some(current)) => (200, current.clone()),
                (None, None) => (404, Vec::new()),
            }
        } else if line == "POST /v1/subject-link-snapshots/epoch-1/receipts HTTP/1.1" {
            let value: serde_json::Value = serde_json::from_slice(&body).unwrap();
            assert_eq!(value.as_object().unwrap().len(), 2, "strict receipt body");
            let (sequence, digest) = (
                value["sequence"].as_str().unwrap().to_owned(),
                value["digest"].as_str().unwrap().to_owned(),
            );
            if let Some(status) = d.receipt_status {
                (status, Vec::new())
            } else {
                d.receipts.push((sequence.clone(), digest.clone()));
                let response = serde_json::json!({
                    "scope": {"organization_id": "org", "vs_id": d.receipt_scope.unwrap_or("vs-1"), "epoch": "epoch-1"},
                    "sequence": sequence, "digest": digest, "key_id": "signer",
                    "client_spki_sha256": "a".repeat(64), "received_at": "1000"
                });
                (200, serde_json::to_vec(&response).unwrap())
            }
        } else {
            (404, Vec::new())
        }
    };
    let mut reply = format!(
        "HTTP/1.1 {status} X\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n",
        response.len()
    )
    .into_bytes();
    reply.extend_from_slice(&response);
    let _ = stream.write_all(&reply).await;
    let _ = stream.shutdown().await;
}

async fn fixture() -> Fixture {
    let temp = tempfile::tempdir().unwrap();
    let ca = issuer();
    let server_key = KeyPair::generate().unwrap();
    let mut params = CertificateParams::new(vec!["localhost".into()]).unwrap();
    params.extended_key_usages = vec![ExtendedKeyUsagePurpose::ServerAuth];
    let server_cert = params.signed_by(&server_key, &ca).unwrap();
    let mut roots = RootCertStore::empty();
    roots.add(ca.der().clone()).unwrap();
    let verifier = WebPkiClientVerifier::builder(Arc::new(roots))
        .build()
        .unwrap();
    let server = ServerConfig::builder()
        .with_client_cert_verifier(verifier)
        .with_single_cert(
            vec![server_cert.der().clone()],
            PrivateKeyDer::try_from(server_key.serialize_der()).unwrap(),
        )
        .unwrap();
    let acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server));
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let port = listener.local_addr().unwrap().port();
    let delivery = Arc::new(Mutex::new(Delivery::default()));
    let shared = delivery.clone();
    tokio::spawn(async move {
        loop {
            let (stream, _) = listener.accept().await.unwrap();
            let acceptor = acceptor.clone();
            let shared = shared.clone();
            tokio::spawn(async move {
                if let Ok(stream) = acceptor.accept(stream).await {
                    handle(stream, shared).await;
                }
            });
        }
    });
    let write = |name: &str, contents: &str| {
        let path = temp.path().join(name);
        std::fs::write(&path, contents).unwrap();
        path
    };
    let (cert, key_pem) = client_identity(&ca);
    let key = PKey::generate_ed25519().unwrap();
    let config = FetchConfig {
        url: format!("https://localhost:{port}"),
        ca_cert: write("ca.pem", &ca.pem()),
        client_cert: write("client.pem", &cert),
        client_key: write("client.key", &key_pem),
        organization_id: "org".into(),
        vs_id: "vs-1".into(),
        epoch: "epoch-1".into(),
        signing_pins_file: write("pins.json", &pins_json(&key)),
        checkpoint_dir: temp.path().join("checkpoint"),
        poll_seconds: 60,
    };
    Fixture {
        _temp: temp,
        config,
        delivery,
        key,
    }
}

fn unchanged(sequence: &str, envelope: &[u8]) -> SyncOutcome {
    SyncOutcome::Unchanged {
        sequence: sequence.into(),
        digest: digest_of(envelope),
    }
}

fn advanced(sequence: &str, envelope: &[u8]) -> SyncOutcome {
    SyncOutcome::Advanced {
        sequence: sequence.into(),
        digest: digest_of(envelope),
    }
}

#[tokio::test]
async fn explicit_initialize_then_pull_install_and_report_receipts() {
    let f = fixture().await;
    let fetcher = Fetcher::new(&f.config).unwrap();
    let now = unix_now();
    assert!(matches!(
        fetcher.initialize(now).await,
        Err(FetchError::NoSnapshot)
    ));
    assert!(
        !f.config.checkpoint_dir.exists(),
        "withheld delivery must not create state"
    );
    assert!(
        matches!(fetcher.open(), Err(FetchError::Checkpoint(_))),
        "a missing checkpoint is never created implicitly"
    );

    let first = sign(&f.key, "vs-1", "1", now - 10, now + 600);
    f.delivery.lock().unwrap().current = Some(first.clone());
    assert_eq!(
        fetcher.initialize(now).await.unwrap(),
        advanced("1", &first)
    );
    assert!(
        matches!(
            fetcher.initialize(now).await,
            Err(FetchError::Checkpoint(_))
        ),
        "initialization never overwrites an existing checkpoint"
    );
    let mut checkpoint = fetcher.open().unwrap();
    let mut reported = None;
    for _ in 0..2 {
        assert_eq!(
            fetcher
                .sync(&mut checkpoint, &mut reported, now)
                .await
                .unwrap(),
            unchanged("1", &first)
        );
    }

    let second = sign(&f.key, "vs-1", "2", now - 5, now + 600);
    f.delivery.lock().unwrap().current = Some(second.clone());
    assert_eq!(
        fetcher
            .sync(&mut checkpoint, &mut reported, now)
            .await
            .unwrap(),
        advanced("2", &second)
    );
    {
        let d = f.delivery.lock().unwrap();
        assert_eq!(
            d.receipts,
            vec![
                ("1".into(), digest_of(&first)),
                ("1".into(), digest_of(&first)),
                ("2".into(), digest_of(&second)),
            ],
            "receipts follow durable acceptance and are not repeated within a process"
        );
        assert!(d.paths.iter().all(|p| {
            p == "GET /v1/subject-link-snapshots/epoch-1/current HTTP/1.1"
                || p == "POST /v1/subject-link-snapshots/epoch-1/receipts HTTP/1.1"
        }));
    }

    // Rollback, wrong scope, expiry and malformed bytes never change state or report.
    for (name, response) in [
        ("rollback", first.clone()),
        (
            "other recipient",
            sign(&f.key, "vs-2", "3", now - 5, now + 600),
        ),
        ("expired", sign(&f.key, "vs-1", "3", now - 20, now - 1)),
        ("malformed", b"{\"version\":1}".to_vec()),
    ] {
        f.delivery.lock().unwrap().current = Some(response);
        assert!(
            fetcher
                .sync(&mut checkpoint, &mut reported, now)
                .await
                .is_err(),
            "{name} must be rejected"
        );
    }
    f.delivery.lock().unwrap().current = None;
    assert_eq!(
        fetcher
            .sync(&mut checkpoint, &mut reported, now)
            .await
            .unwrap(),
        SyncOutcome::Withheld
    );
    f.delivery.lock().unwrap().status = Some(503);
    assert!(matches!(
        fetcher.sync(&mut checkpoint, &mut reported, now).await,
        Err(FetchError::Status(503))
    ));
    assert_eq!(f.delivery.lock().unwrap().receipts.len(), 3);
    assert_eq!(
        checkpoint.current_at(now).unwrap().digest(),
        digest_of(&second)
    );
    drop(checkpoint);
    assert_eq!(
        fetcher.open().unwrap().current_at(now).unwrap().digest(),
        digest_of(&second),
        "durable checkpoint survives reopen"
    );
}

#[tokio::test]
async fn failed_or_mismatched_receipts_are_retried_without_rolling_back_install() {
    let f = fixture().await;
    let fetcher = Fetcher::new(&f.config).unwrap();
    let now = unix_now();
    f.delivery.lock().unwrap().current = Some(sign(&f.key, "vs-1", "1", now - 10, now + 600));
    fetcher.initialize(now).await.unwrap();
    let mut checkpoint = fetcher.open().unwrap();
    let mut reported = None;
    let second = sign(&f.key, "vs-1", "2", now - 5, now + 600);
    {
        let mut d = f.delivery.lock().unwrap();
        d.current = Some(second.clone());
        d.receipt_status = Some(409);
    }
    assert!(matches!(
        fetcher.sync(&mut checkpoint, &mut reported, now).await,
        Err(FetchError::Receipt)
    ));
    assert_eq!(
        checkpoint.current_at(now).unwrap().digest(),
        digest_of(&second),
        "install is durable even when reporting fails"
    );
    {
        let mut d = f.delivery.lock().unwrap();
        d.receipt_status = None;
        d.receipt_scope = Some("vs-2");
    }
    assert!(matches!(
        fetcher.sync(&mut checkpoint, &mut reported, now).await,
        Err(FetchError::Receipt)
    ));
    assert_eq!(reported, None);
    f.delivery.lock().unwrap().receipt_scope = None;
    assert_eq!(
        fetcher
            .sync(&mut checkpoint, &mut reported, now)
            .await
            .unwrap(),
        unchanged("2", &second)
    );
    assert_eq!(reported, Some(digest_of(&second)));
}

#[tokio::test]
async fn oversized_delivery_is_rejected() {
    let f = fixture().await;
    let fetcher = Fetcher::new(&f.config).unwrap();
    f.delivery.lock().unwrap().current = Some(vec![b' '; MAX_ENVELOPE + 1]);
    assert!(matches!(
        fetcher.initialize(unix_now()).await,
        Err(FetchError::TooLarge)
    ));
    assert!(!f.config.checkpoint_dir.exists());
}

#[tokio::test]
async fn delivery_requires_the_pinned_server_root_and_a_trusted_client_certificate() {
    let f = fixture().await;
    let now = unix_now();
    f.delivery.lock().unwrap().current = Some(sign(&f.key, "vs-1", "1", now - 10, now + 600));
    let other = issuer();
    let mut wrong_root = f.config.clone();
    wrong_root.ca_cert = f.config.ca_cert.with_file_name("other-ca.pem");
    std::fs::write(&wrong_root.ca_cert, other.pem()).unwrap();
    assert!(matches!(
        Fetcher::new(&wrong_root).unwrap().initialize(now).await,
        Err(FetchError::Transport(_))
    ));
    let mut foreign_client = f.config.clone();
    let (cert, key) = client_identity(&other);
    foreign_client.client_cert = f.config.client_cert.with_file_name("foreign.pem");
    foreign_client.client_key = f.config.client_key.with_file_name("foreign.key");
    std::fs::write(&foreign_client.client_cert, cert).unwrap();
    std::fs::write(&foreign_client.client_key, key).unwrap();
    assert!(matches!(
        Fetcher::new(&foreign_client).unwrap().initialize(now).await,
        Err(FetchError::Transport(_))
    ));
    assert!(!f.config.checkpoint_dir.exists());
}

#[tokio::test]
async fn configuration_is_strict() {
    let f = fixture().await;
    let base = f.config.clone();
    let mut cases: Vec<(&str, FetchConfig)> = Vec::new();
    for (name, url) in [
        ("http", "http://localhost:1"),
        ("path", "https://localhost:1/api"),
        ("query", "https://localhost:1/?x=1"),
        ("credentials", "https://user@localhost:1"),
    ] {
        cases.push((
            name,
            FetchConfig {
                url: url.into(),
                ..base.clone()
            },
        ));
    }
    cases.push((
        "bad epoch",
        FetchConfig {
            epoch: "epoch.1".into(),
            ..base.clone()
        },
    ));
    cases.push((
        "relative checkpoint",
        FetchConfig {
            checkpoint_dir: "checkpoint".into(),
            ..base.clone()
        },
    ));
    cases.push((
        "fast poll",
        FetchConfig {
            poll_seconds: 1,
            ..base.clone()
        },
    ));
    cases.push((
        "relative pins",
        FetchConfig {
            signing_pins_file: "pins.json".into(),
            ..base.clone()
        },
    ));
    for (name, config) in cases {
        assert!(
            matches!(Fetcher::new(&config), Err(FetchError::Config(_))),
            "{name} must be rejected"
        );
    }
    let public = URL_SAFE_NO_PAD.encode(f.key.raw_public_key().unwrap());
    for (name, pins) in [
        (
            "empty",
            serde_json::json!({"version": 1, "key_id": "signer", "pins": []}),
        ),
        (
            "version",
            serde_json::json!({"version": 2, "key_id": "signer", "pins": [{"key_id": "signer", "public_key": public}]}),
        ),
        (
            "inactive key",
            serde_json::json!({"version": 1, "key_id": "other", "pins": [{"key_id": "signer", "public_key": public}]}),
        ),
        (
            "short key",
            serde_json::json!({"version": 1, "key_id": "signer", "pins": [{"key_id": "signer", "public_key": "AAAA"}]}),
        ),
        (
            "duplicate key",
            serde_json::json!({"version": 1, "key_id": "signer", "pins": [{"key_id": "signer", "public_key": public}, {"key_id": "other", "public_key": public}]}),
        ),
        (
            "unknown field",
            serde_json::json!({"version": 1, "key_id": "signer", "pins": [{"key_id": "signer", "public_key": public, "private_key": "x"}]}),
        ),
    ] {
        assert!(
            parse_signing_pins(pins.to_string().as_bytes()).is_err(),
            "{name} pins must be rejected"
        );
    }
    assert!(parse_signing_pins(pins_json(&f.key).as_bytes()).is_ok());
    let parsed: FetchConfig = toml::from_str(
        "url = \"https://localhost:1\"\nca_cert = \"/a\"\nclient_cert = \"/b\"\nclient_key = \"/c\"\norganization_id = \"org\"\nvs_id = \"vs-1\"\nepoch = \"epoch-1\"\nsigning_pins_file = \"/d\"\ncheckpoint_dir = \"/e\"",
    )
    .unwrap();
    assert_eq!(parsed.poll_seconds, 60);
    assert!(
        toml::from_str::<FetchConfig>(
            "url = \"https://localhost:1\"\nca_cert = \"/a\"\nclient_cert = \"/b\"\nclient_key = \"/c\"\norganization_id = \"org\"\nvs_id = \"vs-1\"\nepoch = \"epoch-1\"\nsigning_pins_file = \"/d\"\ncheckpoint_dir = \"/e\"\nenforce = true",
        )
        .is_err(),
        "unknown keys such as enforcement switches are rejected"
    );
}
