//! Auth-code exchange for policy-declared adapter authentication services.

use std::collections::{BTreeMap, HashSet};
use std::net::{IpAddr, SocketAddr};
use std::path::Path;
use std::sync::Arc;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use jsonwebtoken::{Algorithm, DecodingKey, Validation, decode};
use libeval::attribute::{Attribute, AttributeSource};
use reqwest::redirect::Policy;
use serde::{Deserialize, Serialize};
use serde_json::Value;
use zpr::policy_types::{AttrMapping, ServiceType};
use zpr::vsapi_types::AuthCodeBlob;

use crate::assembly::Assembly;
use crate::config::{DEFAULT_AUTH_EXPIRATION, TrustedServiceHttpConfig};
use crate::error::ServiceError;

const BAS_ISSUER: &str = "zpr/bas";
const ZPR_AUDIENCE: &str = "zpr";
const VALIDATION2_API: &str = "validation/2";
const TOKEN_PATH: &str = "token";
const MAX_TOKEN_RESPONSE_BYTES: usize = 1 << 20;

#[derive(Debug)]
pub struct AuthenticatedAdapter {
    pub client_id: String,
    pub auth_service_id: String,
    pub expires_at: SystemTime,
    pub claims: Vec<Attribute>,
}

#[derive(Serialize)]
struct TokenRequest<'a> {
    grant_type: &'static str,
    code: &'a str,
    client_id: &'a str,
    redirect_url: &'static str,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct TokenResponse {
    access_token: Option<String>,
    token_type: Option<String>,
    expires_in: Option<u64>,
    refresh_token: Option<String>,
    error: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
struct BasClaims {
    iss: String,
    aud: String,
    sub: String,
    exp: u64,
    iat: u64,
    jti: String,
    #[serde(flatten)]
    private: BTreeMap<String, Value>,
}

/// Redeem a BAS one-time code only when the issuer is currently advertised by
/// the requested authentication actor and policy.
pub async fn authenticate(
    asm: &Arc<Assembly>,
    auth_code: &AuthCodeBlob,
) -> Result<AuthenticatedAdapter, ServiceError> {
    if !auth_code.pkce.is_empty() {
        return Err(ServiceError::AuthenticationFailed(
            "BAS auth-code PKCE is not supported".into(),
        ));
    }
    let auth_services = asm.actor_mgr.get_auth_services_list(asm.clone()).await?;
    if !auth_services
        .iter()
        .any(|service| service.zpr_addr == auth_code.asa_addr)
    {
        return Err(ServiceError::AuthenticationFailed(
            "auth-code issuer is not a connected policy-authorized authentication service".into(),
        ));
    }

    let offered = asm
        .actor_mgr
        .list_services_for_actor(&auth_code.asa_addr)
        .await?;
    let policy = asm.policy_mgr.get_current();
    let mut validation_services = policy
        .list_services()
        .into_iter()
        .filter(|service| offered.iter().any(|id| id == &service.id))
        .filter_map(|service| match &service.kind {
            ServiceType::Trusted(api) if api == VALIDATION2_API => Some(service),
            _ => None,
        });
    let validation_service = validation_services.next().ok_or_else(|| {
        ServiceError::AuthenticationFailed(
            "auth-code issuer has no registered validation/2 service".into(),
        )
    })?;
    if validation_services.next().is_some() {
        return Err(ServiceError::AuthenticationFailed(
            "auth-code issuer has ambiguous validation/2 services".into(),
        ));
    }
    let trusted_service = policy
        .trusted_service_by_id(&validation_service.id)
        .ok_or_else(|| {
            ServiceError::AuthenticationFailed(
                "validation/2 service has no signed claim-mapping record".into(),
            )
        })?;
    let client_config = asm
        .config
        .trusted_service_http
        .get(&validation_service.id)
        .ok_or_else(|| {
            ServiceError::TrustedServiceInit(format!(
                "missing HTTPS and token-key configuration for authentication service {}",
                validation_service.id
            ))
        })?;
    let endpoint = validation2_endpoint(
        client_config,
        &auth_code.asa_addr,
        &validation_service.endpoints,
    )?;

    let access_token = exchange_code(
        client_config,
        &endpoint,
        auth_code.asa_addr,
        &auth_code.code,
        &auth_code.client_id,
    )
    .await?;
    let signing_key = load_verification_key(client_config.token_verification_key_file.as_deref())?;
    let claims = verify_token(&access_token, &signing_key, &auth_code.client_id)?;
    let token_expiration = UNIX_EPOCH + Duration::from_secs(claims.exp);
    let policy_expiration =
        SystemTime::now() + Duration::from_secs(u64::from(trusted_service.expiration_seconds));
    let expires_at = token_expiration
        .min(policy_expiration)
        .min(SystemTime::now() + DEFAULT_AUTH_EXPIRATION);
    if expires_at <= SystemTime::now() {
        return Err(ServiceError::AuthenticationFailed(
            "auth token has no remaining lifetime".into(),
        ));
    }
    let claims = map_claims(
        &validation_service.id,
        &trusted_service.returns_attrs,
        &claims.private,
        expires_at,
    )?;

    Ok(AuthenticatedAdapter {
        client_id: auth_code.client_id.clone(),
        auth_service_id: validation_service.id.clone(),
        expires_at,
        claims,
    })
}

