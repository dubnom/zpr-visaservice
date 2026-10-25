use super::*;
use rcgen::{
    BasicConstraints, CertificateParams, CertifiedIssuer, ExtendedKeyUsagePurpose, IsCa, KeyPair,
    KeyUsagePurpose,
};
use std::sync::Arc;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio_rustls::rustls::{
    RootCertStore, ServerConfig, pki_types::PrivateKeyDer, server::WebPkiClientVerifier,
};
use zpr::policy_types::parse_attribute_mapping;

pub(super) fn issuer() -> CertifiedIssuer<'static, KeyPair> {
    let mut params = CertificateParams::new(Vec::<String>::new()).unwrap();
    params.is_ca = IsCa::Ca(BasicConstraints::Unconstrained);
    params.key_usages = vec![KeyUsagePurpose::KeyCertSign, KeyUsagePurpose::CrlSign];
    CertifiedIssuer::self_signed(params, KeyPair::generate().unwrap()).unwrap()
}

fn intermediate(ca: &CertifiedIssuer<'static, KeyPair>) -> CertifiedIssuer<'static, KeyPair> {
    let mut params = CertificateParams::new(Vec::<String>::new()).unwrap();
    params.is_ca = IsCa::Ca(BasicConstraints::Constrained(0));
    params.key_usages = vec![KeyUsagePurpose::KeyCertSign, KeyUsagePurpose::CrlSign];
    params
        .distinguished_name
        .push(rcgen::DnType::CommonName, "lookup intermediate");
    CertifiedIssuer::signed_by(params, KeyPair::generate().unwrap(), ca).unwrap()
}

pub(super) async fn read_lookup_request<S: tokio::io::AsyncRead + Unpin>(
    stream: &mut S,
) -> serde_json::Value {
    let mut request = Vec::new();
    let mut bytes = [0; 4096];
    loop {
        let count = stream.read(&mut bytes).await.unwrap();
        assert!(count > 0, "lookup request ended before its body");
        request.extend_from_slice(&bytes[..count]);
        assert!(request.len() < 65536, "unexpected fixture request size");
        if let Some(header_end) = request.windows(4).position(|part| part == b"\r\n\r\n") {
            let headers = String::from_utf8_lossy(&request[..header_end]);
            let length: usize = headers
                .lines()
                .find_map(|line| {
                    line.to_ascii_lowercase()
                        .strip_prefix("content-length:")
                        .map(|value| value.trim().parse().unwrap())
                })
                .unwrap();
            if request.len() >= header_end + 4 + length {
                assert!(headers.starts_with("POST /v1/attributes HTTP/1.1"));
                assert!(
                    headers
                        .to_ascii_lowercase()
                        .contains("content-type: application/json")
                );
                return serde_json::from_slice(&request[header_end + 4..header_end + 4 + length])
                    .unwrap();
            }
        }
    }
}

#[tokio::test]
async fn production_client_enforces_both_sides_of_mutual_tls() {
    for case in [
        "valid",
        "valid-ca-bundle",
        "wrong-server-root",
        "wrong-hostname",
        "expired-server",
        "wrong-server-purpose",
        "future-server",
        "future-client",
        "valid-server-chain",
        "missing-server-intermediate",
        "valid-client-chain",
        "missing-client-intermediate",
        "valid-ip-san",
        "wrong-client-root",
        "no-client-certificate",
        "expired-client",
        "wrong-client-purpose",
    ] {
        let ca = issuer();
        let unrelated = issuer();
        let intermediate = intermediate(&ca);
        let server_key = KeyPair::generate().unwrap();
        let mut server_params = CertificateParams::new(vec![if case == "valid-ip-san" {
            "127.0.0.1".into()
        } else {
            "localhost".into()
        }])
        .unwrap();
        server_params.extended_key_usages = vec![if case == "wrong-server-purpose" {
            ExtendedKeyUsagePurpose::ClientAuth
        } else {
            ExtendedKeyUsagePurpose::ServerAuth
        }];
        if case == "expired-server" {
            server_params.not_before = rcgen::date_time_ymd(2020, 1, 1);
            server_params.not_after = rcgen::date_time_ymd(2020, 1, 2);
        }
        if case == "future-server" {
            server_params.not_before = rcgen::date_time_ymd(4090, 1, 1);
            server_params.not_after = rcgen::date_time_ymd(4091, 1, 1);
        }
        let server_cert = server_params
            .signed_by(
                &server_key,
                if matches!(case, "valid-server-chain" | "missing-server-intermediate") {
                    &intermediate
                } else {
                    &ca
                },
            )
            .unwrap();
        let client_key = KeyPair::generate().unwrap();
        let mut client_params = CertificateParams::new(Vec::<String>::new()).unwrap();
        client_params.extended_key_usages = vec![if case == "wrong-client-purpose" {
            ExtendedKeyUsagePurpose::ServerAuth
        } else {
            ExtendedKeyUsagePurpose::ClientAuth
        }];
        if case == "expired-client" {
            client_params.not_before = rcgen::date_time_ymd(2020, 1, 1);
            client_params.not_after = rcgen::date_time_ymd(2020, 1, 2);
        }
        if case == "future-client" {
            client_params.not_before = rcgen::date_time_ymd(4090, 1, 1);
            client_params.not_after = rcgen::date_time_ymd(4091, 1, 1);
        }
        let client_cert = client_params
            .signed_by(
                &client_key,
                if case == "wrong-client-root" {
                    &unrelated
                } else if matches!(case, "valid-client-chain" | "missing-client-intermediate") {
                    &intermediate
                } else {
                    &ca
                },
            )
            .unwrap();
        let mut roots = RootCertStore::empty();
        roots.add(ca.der().clone()).unwrap();
        let verifier = WebPkiClientVerifier::builder(Arc::new(roots))
            .build()
            .unwrap();
        let mut server_chain = vec![server_cert.der().clone()];
        if case == "valid-server-chain" {
            server_chain.push(intermediate.der().clone());
        }
        let server = ServerConfig::builder()
            .with_client_cert_verifier(verifier)
            .with_single_cert(
                server_chain,
                PrivateKeyDer::try_from(server_key.serialize_der()).unwrap(),
            )
            .unwrap();
        let acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let expected_client = client_cert.der().clone();
        let server_task = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let Ok(mut stream) = acceptor.accept(stream).await else {
                return false;
            };
            assert_eq!(
                stream.get_ref().1.peer_certificates().unwrap().first(),
                Some(&expected_client)
            );
            let body = read_lookup_request(&mut stream).await;
            assert_eq!(body["identities"][0]["value"], "alice");
            let reply = br#"{"attributes":{"department":["engineering"]}}"#;
            stream
                .write_all(
                    format!(
                        "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                        reply.len()
                    )
                    .as_bytes(),
                )
                .await
                .unwrap();
            stream.write_all(reply).await.unwrap();
            stream.shutdown().await.unwrap();
            true
        });
        let dir = tempfile::tempdir().unwrap();
        let ca_path = dir.path().join("ca.pem");
        let cert_path = dir.path().join("client.pem");
        let key_path = dir.path().join("client-key.pem");
        std::fs::write(
            &ca_path,
            match case {
                "wrong-server-root" => unrelated.pem(),
                "valid-ca-bundle" => format!("{}{}", unrelated.pem(), ca.pem()),
                _ => ca.pem(),
            },
        )
        .unwrap();
        std::fs::write(
            &cert_path,
            if case == "valid-client-chain" {
                format!("{}{}", client_cert.pem(), intermediate.pem())
            } else {
                client_cert.pem()
            },
        )
        .unwrap();
        std::fs::write(&key_path, client_key.serialize_pem()).unwrap();
        let settings = TrustedServiceHttpConfig {
            url: if matches!(case, "wrong-hostname" | "valid-ip-san") {
                format!("https://127.0.0.1:{}", address.port())
            } else {
                format!("https://localhost:{}", address.port())
            },
            ca_cert: ca_path,
            client_cert: cert_path,
            client_key: key_path,
            token_verification_key_file: None,
        };
        let mut store = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper {
                mappings: vec![parse_attribute_mapping("department -> user.department").unwrap()],
            },
            Duration::from_secs(600),
            &settings,
        )
        .unwrap();
        if case == "no-client-certificate" {
            // Omit identity only for the negative server-enforcement control.
            store.client = reqwest::Client::builder()
                .tls_certs_only([reqwest::Certificate::from_pem(ca.pem().as_bytes()).unwrap()])
                .timeout(Duration::from_secs(5))
                .build()
                .unwrap();
        }
        let result = store
            .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
            .await;
        let observed = tokio::time::timeout(Duration::from_secs(5), server_task)
            .await
            .unwrap()
            .unwrap();
        if case.starts_with("valid") {
            let attributes = result.unwrap();
            assert_eq!(attributes.len(), 1);
            assert!(attributes[0].value_has("engineering"));
            assert!(observed);
        } else {
            assert!(
                matches!(result, Err(ServiceError::AttributesIndeterminate(_))),
                "{case}: {result:?}"
            );
            assert!(!observed, "{case}: application lookup reached server");
        }
    }
}

