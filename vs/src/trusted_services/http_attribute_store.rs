//! Mutually authenticated HTTPS trusted-service lookup.

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use async_trait::async_trait;
use libeval::attribute::{Attribute, AttributeSource};
use serde::{Deserialize, Serialize};

use crate::config::TrustedServiceHttpConfig;
use crate::error::ServiceError;

use super::attribute_mapper::{AttrHint, AttributeMapper};
use super::{SOURCE_FRESHNESS_KEY, TrustedServiceInterface, next_revision};

#[derive(Serialize)]
struct Identity<'a> {
    key: &'a str,
    value: &'a str,
}

#[derive(Serialize)]
struct Lookup<'a> {
    identities: Vec<Identity<'a>>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct LookupResult {
    #[serde(deserialize_with = "super::deserialize_unique_map")]
    attributes: BTreeMap<String, Vec<String>>,
    #[serde(default, deserialize_with = "deserialize_source_deadline")]
    expires_unix: Option<u64>,
}

fn deserialize_source_deadline<'de, D: serde::Deserializer<'de>>(
    deserializer: D,
) -> Result<Option<u64>, D::Error> {
    // Missing is legacy; an explicitly supplied value must not erase the bound.
    u64::deserialize(deserializer).map(Some)
}

pub(super) struct HttpAttributeStore {
    id: String,
    mapper: AttributeMapper,
    url: reqwest::Url,
    client: reqwest::Client,
    ttl: Duration,
    revision: AtomicU64,
}

impl HttpAttributeStore {
    pub(super) fn new(
        id: String,
        mapper: AttributeMapper,
        ttl: Duration,
        config: &TrustedServiceHttpConfig,
    ) -> Result<Self, ServiceError> {
        if ttl <= Duration::from_secs(60) {
            return Err(ServiceError::Param(format!(
                "trusted service '{id}': ttl must exceed 60 seconds"
            )));
        }
        let url = reqwest::Url::parse(&config.url).map_err(|e| {
            ServiceError::Param(format!("trusted service '{id}': invalid url: {e}"))
        })?;
        if url.scheme() != "https"
            || url.host_str().is_none()
            || !url.username().is_empty()
            || url.password().is_some()
            || url.query().is_some()
            || url.fragment().is_some()
            || url.path() != "/"
        {
            return Err(ServiceError::Param(format!(
                "trusted service '{id}': url must be an https origin"
            )));
        }
        let read_tls_file = |path: &std::path::Path, purpose: &str| {
            std::fs::read(path).map_err(|source| ServiceError::IoContext {
                context: format!("trusted service '{id}': reading {purpose} file {path:?}"),
                source,
            })
        };
        let ca = reqwest::Certificate::from_pem_bundle(&read_tls_file(&config.ca_cert, "CA")?)
            .map_err(|e| ServiceError::TrustedServiceInit(format!("{id}: invalid CA: {e}")))?;
        if ca.is_empty() {
            return Err(ServiceError::TrustedServiceInit(format!(
                "{id}: CA file contains no certificates"
            )));
        }
        let mut identity_pem = read_tls_file(&config.client_cert, "client certificate")?;
        identity_pem.extend_from_slice(&read_tls_file(&config.client_key, "client private key")?);
        let identity = reqwest::Identity::from_pem(&identity_pem).map_err(|e| {
            ServiceError::TrustedServiceInit(format!("{id}: invalid client identity: {e}"))
        })?;
        let client = reqwest::Client::builder()
            .tls_certs_only(ca)
            .identity(identity)
            .redirect(reqwest::redirect::Policy::none())
            .timeout(Duration::from_secs(5))
            .build()
            .map_err(|e| ServiceError::TrustedServiceInit(format!("{id}: HTTPS client: {e}")))?;
        Ok(Self {
            id,
            mapper,
            url,
            client,
            ttl,
            revision: AtomicU64::new(next_revision()),
        })
    }

