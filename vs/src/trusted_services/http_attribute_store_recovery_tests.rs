use super::tls_tests::{issuer, read_lookup_request};
use super::*;
use rcgen::{CertificateParams, ExtendedKeyUsagePurpose, KeyPair};
use std::sync::Arc;
use tokio::io::AsyncWriteExt;
use tokio_rustls::rustls::{
    RootCertStore, ServerConfig, pki_types::PrivateKeyDer, server::WebPkiClientVerifier,
};
use zpr::policy_types::parse_attribute_mapping;

fn response(status: u16, body: &str) -> Vec<u8> {
    format!(
        "HTTP/1.1 {status} Test\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    ).into_bytes()
}

async fn lookup_sequence(middle: Vec<u8>) -> Vec<Result<Vec<Attribute>, ServiceError>> {
    let ca = issuer();
    let server_key = KeyPair::generate().unwrap();
    let mut server_params = CertificateParams::new(vec!["localhost".into()]).unwrap();
    server_params.extended_key_usages = vec![ExtendedKeyUsagePurpose::ServerAuth];
    let server_cert = server_params.signed_by(&server_key, &ca).unwrap();
    let client_key = KeyPair::generate().unwrap();
    let mut client_params = CertificateParams::new(Vec::<String>::new()).unwrap();
    client_params.extended_key_usages = vec![ExtendedKeyUsagePurpose::ClientAuth];
    let client_cert = client_params.signed_by(&client_key, &ca).unwrap();
    let expected_client = client_cert.der().clone();
    let mut roots = RootCertStore::empty();
    roots.add(ca.der().clone()).unwrap();
    let server = ServerConfig::builder()
        .with_client_cert_verifier(
            WebPkiClientVerifier::builder(Arc::new(roots))
                .build()
                .unwrap(),
        )
        .with_single_cert(
            vec![server_cert.der().clone()],
            PrivateKeyDer::try_from(server_key.serialize_der()).unwrap(),
        )
        .unwrap();
    let acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server));
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = listener.local_addr().unwrap();
    let identities = [
        vec![
            ("user.sub".to_string(), "Alice\"\\\n\u{1f680}".to_string()),
            ("User.Sub".to_string(), "\\ud800".to_string()),
            ("user.sub".to_string(), "alice".to_string()),
        ],
        vec![],
        vec![("device.id".to_string(), "repaired".to_string())],
    ];
    let expected = [
        serde_json::json!({"identities": [
            {"key": "user.sub", "value": "Alice\"\\\n\u{1f680}"},
            {"key": "User.Sub", "value": "\\ud800"},
            {"key": "user.sub", "value": "alice"},
        ]}),
        serde_json::json!({"identities": []}),
        serde_json::json!({"identities": [{"key": "device.id", "value": "repaired"}]}),
    ];
    let deadline = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_secs()
        + 300;
    let replies = [
        response(200, r#"{"attributes":{"department":["sales"]}}"#),
        middle,
        response(
            200,
            &format!(
                r#"{{"attributes":{{"department":["engineering"]}},"expires_unix":{deadline}}}"#
            ),
        ),
    ];
    let mut task = tokio::spawn(async move {
        for (reply, expected) in replies.into_iter().zip(expected) {
            let (socket, _) = listener.accept().await.unwrap();
            let mut stream = acceptor.accept(socket).await.unwrap();
            assert_eq!(
                stream.get_ref().1.peer_certificates().unwrap().first(),
                Some(&expected_client)
            );
            assert_eq!(read_lookup_request(&mut stream).await, expected);
            // The consumer may reject headers before reading the full negative body.
            let _ = stream.write_all(&reply).await;
            let _ = stream.shutdown().await;
        }
    });
    let dir = tempfile::tempdir().unwrap();
    let ca_path = dir.path().join("ca.pem");
    let cert_path = dir.path().join("client.pem");
    let key_path = dir.path().join("client-key.pem");
    std::fs::write(&ca_path, ca.pem()).unwrap();
    std::fs::write(&cert_path, client_cert.pem()).unwrap();
    std::fs::write(&key_path, client_key.serialize_pem()).unwrap();
    let store = HttpAttributeStore::new(
        "directory".into(),
        AttributeMapper {
            mappings: vec![parse_attribute_mapping("department -> user.department").unwrap()],
        },
        Duration::from_secs(600),
        &TrustedServiceHttpConfig {
            url: format!("https://localhost:{}", address.port()),
            ca_cert: ca_path,
            client_cert: cert_path,
            client_key: key_path,
            token_verification_key_file: None,
        },
    )
    .unwrap();
    let result = tokio::time::timeout(Duration::from_secs(20), async {
        let mut results = Vec::new();
        let revision = store.current_revision();
        assert_ne!(revision, super::super::REVISION_NEVER);
        for identities in identities {
            results.push(store.get_attributes_for_actor(&identities).await);
            assert_eq!(
                store.current_revision(),
                revision,
                "lookup must not advance revision"
            );
        }
        let final_attributes = results.last().unwrap().as_ref().unwrap();
        assert!(final_attributes.iter().all(|attr| attr.get_expires()
            == UNIX_EPOCH + Duration::from_secs(deadline)
            && attr.get_source() == "directory"));
        let mut revision = revision;
        for _ in 0..2 {
            store.flush().await.unwrap();
            let next = store.current_revision();
            assert!(next > revision, "flush must advance revision");
            revision = next;
        }
        results
    })
    .await;
    if result.is_err() {
        task.abort();
        let _ = task.await;
        panic!("lookup sequence exceeded watchdog");
    }
    match tokio::time::timeout(Duration::from_secs(2), &mut task).await {
        Ok(joined) => joined.unwrap(),
        Err(_) => {
            task.abort();
            let _ = task.await;
            panic!("fixture did not observe exactly three requests");
        }
    }
    result.unwrap()
}

