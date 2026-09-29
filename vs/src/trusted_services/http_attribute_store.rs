//! Mutually authenticated HTTPS trusted-service lookup.

use std::collections::BTreeMap;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Duration;

use async_trait::async_trait;
use libeval::attribute::{Attribute, AttributeSource};
use serde::{Deserialize, Serialize};

use crate::config::TrustedServiceHttpConfig;
use crate::error::ServiceError;

use super::attribute_mapper::{AttrHint, AttributeMapper};
use super::{TrustedServiceInterface, next_revision};

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
    attributes: BTreeMap<String, Vec<String>>,
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
        let ca = reqwest::Certificate::from_pem(&std::fs::read(&config.ca_cert)?)
            .map_err(|e| ServiceError::TrustedServiceInit(format!("{id}: invalid CA: {e}")))?;
        let mut identity_pem = std::fs::read(&config.client_cert)?;
        identity_pem.extend_from_slice(&std::fs::read(&config.client_key)?);
        let identity = reqwest::Identity::from_pem(&identity_pem).map_err(|e| {
            ServiceError::TrustedServiceInit(format!("{id}: invalid client identity: {e}"))
        })?;
        let client = reqwest::Client::builder()
            .tls_certs_only([ca])
            .identity(identity)
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
        let source = AttributeSource::new(self.id.clone());
        let mut mapped: BTreeMap<String, (AttrHint, Vec<String>)> = BTreeMap::new();
        for (name, values) in result.attributes {
            let Some((key, hint)) = self.mapper.map_attribute(&name) else {
                continue;
            };
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
        Ok(mapped
            .into_iter()
            .map(|(key, (hint, values))| {
                let builder = source.builder(key).expires_in(self.ttl);
                match hint {
                    AttrHint::SingleValued => {
                        builder.value(values.first().cloned().unwrap_or_default())
                    }
                    AttrHint::MultiValued => builder.values(values),
                    AttrHint::Tag => builder.values(Vec::<String>::new()),
                }
            })
            .collect())
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
            })?
            .error_for_status()
            .map_err(|e| {
                ServiceError::AttributesIndeterminate(format!("{}: lookup rejected: {e}", self.id))
            })?;
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
        let result: LookupResult = serde_json::from_slice(&body)?;
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
    fn maps_source_names_and_fails_closed_on_alias_conflict() {
        let source = store();
        let attrs = source
            .map_result(LookupResult {
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
                    attributes: BTreeMap::from([(
                        "department".into(),
                        vec!["engineering".into(), "sales".into()],
                    )]),
                })
                .is_err()
        );
    }

    #[tokio::test]
    async fn sends_lookup_over_verified_https() {
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
            let size = stream.read(&mut bytes).await.unwrap();
            let request = String::from_utf8_lossy(&bytes[..size]);
            assert!(request.starts_with("POST /v1/attributes HTTP/1.1"));
            let reply = r#"{"attributes":{"department":["engineering"]}}"#;
            stream.write_all(format!("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{reply}", reply.len()).as_bytes()).await.unwrap();
        });
        let settings = TrustedServiceHttpConfig {
            url: format!("https://localhost:{}", address.port()),
            ca_cert: cert_path.clone(),
            client_cert: cert_path,
            client_key: key_path,
        };
        let client = HttpAttributeStore::new(
            "directory".into(),
            AttributeMapper {
                mappings: vec![parse_attribute_mapping("department -> user.department").unwrap()],
            },
            Duration::from_secs(3600),
            &settings,
        )
        .unwrap();
        let attributes = client
            .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
            .await
            .unwrap();
        assert_eq!(attributes.len(), 1);
        assert!(attributes[0].value_has("engineering"));
        task.await.unwrap();
    }
}