    fn map_result(&self, result: LookupResult) -> Result<Vec<Attribute>, ServiceError> {
        let now = SystemTime::now();
        let local_expiration = now.checked_add(self.ttl).ok_or_else(|| {
            ServiceError::AttributesIndeterminate(format!("{}: invalid lookup TTL", self.id))
        })?;
        let expiration = match result.expires_unix {
            Some(seconds) => {
                let source_expiration = UNIX_EPOCH
                    .checked_add(Duration::from_secs(seconds))
                    .filter(|deadline| *deadline > now)
                    .ok_or_else(|| {
                        ServiceError::AttributesIndeterminate(format!(
                            "{}: lookup validity is expired or invalid",
                            self.id
                        ))
                    })?;
                source_expiration.min(local_expiration)
            }
            None => local_expiration,
        };
        let source = AttributeSource::new(self.id.clone());
        let mut mapped: BTreeMap<String, (AttrHint, Vec<String>)> = BTreeMap::new();
        for (name, values) in result.attributes {
            let Some((key, hint)) = self.mapper.map_attribute(&name) else {
                continue;
            };
            if key == SOURCE_FRESHNESS_KEY {
                return Err(ServiceError::AttributesIndeterminate(format!(
                    "{}: provider mapping uses reserved freshness metadata",
                    self.id
                )));
            }
            if matches!(hint, AttrHint::SingleValued) && values.len() != 1 {
                return Err(ServiceError::AttributesIndeterminate(format!(
                    "{}: expected one value for {key}",
                    self.id
                )));
            }
            if mapped
                .get(&key)
                .is_some_and(|(_, previous)| previous != &values)
            {
                return Err(ServiceError::AttributesIndeterminate(format!(
                    "{}: conflicting values for {key}",
                    self.id
                )));
            }
            mapped.insert(key, (hint, values));
        }
        let mut attributes: Vec<_> = mapped
            .into_iter()
            .map(|(key, (hint, values))| {
                let builder = source.builder(key).expires(expiration);
                match hint {
                    AttrHint::SingleValued => {
                        builder.value(values.first().cloned().unwrap_or_default())
                    }
                    AttrHint::MultiValued => builder.values(values),
                    AttrHint::Tag => builder.values(Vec::<String>::new()),
                }
            })
            .collect();
        if result.expires_unix.is_some() {
            // Empty and partially populated results also assert absence of other keys.
            attributes.push(
                source
                    .builder(SOURCE_FRESHNESS_KEY)
                    .expires(expiration)
                    .values(Vec::<String>::new()),
            );
        }
        Ok(attributes)
    }
}

#[async_trait]
impl TrustedServiceInterface for HttpAttributeStore {
    async fn get_attributes_for_actor(
        &self,
        identities: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        let request = Lookup {
            identities: identities
                .iter()
                .map(|(key, value)| Identity { key, value })
                .collect(),
        };
        let url = self
            .url
            .join("v1/attributes")
            .map_err(|e| ServiceError::Internal(e.to_string()))?;
        let response = self
            .client
            .post(url)
            .json(&request)
            .send()
            .await
            .map_err(|e| {
                ServiceError::AttributesIndeterminate(format!("{}: lookup failed: {e}", self.id))
            })?;
        if response.status() != reqwest::StatusCode::OK {
            return Err(ServiceError::AttributesIndeterminate(format!(
                "{}: lookup rejected: HTTP {}",
                self.id,
                response.status()
            )));
        }
        if response.content_length().is_some_and(|len| len > 1_048_576) {
            return Err(ServiceError::AttributesIndeterminate(format!(
                "{}: response too large",
                self.id
            )));
        }
        let mut response = response;
        let mut body = Vec::new();
        while let Some(chunk) = response.chunk().await.map_err(|e| {
            ServiceError::AttributesIndeterminate(format!("{}: response read: {e}", self.id))
        })? {
            if chunk.len() > 1_048_576 - body.len() {
                return Err(ServiceError::AttributesIndeterminate(format!(
                    "{}: response too large",
                    self.id
                )));
            }
            body.extend_from_slice(&chunk);
        }
        let result: LookupResult = serde_json::from_slice(&body).map_err(|e| {
            ServiceError::AttributesIndeterminate(format!(
                "{}: invalid lookup response: {e}",
                self.id
            ))
        })?;
        self.map_result(result)
    }