fn validation2_endpoint(
    config: &TrustedServiceHttpConfig,
    auth_service_addr: &IpAddr,
    endpoints: &[zpr::policy_types::Scope],
) -> Result<reqwest::Url, ServiceError> {
    if endpoints.len() != 1 {
        return Err(ServiceError::Param(
            "validation/2 service must declare exactly one scope".into(),
        ));
    }
    let port = endpoints[0]
        .port
        .ok_or_else(|| ServiceError::Param("validation/2 service scope has no port".into()))?;
    let mut base = reqwest::Url::parse(&config.url)
        .map_err(|e| ServiceError::TrustedServiceInit(format!("invalid validation/2 URL: {e}")))?;
    if base.scheme() != "https"
        || base.host_str().is_none()
        || !base.username().is_empty()
        || base.password().is_some()
        || base.query().is_some()
        || base.fragment().is_some()
        || base.path() != "/"
    {
        return Err(ServiceError::TrustedServiceInit(
            "validation/2 URL must be an HTTPS origin".into(),
        ));
    }
    if base.port().is_some_and(|configured| configured != port) {
        return Err(ServiceError::TrustedServiceInit(
            "validation/2 URL port does not match the signed service scope".into(),
        ));
    }
    base.set_port(Some(port)).map_err(|_| {
        ServiceError::TrustedServiceInit("unable to set validation/2 service port".into())
    })?;
    let host = base.host_str().unwrap();
    if host
        .parse::<IpAddr>()
        .is_ok_and(|configured| configured != *auth_service_addr)
    {
        return Err(ServiceError::TrustedServiceInit(
            "validation/2 URL IP does not match the authenticated service address".into(),
        ));
    }
    base.join(TOKEN_PATH)
        .map_err(|e| ServiceError::TrustedServiceInit(format!("invalid token URL: {e}")))
}

