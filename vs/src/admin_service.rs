//! HTTPS admin service implementation.
use std::collections::{HashMap, HashSet};
use std::net::{IpAddr, SocketAddr};
use std::path::Path;
use std::sync::Arc;

use axum::{
    Extension,
    Json,
    Router,
    //routing::post,
    extract::{Json as EJson, Path as EPath, Query, Request, State},
    //extract::Form,
    http::StatusCode,
    middleware::{self, Next},
    response::{IntoResponse, Response},
    //response::Response,
    routing::{delete, get, post},
};

use hyper::body::Incoming;
use hyper_util::rt::{TokioExecutor, TokioIo};
use tower_service::Service;

use zpr::policy_types::{NetAddr, NetworkHost, PolicyBundle, Scope};
use zpr::vsapi_types::{DockPepType, KeyFormat, KeySet, Visa};

use libeval::attribute::{Attribute, ROLE_NODE, key};
use rustls::ServerConfig;
use rustls::pki_types::PrivateKeyDer;
use serde::{Deserialize, Serialize};
use std::time::SystemTime;
use tokio::net::TcpListener;
use tokio_rustls::TlsAcceptor;
use tracing::{debug, error, info, warn};

use crate::admin_apikeys::Permission;
use crate::apikey::ApiKey;
use crate::assembly::Assembly;
use crate::counters::CounterType;
use crate::db::{PolicyOperationReceipt, PolicyOperationRequest, Role};
use crate::error::ServiceError;
use crate::event_mgr::VsEvent;
use crate::logging::targets::ADMIN;
use crate::policy_mgr::DEFAULT_POLICY_ID;
use crate::visa_mgr::VisaMgr;

use zpr::vsapi_types::vsapi_ip_number as ip_proto;

use admin_api_types::{
    ActorDescriptor, ApiAttribute, ApiKeyFormat, ApiKeySet, CnEntry, ConnectionType, DenyRecord,
    ListEntry, NamedListEntry, NetworkDetails, NodeConnection, NodeRecordBrief, Revokes,
    ServiceDescriptor, Stats, VisaDescriptor,
};

// Must use tokio RwLock here becuase we need state to be Send.
type SharedState = Arc<tokio::sync::RwLock<AdminState>>;

#[allow(dead_code)]
struct AdminState {
    asm: Arc<Assembly>,
}

#[derive(Deserialize, Debug)]
struct RoleFilter {
    role: Option<ActorRole>,
}

/// Query args for `GET /admin/visas/denies`. `since` is epoch milliseconds.
#[derive(Deserialize, Debug)]
struct DenyFilter {
    since: Option<u64>,
    limit: Option<usize>,
}

#[derive(Deserialize, Debug)]
#[serde(rename_all = "lowercase")]
enum ActorRole {
    Node,
    Adapter,
}

struct ProtocolDetails {
    /// Human readable protocol name, e.g. "TCP", "UDP", "ICMP"
    protocol: String,
    source_port: Option<u16>,
    dest_port: Option<u16>,
}

/// Blocking start of the admin server.
/// TODO: Do I need a handle or something to stop this cleanly?
pub async fn start_admin_server(
    key_file: &Path,
    cert_file: &Path,
    listen: SocketAddr,
    asm: &Arc<Assembly>,
) {
    debug!(target: ADMIN, "admin service starting");
    let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
    serve(
        rustls_tls_acceptor(key_file, cert_file),
        listen,
        shared_state,
    )
    .await;
}

impl AdminState {
    pub fn new(asm: Arc<Assembly>) -> Self {
        AdminState { asm }
    }
}

const POLICY_OPERATION_ROUTE: &str = "/admin/policy-operations";
const POLICY_OPERATION_CURRENT_ROUTE: &str = "/admin/policy-operations/current";
const POLICY_APPROVAL_PURPOSE: &str = "visa-service-policy-operation";

/// Authoritative CAS inputs; an observation is not a reservation or approval.
#[derive(Debug, Deserialize, Serialize, PartialEq, Eq)]
struct GuardedPolicyCurrentState {
    target_id: String,
    organization: String,
    current_digest: String,
    current_vinst: u64,
}

/// Ed25519 over exact decoded payload bytes, using enrollment ApprovalReceipt's
/// standard-base64 envelope. This is not an enrollment or Control Room token.
#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct PolicyApprovalProof {
    payload: String,
    signature: String,
}

#[derive(Clone, Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct PolicyApprovalClaims {
    version: u32,
    key_id: String,
    issuer: String,
    audience: String,
    purpose: String,
    approver_issuer: String,
    approver_subject: String,
    organization: String,
    action: String,
    method: String,
    route: String,
    operation: PolicyOperationRequest,
    iat: u64,
    exp: u64,
    nonce: String,
}

#[derive(Deserialize, Serialize)]
#[serde(deny_unknown_fields)]
struct GuardedPolicyCommand {
    operation: PolicyOperationRequest,
    bundle: PolicyBundle,
    proof: PolicyApprovalProof,
}

#[derive(Serialize)]
struct PolicyApprovalBinding<'a> {
    signer_issuer: &'a str,
    approver_issuer: &'a str,
    approver_subject: &'a str,
    audience: &'a str,
    organization: &'a str,
    action: &'a str,
    operation: &'a PolicyOperationRequest,
}

fn approval_text(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 256
        && value.trim() == value
        && !value.chars().any(char::is_control)
}

fn approval_https(value: &str) -> bool {
    approval_text(value)
        && reqwest::Url::parse(value).is_ok_and(|url| {
            url.scheme() == "https"
                && url.host_str().is_some()
                && url.username().is_empty()
                && url.password().is_none()
                && url.query().is_none()
                && url.fragment().is_none()
        })
}

fn approval_audience(value: &str) -> bool {
    approval_https(value)
        && reqwest::Url::parse(value).is_ok_and(|url| url.origin().ascii_serialization() == value)
}

fn approval_base64(value: &str) -> Result<Vec<u8>, StatusCode> {
    use base64::{Engine, engine::general_purpose::STANDARD};
    let decoded = STANDARD.decode(value).map_err(|_| StatusCode::FORBIDDEN)?;
    if STANDARD.encode(&decoded) != value {
        return Err(StatusCode::FORBIDDEN);
    }
    Ok(decoded)
}

fn validate_policy_approval_config(
    config: &crate::config::GuardedPolicyConfig,
) -> Result<(), StatusCode> {
    if !approval_audience(&config.audience)
        || !approval_audience(&config.policy_service_audience)
        || config.audience == config.policy_service_audience
        || !approval_text(&config.target_id)
        || !approval_text(&config.organization)
        || config.keys.is_empty()
        || config.grants.is_empty()
    {
        return Err(StatusCode::SERVICE_UNAVAILABLE);
    }
    let mut ids = HashSet::new();
    for key in &config.keys {
        if !approval_text(&key.key_id)
            || !ids.insert(&key.key_id)
            || !approval_https(&key.issuer)
            || !approval_base64(&key.public_key).is_ok_and(|public| public.len() == 32)
            || key.expires_unix == 0
        {
            return Err(StatusCode::SERVICE_UNAVAILABLE);
        }
    }
    for grant in &config.grants {
        if !approval_https(&grant.issuer)
            || !approval_text(&grant.subject)
            || grant.organization != config.organization
            || grant.actions.is_empty()
            || grant
                .actions
                .iter()
                .any(|action| !matches!(action.as_str(), "install" | "rollback"))
        {
            return Err(StatusCode::SERVICE_UNAVAILABLE);
        }
    }
    Ok(())
}

fn verify_policy_approval(
    config: &crate::config::GuardedPolicyConfig,
    proof: &PolicyApprovalProof,
    operation: &PolicyOperationRequest,
    candidate: &[u8],
    now: u64,
) -> Result<PolicyApprovalClaims, StatusCode> {
    use openssl::pkey::{Id, PKey};
    use openssl::sign::Verifier;

    validate_policy_approval_config(config)?;
    if proof.payload.len() > 8192 || proof.signature.len() != 88 {
        return Err(StatusCode::FORBIDDEN);
    }
    let payload = approval_base64(&proof.payload)?;
    let signature = approval_base64(&proof.signature)?;
    let claims: PolicyApprovalClaims =
        serde_json::from_slice(&payload).map_err(|_| StatusCode::FORBIDDEN)?;
    let trusted = config
        .keys
        .iter()
        .find(|key| key.key_id == claims.key_id && key.issuer == claims.issuer)
        .ok_or(StatusCode::FORBIDDEN)?;
    if trusted.expires_unix <= now || claims.exp > trusted.expires_unix {
        return Err(StatusCode::FORBIDDEN);
    }
    let public = approval_base64(&trusted.public_key)?;
    let key = PKey::public_key_from_raw_bytes(&public, Id::ED25519)
        .map_err(|_| StatusCode::SERVICE_UNAVAILABLE)?;
    if signature.len() != 64
        || !Verifier::new_without_digest(&key)
            .and_then(|mut verifier| verifier.verify_oneshot(&signature, &payload))
            .map_err(|_| StatusCode::FORBIDDEN)?
    {
        return Err(StatusCode::FORBIDDEN);
    }
    if claims.version != 1
        || claims.audience != config.audience
        || claims.purpose != POLICY_APPROVAL_PURPOSE
        || claims.organization != config.organization
        || claims.operation.target_id != config.target_id
        || claims.operation != *operation
        || claims.operation.candidate_digest != hex::encode(openssl::sha::sha256(candidate))
        || claims.method != "POST"
        || claims.route != POLICY_OPERATION_ROUTE
        || !matches!(claims.action.as_str(), "install" | "rollback")
        || claims.iat == 0
        || claims.iat > now
        || claims.exp <= now
        || claims.exp <= claims.iat
        || claims.exp - claims.iat > 300
        || !approval_text(&claims.nonce)
        || claims.nonce.len() < 26
        || !claims
            .nonce
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || c == b'-' || c == b'_')
        || !config.grants.iter().any(|grant| {
            grant.issuer == claims.approver_issuer
                && grant.subject == claims.approver_subject
                && grant.organization == claims.organization
                && grant.actions.contains(&claims.action)
        })
    {
        return Err(StatusCode::FORBIDDEN);
    }
    Ok(claims)
}

/// GET /admin/policy-operations/current requires a read-capable API key and
/// valid guarded configuration. No approval proof is needed for this read.
async fn guarded_policy_current(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> Result<Json<GuardedPolicyCurrentState>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    let rstate = state.read().await;
    let config = rstate.asm.config.guarded_policy.as_ref().ok_or_else(|| {
        warn!(target: ADMIN, "guarded current-policy read requires guarded configuration");
        StatusCode::SERVICE_UNAVAILABLE
    })?;
    validate_policy_approval_config(config).map_err(|status| {
        error!(target: ADMIN, "guarded current-policy configuration is invalid");
        status
    })?;
    let now = SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|_| StatusCode::SERVICE_UNAVAILABLE)?
        .as_secs();
    if !config.keys.iter().any(|key| key.expires_unix > now) {
        error!(target: ADMIN, "guarded current-policy trust has expired");
        return Err(StatusCode::SERVICE_UNAVAILABLE);
    }
    let repo = crate::db::PolicyRepo::new(rstate.asm.state_db.clone());
    let current = repo
        .get_current_identifier()
        .await
        .map_err(|error| {
            error!(target: ADMIN, "guarded current-policy identifier read failed: {error}");
            StatusCode::SERVICE_UNAVAILABLE
        })?
        .ok_or_else(|| {
            error!(target: ADMIN, "guarded current-policy state is absent");
            StatusCode::SERVICE_UNAVAILABLE
        })?;
    match repo.current_matches(&current.phash, current.vinst).await {
        Ok(true) => Ok(Json(GuardedPolicyCurrentState {
            target_id: config.target_id.clone(),
            organization: config.organization.clone(),
            current_digest: current.phash,
            current_vinst: current.vinst,
        })),
        result => {
            error!(target: ADMIN, "guarded current-policy readback is unavailable or changed: {result:?}");
            Err(StatusCode::SERVICE_UNAVAILABLE)
        }
    }
}

async fn guarded_policy_operation(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    uri: axum::http::Uri,
    EJson(command): EJson<GuardedPolicyCommand>,
) -> Result<Json<PolicyOperationReceipt>, StatusCode> {
    if !perm.can_write() {
        return Err(StatusCode::FORBIDDEN);
    }
    if uri
        .path_and_query()
        .is_none_or(|path| path.as_str() != POLICY_OPERATION_ROUTE)
    {
        return Err(StatusCode::BAD_REQUEST);
    }
    let rstate = state.read().await;
    let config = rstate
        .asm
        .config
        .guarded_policy
        .as_ref()
        .ok_or(StatusCode::SERVICE_UNAVAILABLE)?;
    let candidate = command
        .bundle
        .decode()
        .map_err(|_| StatusCode::BAD_REQUEST)?;
    let now = SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|_| StatusCode::SERVICE_UNAVAILABLE)?
        .as_secs();
    let claims =
        verify_policy_approval(config, &command.proof, &command.operation, &candidate, now)?;
    command
        .operation
        .validate()
        .map_err(|_| StatusCode::BAD_REQUEST)?;
    let binding = PolicyApprovalBinding {
        signer_issuer: &claims.issuer,
        approver_issuer: &claims.approver_issuer,
        approver_subject: &claims.approver_subject,
        audience: &claims.audience,
        organization: &claims.organization,
        action: &claims.action,
        operation: &claims.operation,
    };
    let approval =
        serde_json::to_string(&binding).map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let nonce_hash = hex::encode(openssl::sha::sha256(
        &serde_json::to_vec(&(&claims.issuer, &claims.key_id, &claims.nonce))
            .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?,
    ));
    let result = rstate
        .asm
        .state_db
        .authorize_policy_operation(
            &format!("policy:approval-nonces:{nonce_hash}"),
            &format!("policy:approvals:{}", command.operation.operation_id),
            &approval,
        )
        .await
        .map_err(|error| {
            error!(target: ADMIN, "policy approval persistence failed: {error}");
            StatusCode::SERVICE_UNAVAILABLE
        })?;
    if result != 0 {
        return Err(StatusCode::CONFLICT);
    }
    let receipt = rstate
        .asm
        .policy_mgr
        .apply_policy_operation(&command.operation, candidate)
        .await
        .map_err(|error| {
            error!(target: ADMIN, "guarded policy operation failed: {error}");
            StatusCode::CONFLICT
        })?;
    if receipt.status == crate::db::PolicyOperationStatus::Succeeded {
        if let Err(error) = rstate
            .asm
            .event_mgr
            .record_event(VsEvent::PolicyUpdated(
                receipt
                    .applied_vinst
                    .ok_or(StatusCode::INTERNAL_SERVER_ERROR)?,
            ))
            .await
        {
            error!(target: ADMIN, "guarded policy applied but notification failed: {error}");
            return Err(StatusCode::SERVICE_UNAVAILABLE);
        }
    }
    Ok(Json(receipt))
}