    async fn flush(&self) -> Result<(), ServiceError> {
        self.revision.store(next_revision(), Ordering::Release);
        Ok(())
    }

    fn current_revision(&self) -> u64 {
        self.revision.load(Ordering::Acquire)
    }
    fn get_source_id(&self) -> &str {
        &self.id
    }
}

#[cfg(test)]
#[path = "http_attribute_store_tls_tests.rs"]
mod tls_tests;

#[cfg(test)]
#[path = "http_attribute_store_recovery_tests.rs"]
mod recovery_tests;

#[cfg(test)]
#[path = "http_attribute_store_parity_tests.rs"]
mod parity_tests;

#[cfg(test)]
mod tests {
    use super::*;
    use zpr::policy_types::parse_attribute_mapping;

    fn store() -> HttpAttributeStore {
        HttpAttributeStore {
            id: "directory".to_string(),
            mapper: AttributeMapper {
                mappings: vec![
                    parse_attribute_mapping("department -> user.department").unwrap(),
                    parse_attribute_mapping("dept -> user.department").unwrap(),
                ],
            },
            url: "https://localhost/".parse().unwrap(),
            client: reqwest::Client::new(),
            ttl: Duration::from_secs(3600),
            revision: AtomicU64::new(next_revision()),
        }
    }