async fn exchange_code(
    config: &TrustedServiceHttpConfig,
    endpoint: &reqwest::Url,
    provider_addr: IpAddr,
    code: &str,
    client_id: &str,
) -> Result<String, ServiceError> {
    let ca = reqwest::Certificate::from_pem(&std::fs::read(&config.ca_cert).map_err(|e| {
        ServiceError::TrustedServiceInit(format!("BAS CA certificate unavailable: {e}"))
    })?)
    .map_err(|e| ServiceError::TrustedServiceInit(format!("invalid BAS CA certificate: {e}")))?;
    let mut identity_pem = std::fs::read(&config.client_cert).map_err(|e| {
        ServiceError::TrustedServiceInit(format!("BAS client certificate unavailable: {e}"))
    })?;
    identity_pem.extend_from_slice(&std::fs::read(&config.client_key).map_err(|e| {
        ServiceError::TrustedServiceInit(format!("BAS client key unavailable: {e}"))
    })?);
    let identity = reqwest::Identity::from_pem(&identity_pem).map_err(|e| {
        ServiceError::TrustedServiceInit(format!("invalid BAS client identity: {e}"))
    })?;
    let host = endpoint
        .host_str()
        .expect("validated HTTPS origin has host");
    let port = endpoint
        .port_or_known_default()
        .expect("HTTPS has default port");
    let address = SocketAddr::new(endpoint_ip_for_provider(endpoint, provider_addr)?, port);
    let client = reqwest::Client::builder()
        .tls_certs_only([ca])
        .identity(identity)
        .resolve_to_addrs(host, &[address])
        .redirect(Policy::none())
        .timeout(Duration::from_secs(5))
        .build()
        .map_err(|e| ServiceError::TrustedServiceInit(format!("BAS HTTPS client: {e}")))?;
    let body = TokenRequest {
        grant_type: "authorization_code",
        code,
        client_id,
        redirect_url: "https://auth.zpr",
    };
    let mut response = client
        .post(endpoint.clone())
        .form(&body)
        .send()
        .await
        .map_err(|e| ServiceError::AuthenticationFailed(format!("BAS token exchange failed: {e}")))?
        .error_for_status()
        .map_err(|e| ServiceError::AuthenticationFailed(format!("BAS rejected auth code: {e}")))?;
    if response
        .content_length()
        .is_some_and(|length| length > MAX_TOKEN_RESPONSE_BYTES as u64)
    {
        return Err(ServiceError::AuthenticationFailed(
            "BAS token response too large".into(),
        ));
    }
    let mut bytes = Vec::new();
    while let Some(chunk) = response.chunk().await.map_err(|e| {
        ServiceError::AuthenticationFailed(format!("BAS token response failed: {e}"))
    })? {
        if chunk.len() > MAX_TOKEN_RESPONSE_BYTES - bytes.len() {
            return Err(ServiceError::AuthenticationFailed(
                "BAS token response too large".into(),
            ));
        }
        bytes.extend_from_slice(&chunk);
    }
    let response: TokenResponse = serde_json::from_slice(&bytes).map_err(|e| {
        ServiceError::AuthenticationFailed(format!("invalid BAS token response: {e}"))
    })?;
    if let Some(error) = response.error {
        return Err(ServiceError::AuthenticationFailed(format!(
            "BAS token exchange failed: {error}"
        )));
    }
    if response
        .token_type
        .as_deref()
        .is_none_or(|kind| !kind.eq_ignore_ascii_case("bearer"))
        || response.expires_in.is_none_or(|expires_in| expires_in == 0)
    {
        return Err(ServiceError::AuthenticationFailed(
            "BAS returned an invalid token response".into(),
        ));
    }
    let _refresh_token = response.refresh_token;
    response
        .access_token
        .filter(|token| !token.is_empty())
        .ok_or_else(|| ServiceError::AuthenticationFailed("BAS returned no access token".into()))
}

fn endpoint_ip_for_provider(
    endpoint: &reqwest::Url,
    provider_addr: IpAddr,
) -> Result<IpAddr, ServiceError> {
    if let Some(configured_ip) = endpoint
        .host_str()
        .and_then(|host| host.parse::<IpAddr>().ok())
        && configured_ip != provider_addr
    {
        return Err(ServiceError::TrustedServiceInit(
            "validation/2 URL IP does not match the auth service ZPR address".into(),
        ));
    }
    Ok(provider_addr)
}

fn load_verification_key(path: Option<&Path>) -> Result<Vec<u8>, ServiceError> {
    let path = path.ok_or_else(|| {
        ServiceError::TrustedServiceInit("validation/2 service has no JWT verification key".into())
    })?;
    let metadata = std::fs::metadata(path).map_err(|e| {
        ServiceError::TrustedServiceInit(format!("BAS JWT verification key unavailable: {e}"))
    })?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if metadata.permissions().mode() & 0o077 != 0 {
            return Err(ServiceError::TrustedServiceInit(
                "BAS JWT verification key must not be accessible by group or other".into(),
            ));
        }
    }
    let key = std::fs::read(path).map_err(|e| {
        ServiceError::TrustedServiceInit(format!("BAS JWT verification key unavailable: {e}"))
    })?;
    if key.len() < 32 {
        return Err(ServiceError::TrustedServiceInit(
            "BAS JWT verification key must contain at least 32 bytes".into(),
        ));
    }
    Ok(key)
}

fn verify_token(token: &str, key: &[u8], client_id: &str) -> Result<BasClaims, ServiceError> {
    let mut validation = Validation::new(Algorithm::HS384);
    validation.set_issuer(&[BAS_ISSUER]);
    validation.set_audience(&[ZPR_AUDIENCE]);
    validation.leeway = 0;
    validation.required_spec_claims = HashSet::from([
        "iss".into(),
        "aud".into(),
        "sub".into(),
        "exp".into(),
        "iat".into(),
        "jti".into(),
    ]);
    let claims = decode::<BasClaims>(token, &DecodingKey::from_secret(key), &validation)
        .map_err(|e| ServiceError::AuthenticationFailed(format!("invalid BAS access token: {e}")))?
        .claims;
    if claims.sub != client_id || claims.jti.is_empty() {
        return Err(ServiceError::AuthenticationFailed(
            "BAS token subject does not match auth-code client".into(),
        ));
    }
    let now = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs();
    if claims.iat > now.saturating_add(60) {
        return Err(ServiceError::AuthenticationFailed(
            "BAS token was issued in the future".into(),
        ));
    }
    Ok(claims)
}