#[test]
fn malformed_and_mismatched_client_material_fails_initialization() {
    let ca = issuer();
    let key = KeyPair::generate().unwrap();
    let cert = CertificateParams::new(Vec::<String>::new())
        .unwrap()
        .signed_by(&key, &ca)
        .unwrap();
    let dir = tempfile::tempdir().unwrap();
    let ca_path = dir.path().join("ca.pem");
    let cert_path = dir.path().join("client.pem");
    let key_path = dir.path().join("client-key.pem");
    for case in [
        "invalid-ca",
        "empty-ca",
        "invalid-ca-der",
        "invalid-cert",
        "invalid-key",
        "mismatched-key",
    ] {
        std::fs::write(
            &ca_path,
            match case {
                "invalid-ca" => "not a CA".into(),
                "empty-ca" => String::new(),
                "invalid-ca-der" => {
                    "-----BEGIN CERTIFICATE-----\nYWJj\n-----END CERTIFICATE-----\n".into()
                }
                _ => ca.pem(),
            },
        )
        .unwrap();
        std::fs::write(
            &cert_path,
            if case == "invalid-cert" {
                "not a certificate".into()
            } else {
                cert.pem()
            },
        )
        .unwrap();
        std::fs::write(
            &key_path,
            match case {
                "invalid-key" => "not a key".into(),
                "mismatched-key" => KeyPair::generate().unwrap().serialize_pem(),
                _ => key.serialize_pem(),
            },
        )
        .unwrap();
        let result = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper { mappings: vec![] },
            Duration::from_secs(600),
            &TrustedServiceHttpConfig {
                url: "https://localhost/".into(),
                ca_cert: ca_path.clone(),
                client_cert: cert_path.clone(),
                client_key: key_path.clone(),
                token_verification_key_file: None,
            },
        );
        assert!(
            matches!(result, Err(ServiceError::TrustedServiceInit(_))),
            "{case}"
        );
    }
}