fn rustls_tls_acceptor(key_file: &Path, cert_file: &Path) -> TlsAcceptor {
    let cert_pem = std::fs::read(cert_file).expect("failed to read admin cert file");
    let certs: Vec<_> = rustls_pemfile::certs(&mut &cert_pem[..])
        .collect::<Result<_, _>>()
        .expect("failed to parse admin cert PEM");

    let key_pem = std::fs::read(key_file).expect("failed to read admin key file");
    let key: PrivateKeyDer = rustls_pemfile::private_key(&mut &key_pem[..])
        .expect("failed to parse admin key PEM")
        .expect("no private key found in admin key file");

    let cfg = ServerConfig::builder()
        .with_no_client_auth()
        .with_single_cert(certs, key)
        .expect("failed to build rustls ServerConfig");

    TlsAcceptor::from(Arc::new(cfg))
}

/// Check the passed API key.
async fn validate_api_key(state: &SharedState, api_key: &str) -> Result<Permission, StatusCode> {
    let rstate = state.read().await;

    let apikey = ApiKey::parse(api_key).map_err(|_| StatusCode::UNAUTHORIZED)?;

    match rstate.asm.admin_api_keys.lookup_permission(&apikey) {
        Ok(Some(perm)) => Ok(perm),
        Ok(None) => Err(StatusCode::UNAUTHORIZED),
        Err(e) => {
            error!(target: ADMIN, "error validating API key {}: {}", apikey.key_id_hex(), e);
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
    }
}

// Check the API key and if all good inserts a Permission as a request extension.
async fn require_api_key(
    State(state): State<SharedState>,
    mut req: Request,
    next: Next,
) -> Result<Response, StatusCode> {
    let api_key = req
        .headers()
        .get("X-API-Key")
        .and_then(|hv| hv.to_str().ok())
        .ok_or(StatusCode::UNAUTHORIZED)?;

    // TODO: Add exponential backoff on unauthorizied requests.

    let perm = validate_api_key(&state, api_key).await?;
    req.extensions_mut().insert(perm);
    Ok(next.run(req).await)
}

fn admin_app(state: SharedState) -> Router {
    Router::new()
        .route("/admin/policies", get(get_policies))
        .route("/admin/policies/{capture}", get(get_policy))
        .route("/admin/policies/curr", get(get_curr_policy))
        .route("/admin/policies", post(install_policy))
        .route(POLICY_OPERATION_ROUTE, post(guarded_policy_operation))
        .route(POLICY_OPERATION_CURRENT_ROUTE, get(guarded_policy_current))
        .route("/admin/visas", get(get_visas).with_state(state.clone()))
        .route("/admin/visas/denies", get(get_denies))
        .route("/admin/visas/{capture}", get(get_visa))
        .route("/admin/visas/{capture}", delete(revoke_visa))
        .route("/admin/actors", get(get_actors).with_state(state.clone()))
        .route("/admin/actors/{capture}", get(get_actor))
        .route("/admin/actors/{capture}/visas", get(get_related_visas))
        .route("/admin/actors/{capture}", delete(revoke_actor))
        .route("/admin/nodes/{capture}/visas", get(get_visas_on_node))
        .route("/admin/services", get(get_services))
        .route("/admin/trusted-services", get(get_trusted_services))
        .route(
            "/admin/trusted-services/{capture}/control",
            get(get_trusted_source_control).put(set_trusted_source_control),
        )
        .route("/admin/services/{capture}", get(get_service))
        .route(
            "/admin/services/{capture}/cache",
            delete(flush_service_cache).get(trusted_reconciliation_status),
        )
        .route("/admin/authrevoke", get(get_revokes))
        .route("/admin/authrevoke/{capture}", get(get_revoke))
        .route("/admin/authrevoke/{capture}", post(add_revoke))
        .route("/admin/authrevoke/clear", post(clear_revokes))
        .route("/admin/authrevoke/{capture}", delete(remove_revoke))
        .route("/admin/network", get(get_network))
        .route("/admin/stats", get(get_stats))
        .layer(middleware::from_fn_with_state(
            state.clone(),
            require_api_key,
        ))
        .with_state(state.clone())
}

async fn serve(tls_acceptor: TlsAcceptor, listen: SocketAddr, state: SharedState) {
    let app = admin_app(state);
    let listener = TcpListener::bind(listen).await.unwrap_or_else(|e| {
        panic!("failed to bind admin https listener on {listen}: {e}");
    });
    info!(target: ADMIN, "admin https service listening on {listen} (TLS)");

    loop {
        let tower_service = app.clone();
        let tls_acceptor = tls_acceptor.clone();
        let (cnx, addr) = match listener.accept().await {
            Ok(v) => v,
            Err(e) => {
                // Exponential backoff saved for later.
                error!(target: ADMIN, "admin accept failed: {e}");
                tokio::time::sleep(std::time::Duration::from_millis(100)).await;
                continue;
            }
        };
        tokio::spawn(async move {
            let stream = match tls_acceptor.accept(cnx).await {
                Ok(stream) => stream,
                Err(e) => {
                    error!(target: ADMIN, "error during TLS handshake from {addr}, Error {e}");
                    return;
                }
            };

            let stream = TokioIo::new(stream);
            let hyper_service = hyper::service::service_fn(move |req: Request<Incoming>| {
                tower_service.clone().call(req)
            });

            let ret = hyper_util::server::conn::auto::Builder::new(TokioExecutor::new())
                .serve_connection_with_upgrades(stream, hyper_service)
                .await;

            if let Err(e) = ret {
                warn!(target: ADMIN, "error serving admin connection from {addr}: {}", e);
            }
        });
    }
}

/// TODO: Placeholder - Only returns ID(0) which is for the current policy.
async fn get_policies(
    Extension(perm): Extension<Permission>,
) -> Result<Json<ListEntry>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    debug!(target: ADMIN, "GET /admin/policies");
    let le = ListEntry {
        id: DEFAULT_POLICY_ID,
    };
    Ok(Json(le))
}

/// TODO: Handle multiple policies. Right now only accepts ID(0).
async fn get_policy(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(id): EPath<u64>,
) -> Result<Json<PolicyBundle>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    debug!(target: ADMIN, "GET /admin/policies/{id}");
    get_policy_by_id(&state.read().await.asm, id).await
}

async fn get_curr_policy(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> Result<Json<PolicyBundle>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    debug!(target: ADMIN, "GET /admin/policies/curr");
    get_policy_by_id(&state.read().await.asm, DEFAULT_POLICY_ID).await
}

/// Helper function used by [get_policy] and [get_curr_policy].
async fn get_policy_by_id(asm: &Assembly, id: u64) -> Result<Json<PolicyBundle>, StatusCode> {
    if id != DEFAULT_POLICY_ID {
        return Err(StatusCode::NOT_FOUND);
    }

    let bundle = {
        let pmgr = &asm.policy_mgr;
        let container = pmgr.get_current_container();
        match PolicyBundle::new_from_policy_container(0, container) {
            Ok(pb) => pb,
            Err(e) => {
                error!(target: ADMIN, "error creating policy bundle for policy {id}: {}", e);
                return Err(StatusCode::INTERNAL_SERVER_ERROR);
            }
        }
    };
    Ok(Json(bundle))
}

async fn install_policy(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EJson(body): EJson<PolicyBundle>,
) -> Result<Json<ListEntry>, StatusCode> {
    if !perm.can_write() {
        return Err(StatusCode::FORBIDDEN);
    }
    debug!(target: ADMIN, "POST /admin/policies");

    let rstate = state.read().await;
    if rstate.asm.config.guarded_policy.is_some() {
        return Err(StatusCode::FORBIDDEN);
    }
    let container_bytes = body.decode().map_err(|e| {
        error!(target: ADMIN, "error decoding policy bundle: {}", e);
        StatusCode::BAD_REQUEST
    })?;

    match rstate
        .asm
        .policy_mgr
        .update_policy_from_container_bytes(container_bytes)
        .await
    {
        Ok(vinst) => {
            info!(target: ADMIN, "policy updated successfully, new vinst={vinst}");
            // fire event!
            let evt = VsEvent::PolicyUpdated(vinst);
            if let Err(e) = rstate.asm.event_mgr.record_event(evt).await {
                warn!(target: ADMIN, "failed to record policy updated event: {}", e);
            }
        }
        Err(e) => {
            error!(target: ADMIN, "failed to update policy: {}", e);
            return Err(StatusCode::INTERNAL_SERVER_ERROR);
        }
    };
    drop(rstate);

    // what do we use as the policy identifier?
    // In the future maybe it will be possible to add policies to system without activating them.
    // In delegating model we mave have several policies installed for different domains.
    // For now we punt on this and just use identifier 0.
    let le = ListEntry {
        id: DEFAULT_POLICY_ID,
    };
    Ok(Json(le))
}

/// Returns a list of visa IDs in ListEntry structs or empty list.
async fn get_visas(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> (StatusCode, Json<Vec<ListEntry>>) {
    if !perm.can_read() {
        return (StatusCode::FORBIDDEN, Json(Vec::<ListEntry>::new()));
    }
    debug!(target: ADMIN, "GET /admin/visas");
    let rstate = state.read().await;

    // TODO: The API does not include details on how to do pagination.
    match rstate.asm.visa_mgr.list_all_visa_ids().await {
        Err(e) => {
            error!(target: ADMIN, "error listing visa IDs: {}", e);
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(Vec::<ListEntry>::new()),
            );
        }
        Ok(visa_ids) => {
            let le_list: Vec<ListEntry> = visa_ids.into_iter().map(|id| ListEntry { id }).collect();
            return (StatusCode::OK, Json(le_list));
        }
    }
}

/// Returns the recent-denies window, newest request first, optionally filtered
/// by `since` (epoch milliseconds, inclusive) and capped by `limit`.
async fn get_denies(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    Query(q): Query<DenyFilter>,
) -> (StatusCode, Json<Vec<DenyRecord>>) {
    if !perm.can_read() {
        return (StatusCode::FORBIDDEN, Json(Vec::<DenyRecord>::new()));
    }
    debug!(target: ADMIN, "GET /admin/visas/denies since={:?} limit={:?}", q.since, q.limit);
    let rstate = state.read().await;

    let records = rstate
        .asm
        .deny_log
        .recent(q.since, q.limit)
        .into_iter()
        .map(|e| DenyRecord {
            source_addr: e.source_addr,
            dest_addr: e.dest_addr,
            protocol: e.protocol,
            dest_port: e.dest_port,
            count: e.count,
            last_deny_ms: e.last_deny_ms,
            deny_code: e.deny_code,
        })
        .collect();

    (StatusCode::OK, Json(records))
}

async fn get_visa(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(id): EPath<u64>,
) -> Result<Json<VisaDescriptor>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    debug!(target: ADMIN, "GET /admin/visas/{}", id);
    let rstate = state.read().await;

    match rstate.asm.visa_mgr.get_visa_with_metadata_by_id(id).await {
        Err(e) => {
            error!(target: ADMIN, "error getting visa {}: {}", id, e);
            return Err(StatusCode::INTERNAL_SERVER_ERROR);
        }
        Ok(opt_visa_and_md) => match opt_visa_and_md {
            None => Err(StatusCode::NOT_FOUND),
            Some(visa_and_md) => {
                let visa = &visa_and_md.visa;
                let metadata = &visa_and_md.metadata;
                let proto_deets = protocol_details_for_visa(visa);

                let vd = VisaDescriptor {
                    id: visa.issuer_id,
                    expires: visa.expires,
                    created: metadata.ctime,
                    requesting_node: metadata.requesting_node.to_string(),
                    path: metadata
                        .path
                        .as_ref()
                        .map(|path| path.iter().map(ToString::to_string).collect()),
                    policy_id: metadata.policy_version.to_string(),
                    zpl: metadata.zpl.to_string(),
                    direction: match metadata.direction {
                        libeval::eval_result::Direction::Forward => {
                            admin_api_types::VisaMatchDirection::Forward
                        }
                        libeval::eval_result::Direction::Reverse => {
                            admin_api_types::VisaMatchDirection::Reverse
                        }
                    },
                    source_addr: if let Some(ref dock_pep) = visa.dock_pep {
                        Some(dock_pep.source_addr.to_string())
                    } else {
                        None
                    },
                    dest_addr: if let Some(ref dock_pep) = visa.dock_pep {
                        Some(dock_pep.dest_addr.to_string())
                    } else {
                        None
                    },
                    source_port: proto_deets.source_port,
                    dest_port: proto_deets.dest_port,
                    proto: proto_deets.protocol,
                    signals: metadata.signal_msgs.clone(),
                    session_key: if let Some(ref dock_pep) = visa.dock_pep {
                        to_api_keyset(&dock_pep.session_key)
                    } else {
                        ApiKeySet::default()
                    },
                };
                return Ok(Json(vd));
            }
        },
    }
}