fn assert_recovered(results: Vec<Result<Vec<Attribute>, ServiceError>>) {
    let mut results = results.into_iter();
    let initial = results.next().unwrap().unwrap();
    assert_eq!(initial.len(), 1);
    assert!(initial[0].value_has("sales"));
    match results.next().unwrap() {
        Err(ServiceError::AttributesIndeterminate(message)) => {
            assert!(message.contains("directory"), "{message}");
        }
        other => panic!("failed lookup returned partial or cached observation: {other:?}"),
    }
    let repaired = results.next().unwrap().unwrap();
    assert_eq!(repaired.len(), 2);
    assert!(
        repaired
            .iter()
            .any(|attr| attr.get_key() == "user.department" && attr.value_has("engineering"))
    );
    assert!(!repaired.iter().any(|attr| attr.value_has("sales")));
    assert!(repaired.iter().any(super::super::is_source_freshness));
    assert_eq!(repaired[0].get_expires(), repaired[1].get_expires());
}

#[tokio::test]
async fn exact_identity_requests_and_empty_lookups_preserve_revision() {
    let results = lookup_sequence(response(200, r#"{"attributes":{}}"#)).await;
    assert!(results[0].is_ok());
    assert!(results[1].as_ref().unwrap().is_empty());
    assert!(results[2].is_ok());
}

#[tokio::test]
async fn same_mutual_tls_client_recovers_after_http_rejections() {
    for status in [401, 403, 429, 503] {
        assert_recovered(
            lookup_sequence(response(
                status,
                r#"{"attributes":{"department":["untrusted"]}}"#,
            ))
            .await,
        );
    }
}

#[tokio::test]
async fn same_mutual_tls_client_recovers_after_invalid_observations() {
    for body in [
        r#"{"attributes":{"department":["engineering"],"department":["sales"]}}"#,
        r#"{"attributes":{"department":["engineering"]},"expires_unix":null}"#,
        r#"{"attributes":{"department":["engineering"]},"expires_unix":1}"#,
        r#"{"attributes":{"department":["engineering","sales"]}}"#,
    ] {
        assert_recovered(lookup_sequence(response(200, body)).await);
    }
}

#[tokio::test]
async fn same_mutual_tls_client_recovers_after_interrupted_responses() {
    for wire in [
        Vec::new(),
        b"HTTP/1.1 200 OK\r\nContent-Length: 1000\r\nConnection: close\r\n\r\n{\"attributes\":{}}".to_vec(),
        b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n11\r\n{\"attributes\":{}}\r\n".to_vec(),
    ] {
        assert_recovered(lookup_sequence(wire).await);
    }
}