#[test]
fn origins_and_ttl_are_validated_before_reading_credentials() {
    let dir = tempfile::tempdir().unwrap();
    let missing = dir.path().join("not-created.pem");
    for url in [
        "http://localhost",
        "https://user@localhost",
        "https://user:pass@localhost",
        "https://localhost/path",
        "https://localhost/?q=1",
        "https://localhost/#fragment",
        "not a url",
    ] {
        let result = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper { mappings: vec![] },
            Duration::from_secs(600),
            &TrustedServiceHttpConfig {
                url: url.into(),
                ca_cert: missing.clone(),
                client_cert: missing.clone(),
                client_key: missing.clone(),
                token_verification_key_file: None,
            },
        );
        assert!(matches!(result, Err(ServiceError::Param(_))), "{url}");
    }
    for ttl in [Duration::ZERO, Duration::from_secs(60)] {
        let result = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper { mappings: vec![] },
            ttl,
            &TrustedServiceHttpConfig {
                url: "https://localhost".into(),
                ca_cert: missing.clone(),
                client_cert: missing.clone(),
                client_key: missing.clone(),
                token_verification_key_file: None,
            },
        );
        assert!(matches!(result, Err(ServiceError::Param(_))));
    }
}

#[test]
fn missing_tls_files_report_source_and_path() {
    let ca = issuer();
    let key = KeyPair::generate().unwrap();
    let cert = CertificateParams::new(Vec::<String>::new())
        .unwrap()
        .signed_by(&key, &ca)
        .unwrap();
    let dir = tempfile::tempdir().unwrap();
    for missing in ["ca.pem", "client.pem", "key.pem"] {
        let ca_path = dir.path().join("ca.pem");
        let cert_path = dir.path().join("client.pem");
        let key_path = dir.path().join("key.pem");
        std::fs::write(&ca_path, ca.pem()).unwrap();
        std::fs::write(&cert_path, cert.pem()).unwrap();
        std::fs::write(&key_path, key.serialize_pem()).unwrap();
        std::fs::remove_file(dir.path().join(missing)).unwrap();
        let result = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper { mappings: vec![] },
            Duration::from_secs(600),
            &TrustedServiceHttpConfig {
                url: "https://localhost".into(),
                ca_cert: ca_path,
                client_cert: cert_path,
                client_key: key_path,
                token_verification_key_file: None,
            },
        );
        let Err(error) = result else {
            panic!("missing file accepted");
        };
        let message = error.to_string();
        assert!(
            message.contains("directory") && message.contains(missing),
            "{message}"
        );
    }
}