fn protocol_details_for_visa(visa: &Visa) -> ProtocolDetails {
    if visa.dock_pep.is_none() {
        return ProtocolDetails {
            protocol: "N/A".to_string(),
            source_port: None,
            dest_port: None,
        };
    }
    let (proto_name, source_port, dest_port) = match &visa.dock_pep.as_ref().unwrap().pep {
        DockPepType::ICMP(icmp_pep) => (
            "ICMP".to_string(),
            Some(icmp_pep.icmp_type as u16),
            Some(icmp_pep.icmp_code as u16),
        ),
        DockPepType::UDP(tu_pep) => (
            "UDP".to_string(),
            Some(tu_pep.source_port),
            Some(tu_pep.dest_port),
        ),
        DockPepType::TCP(tu_pep) => (
            "TCP".to_string(),
            Some(tu_pep.source_port),
            Some(tu_pep.dest_port),
        ),
    };

    ProtocolDetails {
        protocol: proto_name,
        source_port,
        dest_port,
    }
}

async fn revoke_visa(EPath(id): EPath<u64>) -> impl IntoResponse {
    debug!(target: ADMIN, "DELETE /admin/visas/{}", id);
    let r = Revokes {
        id: id.to_string(),
        revoked: vec![0],
    };

    (StatusCode::OK, Json(r)).into_response()
}

/// Returns a list of connected CN values in CnEntry structs or empty list.
async fn get_actors(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    Query(q): Query<RoleFilter>,
) -> (StatusCode, Json<Vec<CnEntry>>) {
    if !perm.can_read() {
        return (StatusCode::FORBIDDEN, Json(Vec::<CnEntry>::new()));
    }
    debug!(target: ADMIN, "GET /admin/actors {:?}", q);
    let db_filter = match q.role {
        Some(ActorRole::Node) => Some(Role::Node),
        Some(ActorRole::Adapter) => Some(Role::Adapter),
        None => None,
    };

    let rstate = state.read().await;
    match rstate.asm.actor_mgr.list_actor_cns(db_filter).await {
        Err(e) => {
            error!(target: ADMIN, "error listing connected actors: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(Vec::<CnEntry>::new()),
            )
        }
        Ok(cns) => {
            let cn_list: Vec<CnEntry> = cns.into_iter().map(|cn| CnEntry { cn }).collect();
            (StatusCode::OK, Json(cn_list))
        }
    }
}

async fn get_actor(
    State(state): State<SharedState>,
    Extension(perm): Extension<Permission>,
    EPath(cn): EPath<String>,
) -> Result<Json<ActorDescriptor>, StatusCode> {
    debug!(target: ADMIN, "GET /admin/actor/{}", cn);
    let rstate = state.read().await;
    let asm = rstate.asm.clone();
    drop(rstate);

    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }

    match asm.actor_mgr.get_actor_by_cn(&cn).await {
        Err(_) => return Err(StatusCode::INTERNAL_SERVER_ERROR),
        Ok(opt_a) => match opt_a {
            None => Err(StatusCode::NOT_FOUND),
            Some(actor) => {
                let ident = match actor.get_identity() {
                    Some(id) => id.join("|"),
                    None => "".to_string(),
                };

                let is_node = match actor.get_attribute(key::ROLE) {
                    Some(role_attr) => {
                        if let Ok(role_val) = role_attr.get_single_value() {
                            role_val == ROLE_NODE
                        } else {
                            false
                        }
                    }
                    _ => false,
                };

                let zpr_addr_str = match actor.get_zpr_addr() {
                    Some(addr) => addr.to_string(),
                    None => "".to_string(),
                };

                let attrs = actor.attrs_iter().map(|a| to_api_attribute(a)).collect();

                let auth_exp = actor.get_authentication_expiration();

                let node_details = match is_node {
                    true => Some(build_node_record_brief(&asm, actor).await?),
                    false => None,
                };

                let descriptor = ActorDescriptor {
                    cn: cn.clone(),
                    ctime: SystemTime::UNIX_EPOCH, // TODO: Not tracked yet
                    ident,
                    node: is_node,
                    zpr_addr: zpr_addr_str,
                    attrs,
                    auth_exp,
                    node_details,
                };
                return Ok(Json(descriptor));
            }
        },
    }
}