fn map_claims(
    service_id: &str,
    mappings: &[AttrMapping],
    private_claims: &BTreeMap<String, Value>,
    expires_at: SystemTime,
) -> Result<Vec<Attribute>, ServiceError> {
    let mut mapped: BTreeMap<String, (bool, bool, Vec<String>)> = BTreeMap::new();
    for mapping in mappings {
        let claim_name = format!("z/{}", mapping.service_attr_key);
        let Some(value) = private_claims.get(&claim_name) else {
            continue;
        };
        let values = match value {
            Value::String(value) if mapping.attr.is_multi_valued() => value
                .split(',')
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
                .collect(),
            Value::String(value) if mapping.attr.is_tag() && value.is_empty() => Vec::new(),
            Value::String(value) => vec![value.clone()],
            Value::Array(values) => values
                .iter()
                .map(|value| {
                    value.as_str().map(ToOwned::to_owned).ok_or_else(|| {
                        ServiceError::AuthenticationFailed(format!(
                            "BAS claim {claim_name} contains a non-string value"
                        ))
                    })
                })
                .collect::<Result<Vec<_>, _>>()?,
            _ => {
                return Err(ServiceError::AuthenticationFailed(format!(
                    "BAS claim {claim_name} has an unsupported value type"
                )));
            }
        };
        let target = mapping.attr.zpl_key();
        let value = (
            mapping.attr.is_tag(),
            mapping.attr.is_multi_valued(),
            values,
        );
        if let Some(previous) = mapped.get(&target) {
            if previous != &value {
                return Err(ServiceError::AuthenticationFailed(format!(
                    "BAS returned conflicting claims for {target}"
                )));
            }
        } else {
            mapped.insert(target, value);
        }
    }
    let source = AttributeSource::new(service_id.to_string());
    mapped
        .into_iter()
        .map(|(key, (is_tag, is_multi, values))| {
            let builder = source.builder(&key).expires(expires_at);
            if is_tag {
                Ok(builder.values(Vec::<String>::new()))
            } else if is_multi {
                Ok(builder.values(values))
            } else if values.len() == 1 {
                Ok(builder.value(values.into_iter().next().unwrap()))
            } else {
                Err(ServiceError::AuthenticationFailed(format!(
                    "BAS returned an invalid single-valued claim for {key}"
                )))
            }
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use jsonwebtoken::{EncodingKey, Header};
    use std::sync::Arc;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio_rustls::rustls::{ServerConfig, pki_types::PrivateKeyDer};
    use zpr::policy_types::parse_attribute_mapping;

    const TEST_KEY: &[u8] = b"test-only BAS HMAC verification key, at least 32 bytes";

    fn make_token(subject: &str, key: &[u8]) -> String {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_secs();
        let claims = BasClaims {
            iss: BAS_ISSUER.into(),
            aud: ZPR_AUDIENCE.into(),
            sub: subject.into(),
            exp: now + 600,
            iat: now,
            jti: "auth-test-token".into(),
            private: BTreeMap::from([
                ("z/color".into(), Value::String("green".into())),
                ("z/groups".into(), Value::String("engineering,ops".into())),
                ("z/active".into(), Value::String(String::new())),
                ("z/unmapped".into(), Value::String("ignored".into())),
            ]),
        };
        jsonwebtoken::encode(
            &Header::new(Algorithm::HS384),
            &claims,
            &EncodingKey::from_secret(key),
        )
        .unwrap()
    }

    #[test]
    fn validates_auth_service_token_and_subject() {
        let token = make_token("adapter.one", TEST_KEY);
        let claims = verify_token(&token, TEST_KEY, "adapter.one").unwrap();
        assert_eq!(claims.sub, "adapter.one");
        assert!(
            verify_token(
                &token,
                b"wrong verification key with enough bytes",
                "adapter.one"
            )
            .is_err()
        );
        assert!(verify_token(&token, TEST_KEY, "adapter.two").is_err());
    }

    #[test]
    fn maps_only_policy_declared_claims_with_token_expiration() {
        let mappings = vec![
            parse_attribute_mapping("color -> user.color").unwrap(),
            parse_attribute_mapping("groups -> user.group{}").unwrap(),
            parse_attribute_mapping("active -> #user.active").unwrap(),
        ];
        let private = BTreeMap::from([
            ("z/color".into(), Value::String("green".into())),
            ("z/groups".into(), Value::String("engineering,ops".into())),
            ("z/active".into(), Value::String(String::new())),
            ("z/unmapped".into(), Value::String("ignored".into())),
        ]);
        let expiry = SystemTime::now() + Duration::from_secs(600);
        let attributes = map_claims("bas", &mappings, &private, expiry).unwrap();

        assert_eq!(attributes.len(), 3);
        let color = attributes
            .iter()
            .find(|a| a.get_key() == "user.color")
            .unwrap();
        assert_eq!(color.get_value(), &vec!["green".to_string()]);
        assert_eq!(color.get_expires(), expiry);
        let groups = attributes
            .iter()
            .find(|a| a.get_key() == "user.group")
            .unwrap();
        assert!(groups.value_has_all(&["engineering".into(), "ops".into()]));
        assert!(
            attributes
                .iter()
                .any(|a| a.get_key() == "user.zpr.tag.active")
        );
    }

    #[tokio::test]
    async fn exchanges_code_over_tls_with_provider_ip_pinning() {
        let certified = rcgen::generate_simple_self_signed(vec!["bas.zpr.org".into()]).unwrap();
        let directory = tempfile::tempdir().unwrap();
        let cert_path = directory.path().join("bas.crt");
        let key_path = directory.path().join("bas.key");
        std::fs::write(&cert_path, certified.cert.pem()).unwrap();
        std::fs::write(&key_path, certified.signing_key.serialize_pem()).unwrap();

        let server = ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(
                vec![certified.cert.der().clone()],
                PrivateKeyDer::try_from(certified.signing_key.serialize_der()).unwrap(),
            )
            .unwrap();
        let acceptor = tokio_rustls::TlsAcceptor::from(Arc::new(server));
        let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
        let port = listener.local_addr().unwrap().port();
        let expected_token = make_token("adapter.one", TEST_KEY);
        let response_body = serde_json::json!({
            "access_token": expected_token,
            "token_type": "bearer",
            "expires_in": 600,
            "refresh_token": null,
            "error": null
        })
        .to_string();
        let server_task = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.unwrap();
            let mut stream = acceptor.accept(stream).await.unwrap();
            let mut request = Vec::new();
            let mut chunk = [0; 2048];
            let (header_end, content_length);
            loop {
                let size = stream.read(&mut chunk).await.unwrap();
                assert_ne!(size, 0);
                request.extend_from_slice(&chunk[..size]);
                let Some(end) = request.windows(4).position(|window| window == b"\r\n\r\n") else {
                    continue;
                };
                let headers = String::from_utf8_lossy(&request[..end]);
                let length = headers
                    .lines()
                    .find_map(|line| {
                        let (name, value) = line.split_once(':')?;
                        name.eq_ignore_ascii_case("content-length")
                            .then(|| value.trim().parse::<usize>().unwrap())
                    })
                    .unwrap();
                if request.len() >= end + 4 + length {
                    header_end = end;
                    content_length = length;
                    break;
                }
            }
            let headers = String::from_utf8_lossy(&request[..header_end]);
            assert!(headers.starts_with("POST /token HTTP/1.1"));
            let body =
                String::from_utf8_lossy(&request[header_end + 4..header_end + 4 + content_length]);
            assert!(body.contains("grant_type=authorization_code"));
            assert!(body.contains("code=one-time-code"));
            assert!(body.contains("client_id=adapter.one"));
            let response = response_body;
            stream
                .write_all(
                    format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{response}",
                        response.len()
                    )
                    .as_bytes(),
                )
                .await
                .unwrap();
        });

        let config = TrustedServiceHttpConfig {
            url: format!("https://bas.zpr.org:{port}"),
            ca_cert: cert_path.clone(),
            client_cert: cert_path,
            client_key: key_path,
            token_verification_key_file: None,
        };
        let endpoint = validation2_endpoint(
            &config,
            &"127.0.0.1".parse().unwrap(),
            &[zpr::policy_types::Scope {
                protocol: 6,
                flag: None,
                port: Some(port),
                port_range: None,
            }],
        )
        .unwrap();
        let token = exchange_code(
            &config,
            &endpoint,
            "127.0.0.1".parse().unwrap(),
            "one-time-code",
            "adapter.one",
        )
        .await
        .unwrap();
        assert_eq!(token, expected_token);
        assert_eq!(
            verify_token(&token, TEST_KEY, "adapter.one").unwrap().sub,
            "adapter.one"
        );
        server_task.await.unwrap();
    }
}