#[tokio::test]
async fn production_timeout_bounds_handshake_headers_and_body() {
    for phase in ["handshake", "headers", "body"] {
        let certified = rcgen::generate_simple_self_signed(vec!["localhost".into()]).unwrap();
        let server = ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(
                vec![certified.cert.der().clone()],
                PrivateKeyDer::try_from(certified.signing_key.serialize_der()).unwrap(),
            )
            .unwrap();
        let acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let (stop, mut stopped) = tokio::sync::oneshot::channel::<()>();
        let task = tokio::spawn(async move {
            let (socket, _) = tokio::select! {
                accepted = listener.accept() => accepted.unwrap(),
                _ = &mut stopped => return,
            };
            if phase == "handshake" {
                let _socket = socket;
                let _ = stopped.await;
                return;
            }
            let mut stream = tokio::select! {
                accepted = acceptor.accept(socket) => accepted.unwrap(),
                _ = &mut stopped => return,
            };
            let mut request = [0; 8192];
            assert!(stream.read(&mut request).await.unwrap() > 0);
            if phase == "body" {
                stream
                    .write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\n{\"attributes\":")
                    .await
                    .unwrap();
            }
            let _ = stopped.await;
        });
        let dir = tempfile::tempdir().unwrap();
        let cert_path = dir.path().join("cert.pem");
        let key_path = dir.path().join("key.pem");
        std::fs::write(&cert_path, certified.cert.pem()).unwrap();
        std::fs::write(&key_path, certified.signing_key.serialize_pem()).unwrap();
        let store = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper { mappings: vec![] },
            Duration::from_secs(600),
            &TrustedServiceHttpConfig {
                url: format!("https://localhost:{}", address.port()),
                ca_cert: cert_path.clone(),
                client_cert: cert_path,
                client_key: key_path,
                token_verification_key_file: None,
            },
        )
        .unwrap();
        let started = std::time::Instant::now();
        let result =
            tokio::time::timeout(Duration::from_secs(8), store.get_attributes_for_actor(&[])).await;
        let _ = stop.send(());
        task.await.unwrap();
        let result = result.expect("production five-second timeout did not bound lookup");
        assert!(
            matches!(result, Err(ServiceError::AttributesIndeterminate(_))),
            "{phase}: {result:?}"
        );
        assert!(
            started.elapsed() >= Duration::from_millis(4500),
            "{phase} failed before timeout"
        );
    }
}