pub(crate) async fn build_node_record_brief(
    asm: &Assembly,
    actor: libeval::actor::Actor,
) -> Result<NodeRecordBrief, StatusCode> {
    let counters = asm.counters.clone();
    let actor_mgr = asm.actor_mgr.clone();
    let visa_mgr = &asm.visa_mgr;
    let topo_mgr = &asm.topo_mgr;

    let zpr_addr = match actor.get_zpr_addr() {
        Some(addr) => addr,
        None => return Err(StatusCode::INTERNAL_SERVER_ERROR),
    };
    let pending_install = visa_mgr
        .get_num_pending_install_visas(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let last_contact = actor_mgr
        .get_node_last_seen(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let visa_requests = counters
        .get_node_counter(zpr_addr, CounterType::VisaRequests)
        .unwrap_or(0);
    let approved_vreqs = counters
        .get_node_counter(zpr_addr, CounterType::VisaRequestsApproved)
        .unwrap_or(0);
    let denied_vreqs = counters
        .get_node_counter(zpr_addr, CounterType::VisaRequestsDenied)
        .unwrap_or(0);
    let last_vreq = counters.get_last_request_time(zpr_addr);
    let adapters = actor_mgr
        .get_adapter_cns_connected_to_node(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let mut links: Vec<String> = Vec::new();
    for peer_addr in topo_mgr.get_peers(zpr_addr) {
        match actor_mgr.get_cn_by_zpr_addr(&peer_addr).await {
            Ok(cn) => links.push(cn),
            Err(e) => {
                warn!(target: ADMIN, "error {} getting CN for peer with addr {}", e, peer_addr);
                links.push(format!("cn_missing:{}", peer_addr))
            }
        }
    }
    let visas = visa_mgr
        .get_installed_visa_ids_for_node(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let visas_enqueued = visa_mgr
        .get_pending_visa_ids_for_node(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let pending_revocation = visa_mgr
        .get_num_pending_revoked_visas(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?;
    let (vss_port, in_sync) = match actor_mgr
        .get_node_vss(zpr_addr)
        .await
        .map_err(|_| StatusCode::INTERNAL_SERVER_ERROR)?
    {
        Some(socket_addr) => (Some(socket_addr.port()), true),
        None => (None, false),
    };

    let geography = actor.get_cn().and_then(|cn| asm.config.nodes.get(cn));
    Ok(NodeRecordBrief {
        latitude: geography.and_then(|node| node.latitude),
        longitude: geography.and_then(|node| node.longitude),
        pending_install,
        last_contact,
        visa_requests,
        connect_requests: 0, // TODO blocked on tracking calls to authorize_connect
        in_sync,
        approved_vreqs,
        denied_vreqs,
        last_vreq,
        adapters,
        links,
        visas,
        visas_enqueued,
        pending_revocation,
        vss_port,
    })
}

async fn revoke_actor(EPath(cn): EPath<String>) -> impl IntoResponse {
    debug!(target: ADMIN, "DELETE /admin/actors/{}", cn);
    let r = Revokes {
        id: "i".to_string(),
        revoked: vec![0, 1, 2],
    };

    (StatusCode::OK, Json(r)).into_response()
}

/// Resolve a CN to its ZPR address and a cloned (Arc-backed) visa manager, releasing the state
/// read-lock before returning so callers can do longer work without blocking writers. When
/// `require_node` is set, non-node actors are rejected with `BAD_REQUEST`. On any failure the
/// appropriate `StatusCode` is returned for the caller to surface.
async fn resolve_actor_addr(
    state: &SharedState,
    cn: &str,
    require_node: bool,
) -> Result<(IpAddr, VisaMgr), StatusCode> {
    let rstate = state.read().await;

    let actor = match rstate.asm.actor_mgr.get_actor_by_cn(cn).await {
        Err(e) => {
            error!(target: ADMIN, "error getting actor with cn {}: {}", cn, e);
            return Err(StatusCode::INTERNAL_SERVER_ERROR);
        }
        Ok(None) => return Err(StatusCode::NOT_FOUND),
        Ok(Some(actor)) => actor,
    };

    if require_node && !actor.is_node() {
        warn!(target: ADMIN, "actor {} is not a node", cn);
        return Err(StatusCode::BAD_REQUEST);
    }

    let actor_addr = match actor.get_zpr_addr() {
        None => {
            warn!(target: ADMIN, "actor {} has no ZPR address", cn);
            return Err(StatusCode::NOT_FOUND);
        }
        Some(addr) => *addr,
    };

    Ok((actor_addr, rstate.asm.visa_mgr.clone()))
}

/// List of visa IDs
async fn get_related_visas(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(cn): EPath<String>,
) -> (StatusCode, Json<Vec<ListEntry>>) {
    if !perm.can_read() {
        return (StatusCode::FORBIDDEN, Json(Vec::<ListEntry>::new()));
    }
    debug!(target: ADMIN, "GET /admin/actors/{}/visas", cn);

    let (actor_addr, visa_mgr) = match resolve_actor_addr(&state, &cn, false).await {
        Ok(pair) => pair,
        Err(code) => return (code, Json(Vec::<ListEntry>::new())),
    };

    match visa_mgr.get_visa_ids_for_actors(&[actor_addr]).await {
        Ok(ids) => (
            StatusCode::OK,
            Json(ids.into_iter().map(|id| ListEntry { id }).collect()),
        ),
        Err(e) => {
            error!(target: ADMIN, "get_visa_ids_for_actors failed: {e}");
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(Vec::<ListEntry>::new()),
            )
        }
    }
}

/// List of visa IDs
async fn get_visas_on_node(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(cn): EPath<String>,
) -> (StatusCode, Json<Vec<ListEntry>>) {
    if !perm.can_read() {
        return (StatusCode::FORBIDDEN, Json(Vec::<ListEntry>::new()));
    }
    debug!(target: ADMIN, "GET /admin/nodes/{}/visas", cn);

    let (actor_addr, visa_mgr) = match resolve_actor_addr(&state, &cn, true).await {
        Ok(pair) => pair,
        Err(code) => return (code, Json(Vec::<ListEntry>::new())),
    };

    let visa_ids = match visa_mgr.get_installed_visa_ids_for_node(&actor_addr).await {
        Ok(ids) => ids,
        Err(e) => {
            error!(target: ADMIN, "get_installed_visa_ids_for_node failed: {e}");
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(Vec::<ListEntry>::new()),
            );
        }
    };
    let related_visas: Vec<ListEntry> = visa_ids.into_iter().map(|id| ListEntry { id }).collect();
    (StatusCode::OK, Json(related_visas))
}

async fn get_services(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> (StatusCode, Json<Vec<NamedListEntry>>) {
    if !perm.can_read() {
        return (StatusCode::FORBIDDEN, Json(Vec::<NamedListEntry>::new()));
    }
    debug!(target: ADMIN, "GET /admin/services");
    let rstate = state.read().await;

    match rstate.asm.actor_mgr.get_services_list().await {
        Err(e) => {
            error!(target: ADMIN, "error listing services: {}", e);
            (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(Vec::<NamedListEntry>::new()),
            )
        }
        Ok(services) => {
            let sle_list: Vec<NamedListEntry> = services
                .into_iter()
                .map(|se| NamedListEntry { id: se.name })
                .collect();
            (StatusCode::OK, Json(sle_list))
        }
    }
}

async fn get_trusted_services(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> Result<Json<Vec<crate::trusted_services::TrustedServiceStatus>>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    let asm = state.read().await.asm.clone();
    Ok(Json(asm.ts_mgr.statuses()))
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct TrustedSourceToggle {
    enabled: bool,
}

#[derive(Serialize)]
struct TrustedSourceControlStatus {
    #[serde(flatten)]
    control: crate::trusted_services::control::Control,
    declared: bool,
    effective_enabled: bool,
}

async fn get_trusted_source_control(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(source): EPath<String>,
) -> Result<Json<TrustedSourceControlStatus>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    let asm = state.read().await.asm.clone();
    Ok(Json(trusted_source_control_status(&asm, &source).await?))
}

async fn trusted_source_control_status(
    asm: &Assembly,
    source: &str,
) -> Result<TrustedSourceControlStatus, StatusCode> {
    let control = crate::trusted_services::control::read(asm.state_db.as_ref(), source)
        .await
        .map_err(|error| {
            error!(target: ADMIN, "cannot read trusted source control for {}: {}", source, error);
            match error {
                ServiceError::Param(_) => StatusCode::BAD_REQUEST,
                _ => StatusCode::SERVICE_UNAVAILABLE,
            }
        })?;
    let declared = asm.ts_mgr.declares_source(source);
    if !declared && control.revision.is_empty() {
        return Err(StatusCode::NOT_FOUND);
    }
    Ok(TrustedSourceControlStatus {
        effective_enabled: declared && asm.ts_mgr.source_enabled(source),
        declared,
        control,
    })
}

async fn set_trusted_source_control(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(source): EPath<String>,
    Json(toggle): Json<TrustedSourceToggle>,
) -> Result<(StatusCode, Json<TrustedSourceControlStatus>), StatusCode> {
    if !perm.can_write() {
        return Err(StatusCode::FORBIDDEN);
    }
    let asm = state.read().await.asm.clone();
    trusted_source_control_status(&asm, &source).await?;
    let control = crate::trusted_services::control::set(
        asm.state_db.as_ref(), &asm.ts_mgr, &source, toggle.enabled,
    ).await.map_err(|error| {
        error!(target: ADMIN, "cannot durably apply trusted source control for {}: {}", source, error);
        StatusCode::SERVICE_UNAVAILABLE
    })?;
    asm.event_mgr.notify_trusted_service_change();
    let declared = asm.ts_mgr.declares_source(&source);
    Ok((
        StatusCode::ACCEPTED,
        Json(TrustedSourceControlStatus {
            effective_enabled: declared && asm.ts_mgr.source_enabled(&source),
            declared,
            control,
        }),
    ))
}

async fn get_service(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(svc_name): EPath<String>,
) -> Result<Json<ServiceDescriptor>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }

    debug!(target: ADMIN, "GET /admin/service name={}", svc_name);
    let rstate = state.read().await;

    match rstate.asm.actor_mgr.get_service_detail(&svc_name).await {
        Err(e) => {
            error!(target: ADMIN, "error getting service detail for service {}: {}", svc_name, e);
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
        Ok(opt_detail) => {
            if let Some(detail) = opt_detail {
                let connect_via = match detail.connect_via.as_ref() {
                    Some(cv_addr) => cv_addr.to_string(),
                    None => "".to_string(),
                };
                let cp = rstate.asm.policy_mgr.get_current();

                let (svc_kind, svc_endpoints) = if let Some(psvc) = cp.service_by_id(&svc_name) {
                    (
                        format!("{:?}", psvc.kind),
                        service_endpoints_to_string(&psvc.endpoints),
                    )
                } else {
                    ("".to_string(), "".to_string())
                };

                let sd = ServiceDescriptor {
                    service_name: detail.service_name,
                    zpr_addr: detail.zpr_addr.to_string(),
                    actor_cn: detail.actor_cn,
                    dock_zpr_addr: connect_via,
                    service_kind: svc_kind,
                    service_endpoints: svc_endpoints,
                };
                Ok(Json(sd))
            } else {
                Err(StatusCode::NOT_FOUND)
            }
        }
    }
}

/// DELETE /admin/services/{id}/cache — reload a trusted service's attribute data and
/// reconcile against it: the actors behind live visas are refreshed and their visas
/// re-checked, so no visa keeps relying on the old data.
///
/// The reload is synchronous (its failure is reported to the caller); the reconciliation
/// runs from a durable outbox on the event worker. `202 Accepted` acknowledges
/// persistence, not completion; GET on this resource reports per-source progress.
async fn flush_service_cache(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
    EPath(svc_id): EPath<String>,
) -> StatusCode {
    if !perm.can_write() {
        return StatusCode::FORBIDDEN;
    }
    debug!(target: ADMIN, "DELETE /admin/services/{}/cache", svc_id);

    let asm = state.read().await.asm.clone();
    match asm.ts_mgr.flush_one(&svc_id).await {
        Ok(()) => {
            if let Err(error) =
                crate::trusted_reconciliation::enqueue(asm.state_db.as_ref(), &svc_id).await
            {
                error!(target: ADMIN, "could not durably accept reconciliation for {}: {}", svc_id, error);
                return StatusCode::SERVICE_UNAVAILABLE;
            }
            // The queue is only a wake-up hint. The durable request is retried by
            // the event worker's timer, including after a process restart.
            asm.event_mgr.notify_trusted_service_change();
            StatusCode::ACCEPTED
        }
        Err(ServiceError::TrustedServiceNotFound(_)) => StatusCode::NOT_FOUND,
        Err(e) => {
            error!(target: ADMIN, "error flushing trusted service {}: {}", svc_id, e);
            StatusCode::INTERNAL_SERVER_ERROR
        }
    }
}

async fn trusted_reconciliation_status(
    State(state): State<SharedState>,
    EPath(svc_id): EPath<String>,
) -> Result<Json<crate::trusted_reconciliation::Status>, StatusCode> {
    let asm = state.read().await.asm.clone();
    crate::trusted_reconciliation::status(asm.state_db.as_ref(), &svc_id)
        .await
        .map(Json)
        .map_err(|_| StatusCode::SERVICE_UNAVAILABLE)
}

fn service_endpoints_to_string(endpoints: &[Scope]) -> String {
    let mut ep_strs: Vec<String> = Vec::new();
    for ep in endpoints {
        let port_str = if let Some(port) = ep.port {
            port.to_string()
        } else if let Some(port_range) = ep.port_range {
            format!("{}-{}", port_range.0, port_range.1)
        } else {
            "".to_string()
        };
        match ep.protocol {
            ip_proto::TCP => ep_strs.push(format!("TCP/{}", port_str)),
            ip_proto::UDP => ep_strs.push(format!("UDP/{}", port_str)),
            ip_proto::ICMP => ep_strs.push(format!("ICMP/{}", port_str)),
            ip_proto::IPV6_ICMP => ep_strs.push(format!("IPV6_ICMP/{}", port_str)),
            _ => ep_strs.push(format!("PROTO_{}/{}", ep.protocol, port_str)),
        }
    }
    ep_strs.join(",")
}

async fn get_revokes() -> impl IntoResponse {
    debug!(target: ADMIN, "GET /admin/authrevoke - NOT IMPLEMENTED");
    (StatusCode::OK, Json(Vec::<ListEntry>::new())).into_response()
}

async fn get_revoke(EPath(id): EPath<String>) -> impl IntoResponse {
    debug!(target: ADMIN, "GET /admin/authrevoke/{} - NOT IMPLEMENTED", id);
    (StatusCode::NOT_FOUND, Json(()).into_response())
}

async fn clear_revokes() -> impl IntoResponse {
    debug!(target: ADMIN, "POST /admin/authrevoke/clear - NOT IMPLEMENTED");
    (StatusCode::NOT_IMPLEMENTED, Json(()).into_response())
}

async fn remove_revoke(EPath(id): EPath<String>) -> impl IntoResponse {
    debug!(target: ADMIN, "DELETE /admin/authrevoke/{} - NOT IMPLEMENTED", id);
    (StatusCode::NOT_IMPLEMENTED, Json(()).into_response())
}

async fn add_revoke(EPath(id): EPath<String>) -> impl IntoResponse {
    debug!(target: ADMIN, "POST /admin/authrevoke/{} - NOT IMPLEMENTED", id);
    (StatusCode::NOT_IMPLEMENTED, Json(()).into_response())
}

/// Stringify a substrate address as a parseable `host:port`. IPv6 hosts get bracketed
/// (`[fd5a:5052::1]:5000`); IPv4 and hostname forms are left plain.
fn stringify_netaddr(addr: &NetAddr) -> String {
    match &addr.host {
        NetworkHost::Ip(ip) => SocketAddr::new(*ip, addr.port).to_string(),
        NetworkHost::Hostname(h) => format!("{}:{}", h, addr.port),
    }
}

async fn get_network(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> Result<Json<NetworkDetails>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }
    debug!(target: ADMIN, "GET /admin/network");
    let rstate = state.read().await;

    // Start with the desired topology setup, then we will mark which ones are up.

    // Only track link(a,b) once (ie, do not also store link(b,a)).
    let mut pairs = HashSet::new();

    let mut topo = Vec::new();
    let psnap = rstate.asm.policy_mgr.get_current_snapshot();

    for node_addr in psnap.policy().all_peered_nodes() {
        if let Some(peers) = psnap.policy().get_peers_for_node(node_addr) {
            for peer in peers {
                let pair_id = if node_addr < &peer.remote_zpr_addr {
                    (node_addr.clone(), peer.remote_zpr_addr.clone())
                } else {
                    (peer.remote_zpr_addr.clone(), node_addr.clone())
                };

                if pairs.contains(&pair_id) {
                    continue;
                }
                pairs.insert(pair_id);

                // Each peer has a link_id, remote_zpr_addr, and a remote_substrate NetAddr.
                // Ok so a desired link is node_addr to peer

                let mut bldr =
                    NodeConnection::builder(node_addr.clone(), peer.remote_zpr_addr.clone())
                        .link_id(peer.link_id.clone())
                        .node_a_substrate(stringify_netaddr(&peer.local_substrate))
                        .node_b_substrate(stringify_netaddr(&peer.remote_substrate));

                if let Ok(link_desc) = psnap
                    .policy()
                    .describe_link(node_addr, &peer.remote_zpr_addr)
                {
                    let api_attrs = link_desc
                        .attrs
                        .iter()
                        .map(to_api_attribute)
                        .collect::<Vec<ApiAttribute>>();

                    bldr = bldr.link_attrs(api_attrs).link_cost(link_desc.cost);
                }
                topo.push(bldr.build());
            }
        }
    }

    // Now we have constructed the ideal topology, lets see what we've actually got
    // starting with our connected nodes.

    let node_addrs = match rstate.asm.actor_mgr.list_node_addrs().await {
        Ok(addrs) => addrs,
        Err(e) => {
            error!(target: ADMIN, "Error {} getting node list", e);
            return Err(StatusCode::INTERNAL_SERVER_ERROR);
        }
    };

    // Everything in topo so far is policy-declared, so only those entries can go UP.
    // Links appended past this point were reported by a node but never declared.
    let declared = topo.len();

    for node_addr in node_addrs {
        for peer_addr in rstate.asm.topo_mgr.get_peers(&node_addr) {
            // Ok we have node_addr <-> peer_addr, so update the node connection to UP
            // Or if it does not exist, insert it as INVALID.
            let mut matched = false;
            for (i, nc) in topo.iter_mut().enumerate() {
                if nc.is_link_between(&node_addr, &peer_addr) {
                    if i < declared {
                        nc.set_status(ConnectionType::UP);
                    }
                    matched = true;
                }
            }
            if !matched {
                topo.push(
                    NodeConnection::builder(node_addr.clone(), peer_addr.clone())
                        .invalid()
                        .build(),
                );
            }
        }
    }

    Ok(Json(NetworkDetails { network: topo }))
}

async fn get_stats(
    Extension(perm): Extension<Permission>,
    State(state): State<SharedState>,
) -> Result<Json<Stats>, StatusCode> {
    if !perm.can_read() {
        return Err(StatusCode::FORBIDDEN);
    }

    debug!(target: ADMIN, "GET /admin/stats");

    let mut stat_map = HashMap::new();

    let rstate = state.read().await;

    stat_map.insert(
        "uptime".to_string(),
        rstate.asm.get_uptime().as_secs().to_string(),
    );
    for (key, ref value) in &rstate.asm.counters.counters {
        stat_map.insert(key.name().to_string(), value.get_count().to_string());
    }

    Ok(Json(Stats { stats: stat_map }))
}

fn to_api_attribute(attr: &Attribute) -> ApiAttribute {
    ApiAttribute {
        key: attr.get_key().to_string(),
        value: attr.get_value().to_vec(),
        expires_at: attr.get_expires(),
    }
}

fn to_api_keyset(ks: &KeySet) -> ApiKeySet {
    ApiKeySet {
        format: to_api_keyformat(&ks.format),
        ingress_key: ks.ingress_key.clone(),
        egress_key: ks.egress_key.clone(),
    }
}

fn to_api_keyformat(kf: &KeyFormat) -> ApiKeyFormat {
    match kf {
        KeyFormat::ZprKF01 => ApiKeyFormat::ZprKF01,
    }
}

#[cfg(test)]
mod tests {

    use super::*;

    include!("policy_interop_fixture.rs");

    /// Visa expiration used by the tests in this module.
    const DEFAULT_VISA_EXPIRATION: std::time::Duration =
        std::time::Duration::from_secs(4 * 60 * 60);

    use crate::apikey::ApiKey;
    use admin_api_types::VisaMatchDirection;
    use axum::body::Body;
    use http_body_util::BodyExt;
    use libeval::eval_result::{Direction, Hit, Signal};
    use libeval::route::Route;
    use std::net::IpAddr;
    use tower::ServiceExt;
    use zpr::vsapi_types::{DenyCode, PacketDesc};

    use crate::admin_apikeys::{ApiKeyRecord, KeyStatus};
    use crate::assembly::tests::{new_assembly_for_tests, new_assembly_with_event_rx};
    use crate::test_helpers::{
        make_adapter_actor_defexp, make_node_actor_defexp, make_peering, policy_with_peerings,
    };
    use zpr::policy_types::AttrExp;

    /// Insert a readwrite test key into the assembly's key store and return the
    /// key string to use in the X-API-Key header.
    fn setup_test_api_rw_key(asm: &Arc<Assembly>) -> String {
        setup_test_api_key_with_perm(asm, Permission::ReadWrite)
    }

    fn setup_test_api_r_key(asm: &Arc<Assembly>) -> String {
        setup_test_api_key_with_perm(asm, Permission::Read)
    }

    /// Insert a test key with the given permission into the assembly's key store
    /// and return the key string to use in the X-API-Key header.
    fn setup_test_api_key_with_perm(asm: &Arc<Assembly>, permission: Permission) -> String {
        let secret_bytes: [u8; 32] = (0u8..32).collect::<Vec<_>>().try_into().unwrap();
        let apikey = ApiKey::new(0xaabbccdd, secret_bytes);
        let record = ApiKeyRecord {
            owner: "test".to_string(),
            permission,
            status: KeyStatus::Active,
            created: "2026-01-01".to_string(),
            secret_hash: apikey.secret_hash().unwrap(),
            description: "test key".to_string(),
        };
        asm.admin_api_keys
            .insert_for_test(apikey.key_id_hex(), record);
        apikey.to_key_string()
    }

    /// Build a valid `PolicyBundle` carrying a minimal policy (no topology, so the
    /// FakeResolver in the test assembly resolves it cleanly) at the minimum
    /// supported compiler version, for use as `install_policy` request input.
    fn make_valid_policy_bundle() -> PolicyBundle {
        let mut msg = capnp::message::Builder::new_default();
        {
            let mut policy = msg.init_root::<zpr::policy::v1::policy::Builder>();
            policy.set_created("2026-01-01T00:00:00Z");
            policy.set_version(1);
            policy.set_metadata("");
        }
        let mut inner = Vec::new();
        capnp::serialize::write_message(&mut inner, &msg).unwrap();
        let container = crate::test_helpers::make_container_bytes(
            crate::config::POLICY_MIN_COMPILER_MAJOR,
            crate::config::POLICY_MIN_COMPILER_MINOR,
            crate::config::POLICY_MIN_COMPILER_PATCH,
            &inner,
        );
        PolicyBundle::new_from_policy_container(0, container.into()).unwrap()
    }

    fn policy_operation_trust_fixture(
        now: u64,
    ) -> (
        crate::config::GuardedPolicyConfig,
        openssl::pkey::PKey<openssl::pkey::Private>,
    ) {
        use base64::{Engine, engine::general_purpose::STANDARD};
        let key = openssl::pkey::PKey::generate_ed25519().unwrap();
        let config = crate::config::GuardedPolicyConfig {
            audience: "https://vs.example.test".into(),
            policy_service_audience: "https://policy.example.test".into(),
            target_id: "vs-fixture".into(),
            organization: "org-fixture".into(),
            keys: vec![crate::config::PolicyApprovalKey {
                key_id: "cr-fixture-key".into(),
                issuer: "https://control-room.example.test".into(),
                public_key: STANDARD.encode(key.raw_public_key().unwrap()),
                expires_unix: now + 3600,
            }],
            grants: vec![crate::config::PolicyApprovalGrant {
                issuer: "https://idp.example.test".into(),
                subject: "named-operator".into(),
                organization: "org-fixture".into(),
                actions: vec!["install".into(), "rollback".into()],
            }],
        };
        (config, key)
    }

    fn sign_policy_operation_fixture(
        claims: &PolicyApprovalClaims,
        key: &openssl::pkey::PKey<openssl::pkey::Private>,
    ) -> PolicyApprovalProof {
        use base64::{Engine, engine::general_purpose::STANDARD};
        let payload = serde_json::to_vec(claims).unwrap();
        let signature = openssl::sign::Signer::new_without_digest(key)
            .unwrap()
            .sign_oneshot_to_vec(&payload)
            .unwrap();
        PolicyApprovalProof {
            payload: STANDARD.encode(payload),
            signature: STANDARD.encode(signature),
        }
    }

    fn policy_operation_claims_fixture(
        config: &crate::config::GuardedPolicyConfig,
        operation: PolicyOperationRequest,
        now: u64,
    ) -> PolicyApprovalClaims {
        PolicyApprovalClaims {
            version: 1,
            key_id: config.keys[0].key_id.clone(),
            issuer: config.keys[0].issuer.clone(),
            audience: config.audience.clone(),
            purpose: POLICY_APPROVAL_PURPOSE.into(),
            approver_issuer: config.grants[0].issuer.clone(),
            approver_subject: config.grants[0].subject.clone(),
            organization: config.organization.clone(),
            action: "install".into(),
            method: "POST".into(),
            route: POLICY_OPERATION_ROUTE.into(),
            operation,
            iat: now,
            exp: now + 30,
            nonce: "fixture-nonce-0000000000000001".into(),
        }
    }

    async fn get_policy_operation_current_fixture(app: &Router, api_key: Option<&str>) -> Response {
        let mut request = Request::builder().uri(POLICY_OPERATION_CURRENT_ROUTE);
        if let Some(key) = api_key {
            request = request.header("X-API-Key", key);
        }
        app.clone()
            .oneshot(request.body(Body::empty()).unwrap())
            .await
            .unwrap()
    }

    #[tokio::test]
    async fn test_policy_operation_current_reads_authoritative_cas_state_with_read_auth() {
        let now = SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs();
        let (trust, _) = policy_operation_trust_fixture(now);
        let mut asm = new_assembly_for_tests(None).await;
        asm.config.guarded_policy = Some(trust.clone());
        let asm = Arc::new(asm);
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
            asm.clone(),
        ))));
        for key in [None, Some("invalid-key")] {
            assert_eq!(
                get_policy_operation_current_fixture(&app, key)
                    .await
                    .status(),
                StatusCode::UNAUTHORIZED
            );
        }

        // Advance only persisted state: the endpoint must not use PolicyMgr's cache.
        let candidate = make_valid_policy_bundle().decode().unwrap();
        let digest = hex::encode(openssl::sha::sha256(&candidate));
        let vinst = (1u64 << 53) + 1;
        asm.state_db
            .atomic_pipeline(&[
                crate::db::DbOp::SetBin {
                    key: format!("policies:{digest}:container"),
                    value: candidate,
                },
                crate::db::DbOp::HSet {
                    hash_key: "policy:current".into(),
                    field: "phash".into(),
                    value: digest.clone(),
                },
                crate::db::DbOp::HSet {
                    hash_key: "policy:current".into(),
                    field: "vinst".into(),
                    value: vinst.to_string(),
                },
            ])
            .await
            .unwrap();
        assert_ne!(asm.policy_mgr.get_current_snapshot().vinst(), vinst);
        for permission in [Permission::Read, Permission::ReadWrite] {
            let key = setup_test_api_key_with_perm(&asm, permission);
            let response = get_policy_operation_current_fixture(&app, Some(&key)).await;
            assert_eq!(response.status(), StatusCode::OK);
            let body = response.into_body().collect().await.unwrap().to_bytes();
            let value: serde_json::Value = serde_json::from_slice(&body).unwrap();
            assert_eq!(
                value,
                serde_json::json!({
                    "target_id": trust.target_id,
                    "organization": trust.organization,
                    "current_digest": digest,
                    "current_vinst": vinst,
                })
            );
        }
    }

    #[tokio::test]
    async fn test_policy_operation_current_fails_closed_without_valid_guarded_config() {
        let now = SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs();
        let (trust, _) = policy_operation_trust_fixture(now);
        let mutations: &[fn(&mut crate::config::GuardedPolicyConfig)] = &[
            |c| c.target_id.clear(),
            |c| c.organization.clear(),
            |c| c.audience = c.policy_service_audience.clone(),
            |c| c.audience = "http://vs.example.test".into(),
            |c| c.keys.clear(),
            |c| c.keys[0].public_key = "invalid".into(),
            |c| c.keys[0].expires_unix = 1,
            |c| c.grants.clear(),
            |c| c.grants[0].organization = "wrong-org".into(),
            |c| c.grants[0].actions = vec!["*".into()],
        ];
        let mut configs = vec![None];
        for mutate in mutations {
            let mut invalid = trust.clone();
            mutate(&mut invalid);
            configs.push(Some(invalid));
        }
        for config in configs {
            let mut asm = new_assembly_for_tests(None).await;
            asm.config.guarded_policy = config;
            let asm = Arc::new(asm);
            let key = setup_test_api_r_key(&asm);
            let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(asm))));
            assert_eq!(
                get_policy_operation_current_fixture(&app, Some(&key))
                    .await
                    .status(),
                StatusCode::SERVICE_UNAVAILABLE
            );
        }
    }

    #[tokio::test]
    async fn test_policy_operation_current_rejects_missing_corrupt_or_unverified_state() {
        let now = SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs();
        let (trust, _) = policy_operation_trust_fixture(now);
        let mut asm = new_assembly_for_tests(None).await;
        asm.config.guarded_policy = Some(trust);
        let asm = Arc::new(asm);
        let key = setup_test_api_r_key(&asm);
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
            asm.clone(),
        ))));
        let cases = [
            None,
            Some(("a".repeat(64), None)),
            Some(("a".repeat(64), Some("not-a-version"))),
            Some(("invalid-digest".into(), Some("1"))),
            Some(("a".repeat(64), Some("1"))),
        ];
        for fields in cases {
            asm.state_db.del("policy:current").await.unwrap();
            if let Some((digest, vinst)) = fields {
                asm.state_db
                    .hset("policy:current", "phash", &digest)
                    .await
                    .unwrap();
                if let Some(vinst) = vinst {
                    asm.state_db
                        .hset("policy:current", "vinst", vinst)
                        .await
                        .unwrap();
                }
            }
            assert_eq!(
                get_policy_operation_current_fixture(&app, Some(&key))
                    .await
                    .status(),
                StatusCode::SERVICE_UNAVAILABLE
            );
        }
        asm.state_db
            .set_bin(
                &format!("policies:{}:container", "a".repeat(64)),
                b"wrong bytes",
            )
            .await
            .unwrap();
        assert_eq!(
            get_policy_operation_current_fixture(&app, Some(&key))
                .await
                .status(),
            StatusCode::SERVICE_UNAVAILABLE
        );
    }

    #[test]
    fn test_policy_operation_proof_rejects_wrong_scope_stale_and_untrusted_assertions() {
        let now = 1_800_000_000;
        let (config, key) = policy_operation_trust_fixture(now);
        let candidate = make_valid_policy_bundle().decode().unwrap();
        let operation = PolicyOperationRequest {
            operation_id: "proof-fixture".into(),
            target_id: config.target_id.clone(),
            source_revision: 12,
            candidate_digest: hex::encode(openssl::sha::sha256(&candidate)),
            expected_current_digest: "a".repeat(64),
            expected_current_vinst: 7,
        };
        let claims = policy_operation_claims_fixture(&config, operation.clone(), now);
        let proof = sign_policy_operation_fixture(&claims, &key);
        assert!(verify_policy_approval(&config, &proof, &operation, &candidate, now).is_ok());

        let mutations: &[fn(&mut PolicyApprovalClaims)] = &[
            |c| c.version = 2,
            |c| c.key_id = "untrusted".into(),
            |c| c.issuer = "https://untrusted.test".into(),
            |c| c.audience = "https://policy.example.test".into(),
            |c| c.purpose = "runtime-key-import".into(),
            |c| c.approver_issuer = "https://untrusted-idp.test".into(),
            |c| c.approver_subject = "caller-asserted-user".into(),
            |c| c.organization = "another-org".into(),
            |c| c.action = "read".into(),
            |c| c.method = "GET".into(),
            |c| c.route = "/admin/policies".into(),
            |c| c.operation.operation_id = "another-operation".into(),
            |c| c.operation.target_id = "another-vs".into(),
            |c| c.operation.source_revision += 1,
            |c| c.operation.candidate_digest = "b".repeat(64),
            |c| c.operation.expected_current_digest = "c".repeat(64),
            |c| c.operation.expected_current_vinst += 1,
            |c| c.iat += 1,
            |c| c.exp = c.iat,
            |c| c.exp = c.iat + 301,
            |c| c.nonce.clear(),
        ];
        for (index, mutate) in mutations.iter().enumerate() {
            let mut wrong = claims.clone();
            mutate(&mut wrong);
            let proof = sign_policy_operation_fixture(&wrong, &key);
            assert!(
                verify_policy_approval(&config, &proof, &operation, &candidate, now).is_err(),
                "mutation {index} must fail closed"
            );
        }
        assert!(verify_policy_approval(&config, &proof, &operation, &candidate, now + 30).is_err());
        assert!(
            verify_policy_approval(&config, &proof, &operation, b"other candidate", now).is_err()
        );
        let other_key = openssl::pkey::PKey::generate_ed25519().unwrap();
        let untrusted = sign_policy_operation_fixture(&claims, &other_key);
        assert!(verify_policy_approval(&config, &untrusted, &operation, &candidate, now).is_err());
        let mut malformed = proof.clone();
        malformed.payload.push('=');
        assert!(verify_policy_approval(&config, &malformed, &operation, &candidate, now).is_err());
        let mut expired_trust = config.clone();
        expired_trust.keys[0].expires_unix = now;
        assert!(
            verify_policy_approval(&expired_trust, &proof, &operation, &candidate, now).is_err()
        );
        let mut rollback = claims.clone();
        rollback.action = "rollback".into();
        let mut install_only = config.clone();
        install_only.grants[0].actions = vec!["install".into()];
        assert!(
            verify_policy_approval(
                &install_only,
                &sign_policy_operation_fixture(&rollback, &key),
                &operation,
                &candidate,
                now,
            )
            .is_err()
        );
        let mut another_target = operation.clone();
        another_target.target_id = "another-vs".into();
        let mut wrong_target = claims.clone();
        wrong_target.operation = another_target.clone();
        assert!(
            verify_policy_approval(
                &config,
                &sign_policy_operation_fixture(&wrong_target, &key),
                &another_target,
                &candidate,
                now,
            )
            .is_err()
        );
        let mut shared_audience = config.clone();
        shared_audience.audience = shared_audience.policy_service_audience.clone();
        assert_eq!(
            verify_policy_approval(&shared_audience, &proof, &operation, &candidate, now).err(),
            Some(StatusCode::SERVICE_UNAVAILABLE)
        );
    }

    async fn post_policy_operation_fixture(
        app: &Router,
        path: &str,
        api_key: Option<&str>,
        command: &GuardedPolicyCommand,
    ) -> Response {
        let mut request = Request::builder()
            .method("POST")
            .uri(path)
            .header("content-type", "application/json");
        if let Some(key) = api_key {
            request = request.header("X-API-Key", key);
        }
        app.clone()
            .oneshot(
                request
                    .body(Body::from(serde_json::to_vec(command).unwrap()))
                    .unwrap(),
            )
            .await
            .unwrap()
    }

    #[tokio::test]
    async fn test_policy_operation_route_dual_auth_replay_and_durable_reconciliation() {
        let now = SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs();
        let (trust, key) = policy_operation_trust_fixture(now);
        let mut asm = new_assembly_for_tests(None).await;
        asm.config.guarded_policy = Some(trust.clone());
        let asm = Arc::new(asm);
        let current = asm.policy_mgr.get_current_snapshot();
        let bundle = make_valid_policy_bundle();
        let candidate = bundle.decode().unwrap();
        let operation = PolicyOperationRequest {
            operation_id: "guarded-route-fixture".into(),
            target_id: trust.target_id.clone(),
            source_revision: 12,
            candidate_digest: hex::encode(openssl::sha::sha256(&candidate)),
            expected_current_digest: hex::encode(openssl::sha::sha256(
                current.container().as_bytes(),
            )),
            expected_current_vinst: current.vinst(),
        };
        let mut claims = policy_operation_claims_fixture(&trust, operation.clone(), now);
        let mut command = GuardedPolicyCommand {
            operation,
            bundle,
            proof: sign_policy_operation_fixture(&claims, &key),
        };
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
            asm.clone(),
        ))));
        assert_eq!(
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, None, &command)
                .await
                .status(),
            StatusCode::UNAUTHORIZED
        );
        let read_key = setup_test_api_r_key(&asm);
        assert_eq!(
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&read_key), &command)
                .await
                .status(),
            StatusCode::FORBIDDEN
        );
        let write_key = setup_test_api_rw_key(&asm);
        assert_eq!(
            post_policy_operation_fixture(
                &app,
                "/admin/policy-operations?bypass=1",
                Some(&write_key),
                &command
            )
            .await
            .status(),
            StatusCode::BAD_REQUEST
        );
        assert_eq!(asm.policy_mgr.get_current_snapshot().vinst(), 1);

        let valid_proof = command.proof.clone();
        command.proof = sign_policy_operation_fixture(
            &claims,
            &openssl::pkey::PKey::generate_ed25519().unwrap(),
        );
        assert_eq!(
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&write_key), &command)
                .await
                .status(),
            StatusCode::FORBIDDEN
        );
        command.proof = valid_proof;
        let response =
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&write_key), &command)
                .await;
        assert_eq!(response.status(), StatusCode::OK);
        let receipt: PolicyOperationReceipt =
            serde_json::from_slice(&response.into_body().collect().await.unwrap().to_bytes())
                .unwrap();
        assert_eq!(receipt.status, crate::db::PolicyOperationStatus::Succeeded);
        assert_eq!(
            receipt.applied_digest,
            Some(claims.operation.candidate_digest.clone())
        );
        assert_eq!(
            asm.policy_mgr.get_current_snapshot().container().as_bytes(),
            candidate
        );
        let approval = asm
            .state_db
            .get("policy:approvals:guarded-route-fixture")
            .await
            .unwrap()
            .unwrap();
        assert!(approval.contains("named-operator"));
        assert!(approval.contains(&claims.operation.candidate_digest));

        assert_eq!(
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&write_key), &command)
                .await
                .status(),
            StatusCode::CONFLICT
        );
        claims.nonce = "fixture-nonce-0000000000000002".into();
        command.proof = sign_policy_operation_fixture(&claims, &key);
        assert_eq!(
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&write_key), &command)
                .await
                .status(),
            StatusCode::OK
        );
        assert_eq!(asm.policy_mgr.get_current_snapshot().vinst(), 2);

        claims.action = "rollback".into();
        claims.nonce = "fixture-nonce-0000000000000003".into();
        command.proof = sign_policy_operation_fixture(&claims, &key);
        assert_eq!(
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&write_key), &command)
                .await
                .status(),
            StatusCode::CONFLICT
        );
        assert_eq!(asm.policy_mgr.get_current_snapshot().vinst(), 2);

        command.bundle = PolicyBundle::new_from_policy_container(0, current.container()).unwrap();
        command.operation.operation_id = "explicit-rollback-fixture".into();
        command.operation.candidate_digest =
            hex::encode(openssl::sha::sha256(current.container().as_bytes()));
        command.operation.expected_current_digest = claims.operation.candidate_digest.clone();
        command.operation.expected_current_vinst = 2;
        claims.operation = command.operation.clone();
        claims.nonce = "fixture-nonce-0000000000000004".into();
        command.proof = sign_policy_operation_fixture(&claims, &key);
        let response =
            post_policy_operation_fixture(&app, POLICY_OPERATION_ROUTE, Some(&write_key), &command)
                .await;
        assert_eq!(response.status(), StatusCode::OK);
        let receipt: PolicyOperationReceipt =
            serde_json::from_slice(&response.into_body().collect().await.unwrap().to_bytes())
                .unwrap();
        assert_eq!(receipt.status, crate::db::PolicyOperationStatus::Succeeded);
        assert_eq!(receipt.applied_vinst, Some(3));
        assert_eq!(
            asm.policy_mgr.get_current_snapshot().container().as_bytes(),
            current.container().as_bytes()
        );
    }

    #[tokio::test]
    async fn test_policy_operation_guarded_mode_blocks_legacy_even_with_invalid_trust() {
        let now = 1_800_000_000;
        let (mut trust, key) = policy_operation_trust_fixture(now);
        let bundle = make_valid_policy_bundle();
        let operation = PolicyOperationRequest {
            operation_id: "disabled-route-fixture".into(),
            target_id: trust.target_id.clone(),
            source_revision: 12,
            candidate_digest: hex::encode(openssl::sha::sha256(&bundle.decode().unwrap())),
            expected_current_digest: "a".repeat(64),
            expected_current_vinst: 1,
        };
        let claims = policy_operation_claims_fixture(&trust, operation.clone(), now);
        let command = GuardedPolicyCommand {
            operation,
            bundle,
            proof: sign_policy_operation_fixture(&claims, &key),
        };
        trust.keys.clear();
        for config in [None, Some(trust)] {
            let mut asm = new_assembly_for_tests(None).await;
            let guarded = config.is_some();
            asm.config.guarded_policy = config;
            let asm = Arc::new(asm);
            let api_key = setup_test_api_rw_key(&asm);
            let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
                asm.clone(),
            ))));
            assert_eq!(
                post_policy_operation_fixture(
                    &app,
                    POLICY_OPERATION_ROUTE,
                    Some(&api_key),
                    &command
                )
                .await
                .status(),
                StatusCode::SERVICE_UNAVAILABLE
            );
            let response = app
                .oneshot(
                    Request::builder()
                        .method("POST")
                        .uri("/admin/policies")
                        .header("X-API-Key", api_key)
                        .header("content-type", "application/json")
                        .body(Body::from(
                            serde_json::to_vec(&make_valid_policy_bundle()).unwrap(),
                        ))
                        .unwrap(),
                )
                .await
                .unwrap();
            assert_eq!(
                response.status(),
                if guarded {
                    StatusCode::FORBIDDEN
                } else {
                    StatusCode::OK
                }
            );
            assert_eq!(
                asm.policy_mgr.get_current_snapshot().vinst(),
                if guarded { 1 } else { 2 }
            );
        }
    }

    /// GET /admin/policies returns the single current policy id (0) with a read key.
    #[tokio::test]
    async fn test_get_policies_ok() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/policies")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let entry: ListEntry = serde_json::from_slice(&body).unwrap();
        assert_eq!(entry.id, DEFAULT_POLICY_ID);
    }

    #[tokio::test]
    async fn test_get_trusted_services_requires_read_key() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm)));
        let app = admin_app(shared_state);
        let unauthorized = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri("/admin/trusted-services")
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(unauthorized.status(), StatusCode::UNAUTHORIZED);

        let response = app
            .oneshot(
                Request::builder()
                    .uri("/admin/trusted-services")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        let statuses: Vec<serde_json::Value> = serde_json::from_slice(&body).unwrap();
        assert!(statuses.is_empty());
    }

    #[tokio::test]
    async fn trusted_source_control_auth_strict_json_and_durable_readback() {
        let (asm, _, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
            asm.clone(),
        ))));
        let uri = "/admin/trusted-services/hr/control";
        let response = app
            .clone()
            .oneshot(Request::builder().uri(uri).body(Body::empty()).unwrap())
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
        let read_key = setup_test_api_r_key(&asm);
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header("X-API-Key", &read_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        let default: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(default["enabled"], true);
        assert_eq!(default["effective_enabled"], true);
        assert_eq!(default["pending"], false);
        assert_eq!(default["revision"], "");
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method("PUT")
                    .uri(uri)
                    .header("X-API-Key", &read_key)
                    .header("Content-Type", "application/json")
                    .body(Body::from(r#"{"enabled":false}"#))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::FORBIDDEN);
        let write_key = setup_test_api_rw_key(&asm);
        for body in [
            r#"{}"#,
            r#"{"enabled":"false"}"#,
            r#"{"enabled":false,"other":1}"#,
            r#"{"enabled":true,"enabled":false}"#,
        ] {
            let response = app
                .clone()
                .oneshot(
                    Request::builder()
                        .method("PUT")
                        .uri(uri)
                        .header("X-API-Key", &write_key)
                        .header("Content-Type", "application/json")
                        .body(Body::from(body))
                        .unwrap(),
                )
                .await
                .unwrap();
            assert_eq!(
                response.status(),
                StatusCode::UNPROCESSABLE_ENTITY,
                "{body}"
            );
        }
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .method("PUT")
                    .uri(uri)
                    .header("X-API-Key", &write_key)
                    .header("Content-Type", "application/json")
                    .body(Body::from(r#"{"enabled":false}"#))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::ACCEPTED);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        let accepted: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(accepted["enabled"], false);
        assert_eq!(accepted["effective_enabled"], false);
        assert_eq!(accepted["pending"], true);
        assert_eq!(accepted["declared"], true);
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header("X-API-Key", &write_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        assert_eq!(
            serde_json::from_slice::<serde_json::Value>(&body).unwrap(),
            accepted
        );
        asm.ts_mgr.update_services(vec![]);
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header("X-API-Key", &write_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        let absent: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(absent["declared"], false);
        assert_eq!(absent["enabled"], false);
        let response = app
            .oneshot(
                Request::builder()
                    .method("PUT")
                    .uri("/admin/trusted-services/unknown/control")
                    .header("X-API-Key", &write_key)
                    .header("Content-Type", "application/json")
                    .body(Body::from(r#"{"enabled":false}"#))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    #[tokio::test]
    async fn trusted_source_control_storage_errors_are_not_default_on_or_acceptance() {
        let (asm, _, _) = crate::test_helpers::build_ttl_refresh_asm(true).await;
        let key = setup_test_api_rw_key(&asm);
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
            asm.clone(),
        ))));
        for malformed in [false, true] {
            asm.state_db.del("trusted-source-controls").await.unwrap();
            if malformed {
                asm.state_db
                    .hset(
                        "trusted-source-controls",
                        "hr",
                        r#"{"enabled":false,"revision":"bad","pending":true}"#,
                    )
                    .await
                    .unwrap();
            } else {
                asm.state_db
                    .set("trusted-source-controls", "wrong type")
                    .await
                    .unwrap();
            }
            for method in ["GET", "PUT"] {
                let response = app
                    .clone()
                    .oneshot(
                        Request::builder()
                            .method(method)
                            .uri("/admin/trusted-services/hr/control")
                            .header("X-API-Key", &key)
                            .header("Content-Type", "application/json")
                            .body(Body::from(r#"{"enabled":false}"#))
                            .unwrap(),
                    )
                    .await
                    .unwrap();
                assert_eq!(response.status(), StatusCode::SERVICE_UNAVAILABLE);
            }
        }
        for source in ["bad.id", "bad__source__id"] {
            let response = app
                .clone()
                .oneshot(
                    Request::builder()
                        .uri(format!("/admin/trusted-services/{source}/control"))
                        .header("X-API-Key", &key)
                        .body(Body::empty())
                        .unwrap(),
                )
                .await
                .unwrap();
            assert_eq!(response.status(), StatusCode::BAD_REQUEST);
        }
    }

    /// GET /admin/policies/curr returns the current policy as a base64;zip PolicyBundle.
    #[tokio::test]
    async fn test_get_curr_policy_ok() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/policies/curr")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let bundle: PolicyBundle = serde_json::from_slice(&body).unwrap();
        assert_eq!(bundle.config_id, DEFAULT_POLICY_ID);
        assert!(bundle.format.starts_with("base64;zip;"));
        assert!(!bundle.container.is_empty());
    }

    /// GET /admin/policies/0 returns the current policy bundle (id 0 is the default).
    #[tokio::test]
    async fn test_get_policy_by_id_zero_ok() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/policies/0")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let bundle: PolicyBundle = serde_json::from_slice(&body).unwrap();
        assert!(bundle.format.starts_with("base64;zip;"));
    }

    /// GET /admin/policies/<non-zero> is rejected because only id 0 currently exists.
    #[tokio::test]
    async fn test_get_policy_not_found() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/policies/1")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    /// POST /admin/policies installs a valid bundle and the new policy is then
    /// served by GET /admin/policies/curr.
    #[tokio::test]
    async fn test_install_policy_ok() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_rw_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));

        let bundle = make_valid_policy_bundle();
        let body = serde_json::to_vec(&bundle).unwrap();

        let app = admin_app(shared_state.clone());
        let response = app
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/admin/policies")
                    .header("X-API-Key", &api_key)
                    .header("Content-Type", "application/json")
                    .body(Body::from(body))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let entry: ListEntry = serde_json::from_slice(&body).unwrap();
        assert_eq!(entry.id, DEFAULT_POLICY_ID);

        // The installed policy should now be the current one.
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/policies/curr")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        let curr: PolicyBundle = serde_json::from_slice(&body).unwrap();
        assert_eq!(curr.decode().unwrap(), bundle.decode().unwrap());
    }

    /// POST /admin/policies rejects a bundle whose container can't be decoded.
    #[tokio::test]
    async fn test_install_policy_bad_bundle() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_rw_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));

        // Valid JSON shape, but an unsupported format makes decode() fail.
        let bad = PolicyBundle {
            config_id: 0,
            version: String::new(),
            format: "hex;zip;0.11.1".to_string(),
            container: String::new(),
        };
        let body = serde_json::to_vec(&bad).unwrap();

        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/admin/policies")
                    .header("X-API-Key", &api_key)
                    .header("Content-Type", "application/json")
                    .body(Body::from(body))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    }

    /// POST /admin/policies is forbidden for a read-only key (no write permission).
    #[tokio::test]
    async fn test_install_policy_forbidden_without_write() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));

        let bundle = make_valid_policy_bundle();
        let body = serde_json::to_vec(&bundle).unwrap();

        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("POST")
                    .uri("/admin/policies")
                    .header("X-API-Key", &api_key)
                    .header("Content-Type", "application/json")
                    .body(Body::from(body))
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::FORBIDDEN);
    }

    #[tokio::test]
    async fn test_get_visas_no_visas() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();

        let visas: Vec<ListEntry> = serde_json::from_slice(&body).unwrap();
        assert!(visas.is_empty());
    }

    #[tokio::test]
    async fn test_get_visas_one_visa() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);

        let node_addr: IpAddr = "fd5a:5052:90de::1".parse().unwrap();
        let pdesc =
            PacketDesc::new_tcp("fd5a:5052:3000::1", "fd5a:5052:3000::2", 12345, 80).unwrap();
        let hit = Hit::new_no_signal(0, Direction::Forward);

        let route = Route::new_direct(node_addr.into());

        // Add a visa.
        let vwmd = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc,
                &hit,
                &route,
                "",
                0,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();

        let created_id = vwmd.visa.issuer_id;
        assert!(created_id > 0);

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();

        let visas: Vec<ListEntry> = serde_json::from_slice(&body).unwrap();
        assert_eq!(visas.len(), 1);
        assert_eq!(visas[0].id, created_id);
    }

    /// Drives `GET /admin/visas/denies` with the given query string and returns
    /// the status plus the decoded body.
    async fn fetch_denies(
        asm: &Arc<Assembly>,
        api_key: &str,
        query: &str,
    ) -> (StatusCode, Vec<DenyRecord>) {
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri(format!("/admin/visas/denies{query}"))
                    .header("X-API-Key", api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        let status = response.status();
        let body = response.into_body().collect().await.unwrap().to_bytes();
        // A non-OK response has no DenyRecord list body.
        let records = if status == StatusCode::OK {
            serde_json::from_slice(&body).unwrap()
        } else {
            Vec::new()
        };
        (status, records)
    }

    /// Records three denies at known times so the ordering and query-filter
    /// assertions below are deterministic.
    fn seed_denies(asm: &Arc<Assembly>) {
        for (i, dport) in [80u16, 443, 8080].into_iter().enumerate() {
            let ft = PacketDesc::new_tcp(
                "fd5a:5052:3000::1",
                "fd5a:5052:3000::2",
                12345,
                dport.into(),
            )
            .unwrap()
            .five_tuple;
            asm.deny_log
                .record_at_ms(&ft, &DenyCode::NoMatch, 1_000 + i as u64 * 1_000);
        }
    }

    #[tokio::test]
    async fn test_get_denies_empty() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let (status, denies) = fetch_denies(&asm, &api_key, "").await;
        assert_eq!(status, StatusCode::OK);
        assert!(denies.is_empty());
    }

    #[tokio::test]
    async fn test_get_denies_newest_first() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        seed_denies(&asm);

        let (status, denies) = fetch_denies(&asm, &api_key, "").await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(
            denies.iter().map(|d| d.dest_port).collect::<Vec<_>>(),
            vec![8080, 443, 80]
        );

        let newest = &denies[0];
        assert_eq!(
            newest.source_addr,
            "fd5a:5052:3000::1".parse::<IpAddr>().unwrap()
        );
        assert_eq!(
            newest.dest_addr,
            "fd5a:5052:3000::2".parse::<IpAddr>().unwrap()
        );
        assert_eq!(newest.protocol, ip_proto::TCP);
        assert_eq!(newest.count, 1);
        assert_eq!(newest.last_deny_ms, 3_000);
        assert_eq!(newest.deny_code, "NoMatch");
    }

    #[tokio::test]
    async fn test_get_denies_since_and_limit() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        seed_denies(&asm);

        // `since` is inclusive, so the entry recorded exactly at 2000 is returned.
        let (status, denies) = fetch_denies(&asm, &api_key, "?since=2000").await;
        assert_eq!(status, StatusCode::OK);
        assert_eq!(
            denies.iter().map(|d| d.last_deny_ms).collect::<Vec<_>>(),
            vec![3_000, 2_000]
        );

        let (_, denies) = fetch_denies(&asm, &api_key, "?limit=1").await;
        assert_eq!(denies.len(), 1);
        assert_eq!(denies[0].last_deny_ms, 3_000);

        let (_, denies) = fetch_denies(&asm, &api_key, "?since=1000&limit=2").await;
        assert_eq!(
            denies.iter().map(|d| d.last_deny_ms).collect::<Vec<_>>(),
            vec![3_000, 2_000]
        );
    }

    #[tokio::test]
    async fn test_get_denies_rejects_malformed_query() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        for query in ["?since=-1", "?since=abc", "?limit=-5", "?limit=1.5"] {
            let (status, _) = fetch_denies(&asm, &api_key, query).await;
            assert_eq!(
                status,
                StatusCode::BAD_REQUEST,
                "expected 400 for query {query}"
            );
        }
    }

    #[tokio::test]
    async fn test_get_visas_three_visas() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);

        let node_addr: IpAddr = "fd5a:5052:90de::1".parse().unwrap();
        let hit = Hit::new_no_signal(0, Direction::Forward);

        // Add three visas with distinct packet descriptors.
        let pdesc0 =
            PacketDesc::new_tcp("fd5a:5052:3000::1", "fd5a:5052:3000::2", 12345, 80).unwrap();
        let pdesc1 =
            PacketDesc::new_tcp("fd5a:5052:3000::3", "fd5a:5052:3000::4", 12346, 443).unwrap();
        let pdesc2 =
            PacketDesc::new_tcp("fd5a:5052:3000::5", "fd5a:5052:3000::6", 12347, 22).unwrap();

        let route = Route::new_direct(node_addr.into());

        let v0 = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc0,
                &hit,
                &route,
                "",
                0,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();
        let v1 = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc1,
                &hit,
                &route,
                "",
                0,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();
        let v2 = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc2,
                &hit,
                &route,
                "",
                0,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();

        let mut created_ids = vec![v0.visa.issuer_id, v1.visa.issuer_id, v2.visa.issuer_id];
        created_ids.sort_unstable();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();

        let visas: Vec<ListEntry> = serde_json::from_slice(&body).unwrap();
        assert_eq!(visas.len(), 3);

        let mut returned_ids: Vec<u64> = visas.into_iter().map(|v| v.id).collect();
        returned_ids.sort_unstable();
        assert_eq!(returned_ids, created_ids);
    }

    #[tokio::test]
    async fn test_get_actors_no_actors() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();

        let actors: Vec<CnEntry> = serde_json::from_slice(&body).unwrap();
        assert!(actors.is_empty());
    }

    #[tokio::test]
    async fn test_get_actors_one_actor() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let actor = make_node_actor_defexp("fd5a:5052::10", "node-1", "[fd5a:5052::100]:1234");
        asm.actor_mgr.add_node(&actor, false).await.unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();

        let actors: Vec<CnEntry> = serde_json::from_slice(&body).unwrap();
        assert_eq!(actors.len(), 1);
        assert_eq!(actors[0].cn, "node-1");
    }

    #[tokio::test]
    async fn test_get_actors_multiple_actors() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let actor0 = make_node_actor_defexp("fd5a:5052::11", "node-1", "[fd5a:5052::101]:1234");
        let actor1 = make_node_actor_defexp("fd5a:5052::12", "node-2", "[fd5a:5052::102]:1234");
        let actor2 = make_node_actor_defexp("fd5a:5052::13", "node-3", "[fd5a:5052::103]:1234");

        asm.actor_mgr.add_node(&actor0, false).await.unwrap();
        asm.actor_mgr.add_node(&actor1, false).await.unwrap();
        asm.actor_mgr.add_node(&actor2, false).await.unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();

        let mut actors: Vec<CnEntry> = serde_json::from_slice(&body).unwrap();
        assert_eq!(actors.len(), 3);
        actors.sort_by(|a, b| a.cn.cmp(&b.cn));
        let actor_cns: Vec<String> = actors.into_iter().map(|a| a.cn).collect();
        assert_eq!(
            actor_cns,
            vec![
                "node-1".to_string(),
                "node-2".to_string(),
                "node-3".to_string()
            ]
        );
    }

    #[tokio::test]
    async fn test_get_actors_role_filter() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let node_actor = make_node_actor_defexp("fd5a:5052::20", "node-1", "[fd5a:5052::120]:1234");
        let adapter_actor = make_adapter_actor_defexp("fd5a:5052::21", "adapter-1");

        asm.actor_mgr.add_node(&node_actor, false).await.unwrap();
        asm.actor_mgr
            .add_adapter_via_node(&adapter_actor, node_actor.get_zpr_addr().unwrap())
            .await
            .unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);

        let response_nodes = app
            .clone()
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors?role=node")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response_nodes.status(), StatusCode::OK);

        let body_nodes = response_nodes
            .into_body()
            .collect()
            .await
            .unwrap()
            .to_bytes();
        let actors_nodes: Vec<CnEntry> = serde_json::from_slice(&body_nodes).unwrap();
        assert_eq!(actors_nodes.len(), 1);
        assert_eq!(actors_nodes[0].cn, "node-1");

        let response_adapters = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors?role=adapter")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response_adapters.status(), StatusCode::OK);

        let body_adapters = response_adapters
            .into_body()
            .collect()
            .await
            .unwrap()
            .to_bytes();
        let actors_adapters: Vec<CnEntry> = serde_json::from_slice(&body_adapters).unwrap();
        assert_eq!(actors_adapters.len(), 1);
        assert_eq!(actors_adapters[0].cn, "adapter-1");
    }

    #[tokio::test]
    async fn test_get_visa_not_found() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/visas/9999")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    #[tokio::test]
    async fn test_get_visa_fields_forward_no_signal() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);

        let node_addr: IpAddr = "fd5a:5052:90de::1".parse().unwrap();
        let pdesc =
            PacketDesc::new_tcp("fd5a:5052:3000::1", "fd5a:5052:3000::2", 12345, 80).unwrap();
        let zpl_str = "permit tcp from groupA to groupB port 80";
        let policy_version: u64 = 42;
        let hit = Hit::new_no_signal(0, Direction::Forward);
        let route = Route::new_direct(node_addr.into());

        let v = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc,
                &hit,
                &route,
                zpl_str,
                policy_version,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri(&format!("/admin/visas/{}", v.visa.issuer_id))
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let vd: VisaDescriptor = serde_json::from_slice(&body).unwrap();

        assert_eq!(vd.id, v.visa.issuer_id);
        assert_eq!(vd.policy_id, "42");
        assert_eq!(vd.zpl, zpl_str);
        assert_eq!(vd.direction, VisaMatchDirection::Forward);
        assert!(vd.signals.is_empty());

        // Verify JSON encodes direction as lowercase string.
        let raw: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(raw["direction"], "forward");
    }

    #[tokio::test]
    async fn test_get_visa_direction_reverse() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);

        let node_addr: IpAddr = "fd5a:5052:90de::1".parse().unwrap();
        let pdesc =
            PacketDesc::new_tcp("fd5a:5052:3000::1", "fd5a:5052:3000::2", 12345, 80).unwrap();
        let hit = Hit::new_no_signal(0, Direction::Reverse);
        let route = Route::new_direct(node_addr.into());

        let v = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc,
                &hit,
                &route,
                "",
                0,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri(&format!("/admin/visas/{}", v.visa.issuer_id))
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let vd: VisaDescriptor = serde_json::from_slice(&body).unwrap();
        assert_eq!(vd.direction, VisaMatchDirection::Reverse);

        let raw: serde_json::Value = serde_json::from_slice(&body).unwrap();
        assert_eq!(raw["direction"], "reverse");
    }

    #[tokio::test]
    async fn test_get_visa_with_signal() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);

        let node_addr: IpAddr = "fd5a:5052:90de::1".parse().unwrap();
        let pdesc =
            PacketDesc::new_tcp("fd5a:5052:3000::1", "fd5a:5052:3000::2", 12345, 80).unwrap();
        let signal = Signal {
            message: "alert: suspicious traffic".to_string(),
            service: "svc1".to_string(),
        };
        let hit = Hit::new_with_signal(0, Direction::Forward, signal);
        let route = Route::new_direct(node_addr.into());

        let v = asm
            .visa_mgr
            .create_visa(
                &asm,
                &node_addr,
                &pdesc,
                &hit,
                &route,
                "",
                0,
                0,
                SystemTime::now() + DEFAULT_VISA_EXPIRATION,
            )
            .await
            .unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri(&format!("/admin/visas/{}", v.visa.issuer_id))
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let vd: VisaDescriptor = serde_json::from_slice(&body).unwrap();
        assert_eq!(vd.signals, vec!["alert: suspicious traffic".to_string()]);
    }

    #[tokio::test]
    async fn test_get_actors_invalid_role_filter() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);

        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors?role=invalid")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    }

    #[tokio::test]
    /// The test policy declares no topology, so there are no links to report.
    async fn test_get_network_empty() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let details = fetch_network(&asm).await;
        assert!(details.network.is_empty());
    }

    /// GET /admin/network and deserialize the response body.
    async fn fetch_network(asm: &Arc<Assembly>) -> NetworkDetails {
        let api_key = setup_test_api_r_key(asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let response = admin_app(shared_state)
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/network")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);
        let body = response.into_body().collect().await.unwrap().to_bytes();
        serde_json::from_slice(&body).unwrap()
    }

    /// Install a policy peering `a` and `b` over `link-ab`, with a link cost attribute.
    async fn install_ab_peering(asm: &Arc<Assembly>, a: IpAddr, b: IpAddr) {
        let attrs = vec![AttrExp {
            key: libeval::attribute::key::LINK_COST.to_string(),
            op: zpr::policy_types::AttrOp::Eq,
            value: vec!["7".to_string()],
        }];
        asm.policy_mgr
            .update_policy_from_container_bytes(policy_with_peerings(&[make_peering(
                a, b, "link-ab", attrs,
            )]))
            .await
            .unwrap();
    }

    #[tokio::test]
    /// A link the policy declares but that no connected node reports is DOWN, and it
    /// carries the link detail (id, substrate, cost, attrs) taken from policy.
    async fn test_get_network_declared_link_down() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let addr_a: IpAddr = "fd5a:5052::10".parse().unwrap();
        let addr_b: IpAddr = "fd5a:5052::11".parse().unwrap();
        install_ab_peering(&asm, addr_a, addr_b).await;

        let details = fetch_network(&asm).await;

        assert_eq!(details.network.len(), 1);
        for nc in &details.network {
            assert!(nc.is_link_between(&addr_a, &addr_b));
            assert!(matches!(nc.ctype, ConnectionType::DOWN));
            assert_eq!(nc.link_id, "link-ab");
            assert_eq!(nc.link_cost, 7);
            // The fixture uses each node's ZPR (IPv6) address as its substrate, so both
            // ends must come back bracketed and parseable.
            assert_eq!(nc.node_a_substrate, format!("[{}]:0", nc.node_a_addr));
            assert_eq!(nc.node_b_substrate, format!("[{}]:0", nc.node_b_addr));
            assert_eq!(nc.link_attrs.len(), 1);
        }
    }

    #[tokio::test]
    /// A declared link that the connected nodes also report is UP.
    async fn test_get_network_declared_link_up() {
        use libeval::route::LinkId;

        let asm = Arc::new(new_assembly_for_tests(None).await);
        let addr_a: IpAddr = "fd5a:5052::10".parse().unwrap();
        let addr_b: IpAddr = "fd5a:5052::11".parse().unwrap();
        install_ab_peering(&asm, addr_a, addr_b).await;

        let actor_a = make_node_actor_defexp("fd5a:5052::10", "node-a", "[fd5a:5052::100]:1234");
        let actor_b = make_node_actor_defexp("fd5a:5052::11", "node-b", "[fd5a:5052::101]:1234");
        asm.actor_mgr.add_node(&actor_a, false).await.unwrap();
        asm.actor_mgr.add_node(&actor_b, false).await.unwrap();
        asm.topo_mgr.add_node(addr_a).unwrap();
        asm.topo_mgr.add_node(addr_b).unwrap();
        asm.topo_mgr
            .add_link(addr_a, addr_b, LinkId("link-ab".into()), vec![], 1)
            .unwrap();

        let details = fetch_network(&asm).await;

        assert_eq!(details.network.len(), 1);
        for nc in &details.network {
            assert!(nc.is_link_between(&addr_a, &addr_b));
            assert!(matches!(nc.ctype, ConnectionType::UP));
        }
    }

    #[tokio::test]
    /// A link the nodes report but that the policy does not declare is INVALID.
    async fn test_get_network_undeclared_link_invalid() {
        use libeval::route::LinkId;

        let asm = Arc::new(new_assembly_for_tests(None).await);
        let addr_a: IpAddr = "fd5a:5052::10".parse().unwrap();
        let addr_b: IpAddr = "fd5a:5052::11".parse().unwrap();

        // Note: no policy peering installed, so this link is undeclared.
        let actor_a = make_node_actor_defexp("fd5a:5052::10", "node-a", "[fd5a:5052::100]:1234");
        let actor_b = make_node_actor_defexp("fd5a:5052::11", "node-b", "[fd5a:5052::101]:1234");
        asm.actor_mgr.add_node(&actor_a, false).await.unwrap();
        asm.actor_mgr.add_node(&actor_b, false).await.unwrap();
        asm.topo_mgr.add_node(addr_a).unwrap();
        asm.topo_mgr.add_node(addr_b).unwrap();
        asm.topo_mgr
            .add_link(addr_a, addr_b, LinkId("link-ab".into()), vec![], 1)
            .unwrap();

        let details = fetch_network(&asm).await;

        assert_eq!(details.network.len(), 1);
        assert!(
            matches!(details.network[0].ctype, ConnectionType::INVALID),
            "got {:?}",
            details.network[0].ctype
        );
    }

    /// Build a Scope with the given protocol, single port, and port range.
    fn scope(protocol: u8, port: Option<u16>, port_range: Option<(u16, u16)>) -> Scope {
        Scope {
            protocol,
            flag: None,
            port,
            port_range,
        }
    }

    /// service_endpoints_to_string formats known protocols, port ranges, unknown
    /// protocols, and the empty list as expected.
    #[test]
    fn test_service_endpoints_to_string() {
        // Empty list -> empty string.
        assert_eq!(service_endpoints_to_string(&[]), "");

        // Single port with each known protocol.
        assert_eq!(
            service_endpoints_to_string(&[scope(ip_proto::TCP, Some(80), None)]),
            "TCP/80"
        );
        assert_eq!(
            service_endpoints_to_string(&[scope(ip_proto::UDP, Some(53), None)]),
            "UDP/53"
        );

        // Port range.
        assert_eq!(
            service_endpoints_to_string(&[scope(ip_proto::TCP, None, Some((8000, 8080)))]),
            "TCP/8000-8080"
        );

        // Unknown protocol falls back to PROTO_<n>.
        assert_eq!(
            service_endpoints_to_string(&[scope(200, Some(1), None)]),
            "PROTO_200/1"
        );

        // Multiple endpoints are comma-joined in order.
        assert_eq!(
            service_endpoints_to_string(&[
                scope(ip_proto::TCP, Some(80), None),
                scope(ip_proto::UDP, Some(53), None),
            ]),
            "TCP/80,UDP/53"
        );
    }

    /// GET /admin/nodes/{cn}/visas for a node with no installed visas returns OK
    /// with an empty list.
    #[tokio::test]
    async fn test_get_visas_on_node_empty_ok() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let node = make_node_actor_defexp("fd5a:5052::30", "node-1", "[fd5a:5052::130]:1234");
        asm.actor_mgr.add_node(&node, false).await.unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/nodes/node-1/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::OK);

        let body = response.into_body().collect().await.unwrap().to_bytes();
        let visas: Vec<ListEntry> = serde_json::from_slice(&body).unwrap();
        assert!(visas.is_empty());
    }

    /// GET /admin/nodes/{cn}/visas for a non-node actor is rejected with BAD_REQUEST.
    #[tokio::test]
    async fn test_get_visas_on_node_non_node_bad_request() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let node = make_node_actor_defexp("fd5a:5052::31", "node-1", "[fd5a:5052::131]:1234");
        let adapter = make_adapter_actor_defexp("fd5a:5052::32", "adapter-1");
        asm.actor_mgr.add_node(&node, false).await.unwrap();
        asm.actor_mgr
            .add_adapter_via_node(&adapter, node.get_zpr_addr().unwrap())
            .await
            .unwrap();

        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/nodes/adapter-1/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::BAD_REQUEST);
    }

    /// GET /admin/nodes/{cn}/visas for an unknown CN returns NOT_FOUND.
    #[tokio::test]
    async fn test_get_visas_on_node_unknown_cn_not_found() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/nodes/does-not-exist/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    /// GET /admin/actors/{cn}/visas for an unknown CN returns NOT_FOUND (exercises
    /// the shared resolve_actor_addr helper with require_node = false).
    #[tokio::test]
    async fn test_get_related_visas_unknown_cn_not_found() {
        let asm = Arc::new(new_assembly_for_tests(None).await);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("GET")
                    .uri("/admin/actors/does-not-exist/visas")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
    }

    const FLUSH_TS_ID: &str = "ts-file";

    /// Minimal trusted service that only records whether it was flushed.
    struct FlushCountingService {
        flushes: std::sync::atomic::AtomicUsize,
    }

    #[async_trait::async_trait]
    impl crate::trusted_services::TrustedServiceInterface for FlushCountingService {
        async fn get_attributes_for_actor(
            &self,
            _identities: &[(String, String)],
        ) -> Result<Vec<Attribute>, ServiceError> {
            Ok(Vec::new())
        }

        async fn flush(&self) -> Result<(), ServiceError> {
            self.flushes
                .fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            Ok(())
        }

        // Holds no snapshot; its flush count doubles as the revision.
        fn current_revision(&self) -> u64 {
            self.flushes.load(std::sync::atomic::Ordering::SeqCst) as u64
        }

        fn get_source_id(&self) -> &str {
            FLUSH_TS_ID
        }
    }

    /// Register one flush-counting trusted service on the assembly and return it for
    /// assertions.
    fn register_flush_counting_service(asm: &Arc<Assembly>) -> Arc<FlushCountingService> {
        let svc = Arc::new(FlushCountingService {
            flushes: std::sync::atomic::AtomicUsize::new(0),
        });
        asm.ts_mgr.update_services(vec![svc.clone()]);
        svc
    }

    /// DELETE /admin/services/{id}/cache flushes the named trusted service.
    #[tokio::test]
    async fn test_flush_service_cache_ok() {
        let (asm, mut event_rx) = new_assembly_with_event_rx(None).await;
        let asm = Arc::new(asm);
        let svc = register_flush_counting_service(&asm);
        let api_key = setup_test_api_rw_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("DELETE")
                    .uri(format!("/admin/services/{FLUSH_TS_ID}/cache"))
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::ACCEPTED);
        assert_eq!(svc.flushes.load(std::sync::atomic::Ordering::SeqCst), 1);
        // The revalidation of existing visas is queued for the event worker.
        match event_rx.try_recv() {
            Ok(VsEvent::TrustedServiceChange) => {}
            other => panic!("expected TrustedServiceChange event, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_flush_service_cache_durable_retry_after_worker_restart() {
        let (asm, event_rx) = new_assembly_with_event_rx(None).await;
        let asm = Arc::new(asm);
        let svc = register_flush_counting_service(&asm);
        let api_key = setup_test_api_rw_key(&asm);
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(
            asm.clone(),
        ))));
        drop(event_rx); // Lost wake-up / dead worker must not lose accepted work.
        for _ in 0..2 {
            let response = app
                .clone()
                .oneshot(
                    Request::builder()
                        .method("DELETE")
                        .uri(format!("/admin/services/{FLUSH_TS_ID}/cache"))
                        .header("X-API-Key", &api_key)
                        .body(Body::empty())
                        .unwrap(),
                )
                .await
                .unwrap();
            assert_eq!(response.status(), StatusCode::ACCEPTED);
        }
        let read_status = || async {
            let response = app
                .clone()
                .oneshot(
                    Request::builder()
                        .uri(format!("/admin/services/{FLUSH_TS_ID}/cache"))
                        .header("X-API-Key", &api_key)
                        .body(Body::empty())
                        .unwrap(),
                )
                .await
                .unwrap();
            assert_eq!(response.status(), StatusCode::OK);
            serde_json::from_slice::<serde_json::Value>(
                &response.into_body().collect().await.unwrap().to_bytes(),
            )
            .unwrap()
        };
        assert_eq!(read_status().await["state"], "pending");
        asm.ts_mgr.update_services(vec![]);
        crate::trusted_reconciliation::process_pending(&asm).await;
        assert_eq!(read_status().await["state"], "failed");
        // A new processor has no in-memory queue or retry state; it discovers the
        // persisted request, reloads and completes the coalesced duplicates.
        asm.ts_mgr.update_services(vec![svc.clone()]);
        crate::trusted_reconciliation::process_pending(&asm).await;
        let status = read_status().await;
        assert_eq!(status["state"], "idle");
        assert_eq!(status["completed"], 2);
        assert_eq!(svc.flushes.load(std::sync::atomic::Ordering::SeqCst), 3);
        crate::trusted_reconciliation::process_pending(&asm).await;
        assert_eq!(svc.flushes.load(std::sync::atomic::Ordering::SeqCst), 3);
    }

    #[tokio::test]
    async fn test_flush_service_cache_enqueue_failure_returns_unavailable() {
        let (asm, mut event_rx) = new_assembly_with_event_rx(None).await;
        let asm = Arc::new(asm);
        register_flush_counting_service(&asm);
        let encoded =
            percent_encoding::utf8_percent_encode(FLUSH_TS_ID, percent_encoding::NON_ALPHANUMERIC);
        asm.state_db
            .hset(
                &format!("trusted-reconciliation:requested:{encoded}"),
                "invalid",
                "value",
            )
            .await
            .unwrap();
        let api_key = setup_test_api_rw_key(&asm);
        let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(asm))));
        let response = app
            .oneshot(
                Request::builder()
                    .method("DELETE")
                    .uri(format!("/admin/services/{FLUSH_TS_ID}/cache"))
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::SERVICE_UNAVAILABLE);
        assert!(event_rx.try_recv().is_err());
    }

    /// DELETE /admin/services/{id}/cache for an unregistered service returns NOT_FOUND.
    #[tokio::test]
    async fn test_flush_service_cache_unknown_not_found() {
        let (asm, mut event_rx) = new_assembly_with_event_rx(None).await;
        let asm = Arc::new(asm);
        register_flush_counting_service(&asm);
        let api_key = setup_test_api_rw_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("DELETE")
                    .uri("/admin/services/does-not-exist/cache")
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
        // Nothing was invalidated, so nothing needs revalidating.
        assert!(event_rx.try_recv().is_err());
    }

    /// A read-only key may not flush a trusted service.
    #[tokio::test]
    async fn test_flush_service_cache_read_key_forbidden() {
        let (asm, mut event_rx) = new_assembly_with_event_rx(None).await;
        let asm = Arc::new(asm);
        let svc = register_flush_counting_service(&asm);
        let api_key = setup_test_api_r_key(&asm);
        let shared_state = Arc::new(tokio::sync::RwLock::new(AdminState::new(asm.clone())));
        let app = admin_app(shared_state);
        let response = app
            .oneshot(
                Request::builder()
                    .method("DELETE")
                    .uri(format!("/admin/services/{FLUSH_TS_ID}/cache"))
                    .header("X-API-Key", &api_key)
                    .body(Body::empty())
                    .unwrap(),
            )
            .await
            .unwrap();
        assert_eq!(response.status(), StatusCode::FORBIDDEN);
        assert_eq!(svc.flushes.load(std::sync::atomic::Ordering::SeqCst), 0);
        assert!(event_rx.try_recv().is_err());
    }
}