    #[test]
    fn source_deadline_may_be_omitted_but_must_be_an_integer_when_present() {
        let legacy: LookupResult = serde_json::from_str(r#"{"attributes":{}}"#).unwrap();
        assert_eq!(legacy.expires_unix, None);
        let bounded: LookupResult =
            serde_json::from_str(r#"{"attributes":{},"expires_unix":1792886400}"#).unwrap();
        assert_eq!(bounded.expires_unix, Some(1792886400));
        for value in ["null", "false", "-1", "1.5", "\"1792886400\"", "[]", "{}"] {
            let json = format!(r#"{{"attributes":{{}},"expires_unix":{value}}}"#);
            assert!(
                serde_json::from_str::<LookupResult>(&json).is_err(),
                "{json}"
            );
        }
        assert!(
            serde_json::from_str::<LookupResult>(
                r#"{"attributes":{},"expires_unix":null,"expires_unix":1792886400}"#
            )
            .is_err()
        );
    }

    #[tokio::test]
    async fn duplicate_response_attribute_names_are_indeterminate() {
        for reply in [
            r#"{"attributes":{"department":["sales"],"department":["engineering"]}}"#,
            r#"{"attributes":{"department":["engineering"],"department":["sales"]}}"#,
            r#"{"attributes":{"department":["engineering"],"department":["engineering"]}}"#,
            r#"{"attributes":{"department":["sales"],"depart\u006dent":["engineering"]}}"#,
            r#"{"attributes":{"ignored":["one"],"ignored":["two"]}}"#,
            r#"{"attributes":{},"attributes":{"department":["engineering"]}}"#,
        ] {
            match lookup_over_verified_https(reply.into()).await {
                Err(ServiceError::AttributesIndeterminate(message)) => {
                    assert!(message.contains("directory"), "{message}");
                    assert!(message.contains("duplicate"), "{message}");
                }
                other => panic!("duplicate names must fail: {reply}: {other:?}"),
            }
        }
    }

    #[tokio::test]
    async fn distinct_response_aliases_with_identical_claims_remain_valid() {
        let attributes = lookup_over_verified_https(
            r#"{"attributes":{"department":["engineering"],"ignored":["value"]}}"#.into(),
        )
        .await
        .unwrap();
        assert_eq!(attributes.len(), 1);
        assert_eq!(attributes[0].get_key(), "user.department");
        assert!(attributes[0].value_has("engineering"));

        let result: LookupResult = serde_json::from_str(
            r#"{"attributes":{"department":["engineering"],"dept":["engineering"]}}"#,
        )
        .unwrap();
        let mapped = store().map_result(result).unwrap();
        assert_eq!(mapped.len(), 1);
        assert!(mapped[0].value_has("engineering"));
    }

    #[tokio::test]
    async fn malformed_https_responses_are_indeterminate_not_legacy_observations() {
        for reply in [
            r#"{"attributes":{},"expires_unix":null}"#,
            r#"{"attributes":{"department":["engineering"]},"expires_unix":null}"#,
            r#"{"attributes":{},"expires_unix":"tomorrow"}"#,
            r#"{"attributes":{},"expires_unix":false}"#,
            r#"{"attributes":{},"expires_unix":18446744073709551616}"#,
            r#"{"attributes":{},"unknown":true}"#,
            r#"{"attributes":null}"#,
            r#"{"attributes":{}"#,
        ] {
            let result = lookup_over_verified_https(reply.into()).await;
            match result {
                Err(ServiceError::AttributesIndeterminate(message)) => {
                    assert!(message.contains("directory"), "{message}");
                    assert!(message.contains("response"), "{message}");
                }
                other => panic!("malformed response must be indeterminate: {reply}: {other:?}"),
            }
        }
    }

    #[test]
    fn source_validity_caps_attributes_and_empty_observations() {
        let source = store();
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs();
        for attributes in [
            BTreeMap::new(),
            BTreeMap::from([("department".into(), vec!["engineering".into()])]),
        ] {
            let attrs = source
                .map_result(LookupResult {
                    attributes,
                    expires_unix: Some(now + 120),
                })
                .unwrap();
            assert!(!attrs.is_empty());
            assert!(
                attrs.iter().all(|attr| {
                    attr.get_expires() == UNIX_EPOCH + Duration::from_secs(now + 120)
                })
            );
            assert!(attrs.iter().any(super::super::is_source_freshness));
        }
        let attrs = source
            .map_result(LookupResult {
                attributes: BTreeMap::new(),
                expires_unix: Some(now + 7200),
            })
            .unwrap();
        assert!(attrs[0].get_expires() <= SystemTime::now() + source.ttl);
        assert!(attrs[0].get_expires() > UNIX_EPOCH + Duration::from_secs(now + 3500));
    }

    #[test]
    fn invalid_source_deadlines_and_forged_metadata_are_rejected() {
        let source = store();
        for deadline in [0, 1, u64::MAX] {
            assert!(
                source
                    .map_result(LookupResult {
                        attributes: BTreeMap::new(),
                        expires_unix: Some(deadline),
                    })
                    .is_err()
            );
        }
        assert!(parse_attribute_mapping("forged -> zpr.source_freshness").is_err());
        for json in [
            r#"{"attributes":{},"expires_unix":-1}"#,
            r#"{"attributes":{},"expires_unix":"tomorrow"}"#,
        ] {
            assert!(serde_json::from_str::<LookupResult>(json).is_err());
        }
    }

    #[test]
    fn maps_source_names_and_fails_closed_on_alias_conflict() {
        let source = store();
        let attrs = source
            .map_result(LookupResult {
                expires_unix: None,
                attributes: BTreeMap::from([
                    ("department".into(), vec!["engineering".into()]),
                    ("ignored".into(), vec!["secret".into()]),
                ]),
            })
            .unwrap();
        assert_eq!(attrs.len(), 1);
        assert_eq!(attrs[0].get_key(), "user.department");
        assert!(attrs[0].value_has("engineering"));
        assert!(
            source
                .map_result(LookupResult {
                    expires_unix: None,
                    attributes: BTreeMap::from([
                        ("department".into(), vec!["engineering".into()]),
                        ("dept".into(), vec!["sales".into()]),
                    ])
                })
                .is_err()
        );
        assert!(
            source
                .map_result(LookupResult {
                    expires_unix: None,
                    attributes: BTreeMap::from([(
                        "department".into(),
                        vec!["engineering".into(), "sales".into()],
                    )]),
                })
                .is_err()
        );
    }

    async fn lookup_over_verified_https(reply: String) -> Result<Vec<Attribute>, ServiceError> {
        lookup_raw_verified_https(format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{reply}",
            reply.len()
        ).into_bytes()).await.0
    }

    async fn lookup_raw_verified_https(
        reply: Vec<u8>,
    ) -> (Result<Vec<Attribute>, ServiceError>, bool) {
        lookup_raw_verified_https_with_mappings(reply, &["department -> user.department"]).await
    }

    pub(super) async fn lookup_raw_verified_https_with_mappings(
        reply: Vec<u8>,
        mappings: &[&str],
    ) -> (Result<Vec<Attribute>, ServiceError>, bool) {
        use std::sync::Arc;
        use tokio::io::{AsyncReadExt, AsyncWriteExt};
        use tokio_rustls::rustls::{ServerConfig, pki_types::PrivateKeyDer};

        let certified = rcgen::generate_simple_self_signed(vec!["localhost".into()]).unwrap();
        let dir = tempfile::tempdir().unwrap();
        let cert_path = dir.path().join("cert.pem");
        let key_path = dir.path().join("key.pem");
        std::fs::write(&cert_path, certified.cert.pem()).unwrap();
        std::fs::write(&key_path, certified.signing_key.serialize_pem()).unwrap();
        let cert = certified.cert.der().clone();
        let key = PrivateKeyDer::try_from(certified.signing_key.serialize_der()).unwrap();
        let server = ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(vec![cert], key)
            .unwrap();
        let acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let address = listener.local_addr().unwrap();
        let task = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let mut stream = acceptor.accept(stream).await.unwrap();
            let mut bytes = vec![0; 8192];
            let mut request = Vec::new();
            loop {
                let size = stream.read(&mut bytes).await.unwrap();
                assert!(size > 0, "lookup request ended before its body");
                request.extend_from_slice(&bytes[..size]);
                if let Some(end) = request.windows(4).position(|part| part == b"\r\n\r\n") {
                    let headers = String::from_utf8_lossy(&request[..end]);
                    let length: usize = headers
                        .lines()
                        .find_map(|line| {
                            line.to_ascii_lowercase()
                                .strip_prefix("content-length:")
                                .map(|value| value.trim().parse().unwrap())
                        })
                        .unwrap();
                    if request.len() >= end + 4 + length {
                        assert!(headers.starts_with("POST /v1/attributes HTTP/1.1"));
                        break;
                    }
                }
            }
            // Rejections may close the connection before the entire test body is sent.
            let _ = stream.write_all(&reply).await;
            let _ = stream.shutdown().await;
            drop(stream);
            if let Ok(Ok((stream, _))) =
                tokio::time::timeout(Duration::from_millis(200), listener.accept()).await
            {
                let mut stream = acceptor.accept(stream).await.unwrap();
                let _ = stream.read(&mut bytes).await.unwrap();
                stream.write_all(b"HTTP/1.1 200 OK\r\nContent-Length: 17\r\nConnection: close\r\n\r\n{\"attributes\":{}}").await.unwrap();
                true
            } else {
                false
            }
        });
        let settings = TrustedServiceHttpConfig {
            url: format!("https://localhost:{}", address.port()),
            ca_cert: cert_path.clone(),
            client_cert: cert_path,
            client_key: key_path,
            token_verification_key_file: None,
        };
        let client = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper {
                mappings: mappings
                    .iter()
                    .map(|mapping| parse_attribute_mapping(mapping).unwrap())
                    .collect(),
            },
            Duration::from_secs(3600),
            &settings,
        )
        .unwrap();
        let result = client
            .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
            .await;
        (result, task.await.unwrap())
    }

    #[tokio::test]
    async fn redirects_never_relocate_the_trusted_source_lookup() {
        for status in [301, 302, 303, 307, 308] {
            let (result, followed) = lookup_raw_verified_https(format!(
                "HTTP/1.1 {status} Redirect\r\nLocation: /other\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
            ).into_bytes()).await;
            assert!(!followed, "followed redirect {status}");
            assert!(matches!(
                result,
                Err(ServiceError::AttributesIndeterminate(_))
            ));
        }
    }

    #[tokio::test]
    async fn complete_observations_require_http_ok() {
        for status in [202, 206, 207, 300, 304, 400, 401, 403, 429, 500, 503] {
            let body = r#"{"attributes":{"department":["engineering"]}}"#;
            let (result, _) = lookup_raw_verified_https(format!(
                "HTTP/1.1 {status} Rejected\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}", body.len()
            ).into_bytes()).await;
            assert!(
                matches!(result, Err(ServiceError::AttributesIndeterminate(_))),
                "accepted HTTP {status}: {result:?}"
            );
        }
    }

    #[tokio::test]
    async fn verified_https_enforces_exact_and_streamed_response_limits() {
        for chunked in [false, true] {
            for size in [1_048_575, 1_048_576, 1_048_577] {
                let mut body = br#"{"attributes":{}}"#.to_vec();
                body.resize(size, b' ');
                let wire = if chunked {
                    let mut wire = b"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n".to_vec();
                    for chunk in body.chunks(16384) {
                        wire.extend_from_slice(format!("{:x}\r\n", chunk.len()).as_bytes());
                        wire.extend_from_slice(chunk);
                        wire.extend_from_slice(b"\r\n");
                    }
                    wire.extend_from_slice(b"0\r\n\r\n");
                    wire
                } else {
                    let mut wire = format!(
                        "HTTP/1.1 200 OK\r\nContent-Length: {size}\r\nConnection: close\r\n\r\n"
                    )
                    .into_bytes();
                    wire.extend_from_slice(&body);
                    wire
                };
                let (result, _) = lookup_raw_verified_https(wire).await;
                if size <= 1_048_576 {
                    assert!(result.unwrap().is_empty(), "{size}/{chunked}");
                } else {
                    assert!(
                        matches!(result, Err(ServiceError::AttributesIndeterminate(_))),
                        "{size}/{chunked}"
                    );
                }
            }
        }
    }

    #[tokio::test]
    async fn malformed_transport_text_and_truncation_are_indeterminate() {
        let mut invalid_utf8 = br#"{"attributes":{"department":[""#.to_vec();
        invalid_utf8.push(0xff);
        invalid_utf8.extend_from_slice(br#""]}}"#);
        for body in [
            invalid_utf8,
            br#"{"attributes":{"department":["\ud800"]}}"#.to_vec(),
            br#"{"attributes":{"department":["\udfff"]}}"#.to_vec(),
        ] {
            let mut wire = format!(
                "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
                body.len()
            )
            .into_bytes();
            wire.extend_from_slice(&body);
            assert!(matches!(
                lookup_raw_verified_https(wire).await.0,
                Err(ServiceError::AttributesIndeterminate(_))
            ));
        }
        assert!(matches!(
            lookup_raw_verified_https(b"HTTP/1.1 200 OK\r\nContent-Length: 1000\r\nConnection: close\r\n\r\n{\"attributes\":{}}".to_vec()).await.0,
            Err(ServiceError::AttributesIndeterminate(_))
        ));
    }

    #[tokio::test]
    async fn sends_lookup_over_verified_https() {
        let attributes =
            lookup_over_verified_https(r#"{"attributes":{"department":["engineering"]}}"#.into())
                .await
                .unwrap();
        assert_eq!(attributes.len(), 1);
        assert!(attributes[0].value_has("engineering"));
    }

    #[tokio::test]
    async fn verified_https_carries_empty_result_deadline_and_rejects_expiry() {
        let deadline = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs()
            + 120;
        let attributes = lookup_over_verified_https(format!(
            r#"{{"attributes":{{}},"expires_unix":{deadline}}}"#
        ))
        .await
        .unwrap();
        assert_eq!(attributes.len(), 1);
        assert!(super::super::is_source_freshness(&attributes[0]));
        assert_eq!(
            attributes[0].get_expires(),
            UNIX_EPOCH + Duration::from_secs(deadline)
        );
        assert!(matches!(
            lookup_over_verified_https(r#"{"attributes":{},"expires_unix":1}"#.into()).await,
            Err(ServiceError::AttributesIndeterminate(_))
        ));
    }
}
