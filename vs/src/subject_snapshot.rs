//! Offline complete subject-link snapshot verification.
//!
//! No listener, device admission, publication or enforcement is enabled here.
//! The optional offline checkpoint module persists accepted envelopes and their
//! ordering state; it does not provide a network acknowledgement.

#[cfg(unix)]
pub mod checkpoint;
pub mod fetch;

use std::collections::{HashMap, HashSet};

use base64::{Engine, engine::general_purpose::URL_SAFE_NO_PAD};
use openssl::{pkey::PKey, sign::Verifier};
use serde::{Deserialize, Serialize};
use thiserror::Error;

pub const DOMAIN: &[u8] = b"ZPR-SUBJECT-LINK-SNAPSHOT-V1\0";
pub const MAX_PAYLOAD: usize = 1 << 20;
pub const MAX_ENVELOPE: usize = 1_400_000;
pub const MAX_RECORDS: usize = 1024;

#[derive(Debug, Error)]
pub enum SnapshotError {
    #[error("invalid subject-link snapshot")]
    Invalid,
    #[error("subject-link snapshot authentication failed")]
    Trust,
    #[error("subject-link snapshot rollback, equivocation or epoch recovery required")]
    Replay,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Authority {
    pub authority_id: String,
    pub version: String,
    pub kind: String,
    pub service_id: String,
    pub issuer: String,
    pub enabled: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Source {
    pub source_id: String,
    pub version: String,
    pub provider_id: String,
    pub selector_kind: String,
    pub enrichment_keys: Vec<String>,
    pub enabled: bool,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Root {
    pub authority_id: String,
    pub authority_version: String,
    pub subject_kind: String,
    pub subject_id: String,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Target {
    pub source_id: String,
    pub source_version: String,
    pub selector_kind: String,
    pub subject_id: String,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Binding {
    pub binding_id: String,
    pub version: String,
    pub root: Root,
    pub target: Target,
    pub enrichment_keys: Vec<String>,
    pub not_before: String,
    pub expires_at: String,
}

#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Tombstone {
    pub binding_id: String,
    pub version: String,
}

/// Times are positive canonical decimal Unix seconds, not floating-point values.
/// This contract supplies no clock skew, lease duration or runtime defaults.
#[derive(Clone, Debug, Serialize, Deserialize, PartialEq)]
#[serde(deny_unknown_fields)]
pub struct Payload {
    pub version: u8,
    pub organization_id: String,
    pub vs_id: String,
    pub epoch: String,
    pub sequence: String,
    pub issued_at: String,
    pub expires_at: String,
    pub authorities: Vec<Authority>,
    pub sources: Vec<Source>,
    pub bindings: Vec<Binding>,
    pub tombstones: Vec<Tombstone>,
}

#[derive(Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct Envelope {
    version: u8,
    key_id: String,
    payload: String,
    signature: String,
}

pub struct Scope<'a> {
    pub organization: &'a str,
    pub recipient: &'a str,
    pub epoch: &'a str,
}

/// The private fields prevent unverified JSON becoming accepted state.
#[derive(Clone, Debug)]
pub struct Verified {
    payload: Payload,
    digest: String,
}

impl Verified {
    pub fn digest(&self) -> &str {
        &self.digest
    }
}

#[derive(Debug, PartialEq)]
pub enum Decision {
    Advance,
    HistoricalReplay,
}

fn id(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 128
        && value
            .bytes()
            .all(|c| c.is_ascii_alphanumeric() || c == b'_' || c == b'-')
}

fn decimal(value: &str, limit: u64) -> Option<u64> {
    let n: u64 = value.parse().ok()?;
    (n > 0 && n <= limit && n.to_string() == value).then_some(n)
}

fn text(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= 256
        && value.trim() == value
        && value.bytes().all(|c| (32..=126).contains(&c))
}

fn keys(values: &[String]) -> bool {
    let mut seen = HashSet::new();
    !values.is_empty()
        && values.len() <= 128
        && values.iter().all(|v| {
            !v.is_empty()
                && v.len() <= 256
                && (v.as_bytes()[0].is_ascii_alphabetic() || v.as_bytes()[0] == b'_')
                && v.bytes()
                    .all(|c| c.is_ascii_alphanumeric() || b"_.-".contains(&c))
                && seen.insert(v)
        })
}

fn uuid(value: &str) -> bool {
    value.len() == 36
        && value != "00000000-0000-0000-0000-000000000000"
        && value.bytes().enumerate().all(|(i, c)| {
            if [8, 13, 18, 23].contains(&i) {
                c == b'-'
            } else {
                c.is_ascii_digit() || (b'a'..=b'f').contains(&c)
            }
        })
}

impl Payload {
    fn validate(&self) -> Result<(), SnapshotError> {
        let invalid = SnapshotError::Invalid;
        let issued = decimal(&self.issued_at, i64::MAX as u64).ok_or(invalid)?;
        let expires = decimal(&self.expires_at, i64::MAX as u64).ok_or(SnapshotError::Invalid)?;
        if self.version != 1
            || !id(&self.organization_id)
            || !id(&self.vs_id)
            || !id(&self.epoch)
            || decimal(&self.sequence, u64::MAX).is_none()
            || expires <= issued
            || [
                self.authorities.len(),
                self.sources.len(),
                self.bindings.len(),
                self.tombstones.len(),
            ]
            .iter()
            .any(|&n| n > MAX_RECORDS)
        {
            return Err(SnapshotError::Invalid);
        }
        let mut authorities = HashMap::new();
        let mut tuples = HashSet::new();
        for a in &self.authorities {
            if !id(&a.authority_id)
                || decimal(&a.version, u64::MAX).is_none()
                || a.kind != "bas_device"
                || !text(&a.service_id)
                || !text(&a.issuer)
                || authorities.insert(&a.authority_id, a).is_some()
                || !tuples.insert((&a.kind, &a.service_id, &a.issuer))
            {
                return Err(SnapshotError::Invalid);
            }
        }
        let mut sources = HashMap::new();
        for s in &self.sources {
            if !id(&s.source_id)
                || decimal(&s.version, u64::MAX).is_none()
                || !id(&s.provider_id)
                || s.selector_kind != "ldap_entry_uuid"
                || !keys(&s.enrichment_keys)
                || sources.insert(&s.source_id, s).is_some()
            {
                return Err(SnapshotError::Invalid);
            }
        }
        let mut seen = HashSet::new();
        for b in &self.bindings {
            let start = decimal(&b.not_before, i64::MAX as u64).ok_or(SnapshotError::Invalid)?;
            let end = decimal(&b.expires_at, i64::MAX as u64).ok_or(SnapshotError::Invalid)?;
            let a = authorities
                .get(&b.root.authority_id)
                .ok_or(SnapshotError::Invalid)?;
            let s = sources
                .get(&b.target.source_id)
                .ok_or(SnapshotError::Invalid)?;
            if !id(&b.binding_id)
                || decimal(&b.version, u64::MAX).is_none()
                || !seen.insert(&b.binding_id)
                || end <= start
                || end <= issued
                || !a.enabled
                || a.version != b.root.authority_version
                || b.root.subject_kind != "device"
                || !text(&b.root.subject_id)
                || !s.enabled
                || s.version != b.target.source_version
                || b.target.selector_kind != s.selector_kind
                || !uuid(&b.target.subject_id)
                || !keys(&b.enrichment_keys)
                || b.enrichment_keys
                    .iter()
                    .any(|key| !s.enrichment_keys.contains(key))
            {
                return Err(SnapshotError::Invalid);
            }
        }
        for t in &self.tombstones {
            if !id(&t.binding_id)
                || decimal(&t.version, u64::MAX).is_none()
                || !seen.insert(&t.binding_id)
            {
                return Err(SnapshotError::Invalid);
            }
        }
        Ok(())
    }
}

/// Only caller-provisioned pins are trusted. key_id is an untrusted lookup hint.
/// Authenticate exact bytes before decoding the complete payload; never
/// canonicalize or reserialize payload bytes for signature verification.
pub fn verify(
    bytes: &[u8],
    pins: &HashMap<String, Vec<u8>>,
    scope: &Scope<'_>,
    now: u64,
) -> Result<Verified, SnapshotError> {
    if bytes.len() > MAX_ENVELOPE
        || pins.is_empty()
        || pins.len() > 64
        || !id(scope.organization)
        || !id(scope.recipient)
        || !id(scope.epoch)
        || now == 0
        || now > i64::MAX as u64
    {
        return Err(SnapshotError::Invalid);
    }
    if pins.iter().any(|(name, key)| !id(name) || key.len() != 32) {
        return Err(SnapshotError::Trust);
    }
    let envelope: Envelope = serde_json::from_slice(bytes).map_err(|_| SnapshotError::Invalid)?;
    if envelope.version != 1 {
        return Err(SnapshotError::Invalid);
    }
    let raw = URL_SAFE_NO_PAD
        .decode(&envelope.payload)
        .map_err(|_| SnapshotError::Invalid)?;
    if raw.len() > MAX_PAYLOAD || URL_SAFE_NO_PAD.encode(&raw) != envelope.payload {
        return Err(SnapshotError::Invalid);
    }
    let signature = URL_SAFE_NO_PAD
        .decode(&envelope.signature)
        .map_err(|_| SnapshotError::Trust)?;
    let key = pins.get(&envelope.key_id).ok_or(SnapshotError::Trust)?;
    if signature.len() != 64 || URL_SAFE_NO_PAD.encode(&signature) != envelope.signature {
        return Err(SnapshotError::Trust);
    }
    let public = PKey::public_key_from_raw_bytes(key, openssl::pkey::Id::ED25519)
        .map_err(|_| SnapshotError::Trust)?;
    let mut verifier = Verifier::new_without_digest(&public).map_err(|_| SnapshotError::Trust)?;
    let mut signed = Vec::with_capacity(DOMAIN.len() + raw.len());
    signed.extend_from_slice(DOMAIN);
    signed.extend_from_slice(&raw);
    if !verifier
        .verify_oneshot(&signature, &signed)
        .map_err(|_| SnapshotError::Trust)?
    {
        return Err(SnapshotError::Trust);
    }
    let payload: Payload = serde_json::from_slice(&raw).map_err(|_| SnapshotError::Invalid)?;
    payload.validate()?;
    let issued = decimal(&payload.issued_at, i64::MAX as u64).ok_or(SnapshotError::Invalid)?;
    let expires = decimal(&payload.expires_at, i64::MAX as u64).ok_or(SnapshotError::Invalid)?;
    if payload.organization_id != scope.organization
        || payload.vs_id != scope.recipient
        || payload.epoch != scope.epoch
        || now < issued
        || now >= expires
    {
        return Err(SnapshotError::Invalid);
    }
    Ok(Verified {
        payload,
        digest: hex::encode(openssl::sha::sha256(&raw)),
    })
}

fn at_least(next: &str, prior: &str) -> bool {
    match (decimal(next, u64::MAX), decimal(prior, u64::MAX)) {
        (Some(n), Some(p)) => n >= p,
        _ => false,
    }
}

fn records_advance<T: PartialEq>(
    prior: &[T],
    next: &[T],
    retain: bool,
    identity: impl Fn(&T) -> (&str, &str),
) -> bool {
    let old: HashMap<_, _> = prior.iter().map(|v| (identity(v).0, v)).collect();
    let next_ids: HashSet<_> = next.iter().map(|v| identity(v).0).collect();
    (!retain || old.keys().all(|id| next_ids.contains(id)))
        && next.iter().all(|v| {
            let (id, version) = identity(v);
            match old.get(id) {
                Some(p) => {
                    let (_, prior_version) = identity(p);
                    at_least(version, prior_version) && (version != prior_version || *p == v)
                }
                None => true,
            }
        })
}

/// Pure ordering decision, not an atomic install or durable receipt.
/// Historical replay never renews the signed lease. Epoch transitions are
/// rejected even when individually pinned; recovery needs separate approval.
pub fn compare(prior: &Verified, next: &Verified) -> Result<Decision, SnapshotError> {
    let (a, b) = (&prior.payload, &next.payload);
    if a.organization_id != b.organization_id || a.vs_id != b.vs_id || a.epoch != b.epoch {
        return Err(SnapshotError::Replay);
    }
    let old = decimal(&a.sequence, u64::MAX).ok_or(SnapshotError::Replay)?;
    let new = decimal(&b.sequence, u64::MAX).ok_or(SnapshotError::Replay)?;
    if new < old || (new == old && prior.digest != next.digest) {
        return Err(SnapshotError::Replay);
    }
    if new == old {
        return Ok(Decision::HistoricalReplay);
    }
    if decimal(&b.issued_at, i64::MAX as u64) < decimal(&a.issued_at, i64::MAX as u64) {
        return Err(SnapshotError::Replay);
    }
    let tombstones: HashMap<_, _> = b
        .tombstones
        .iter()
        .map(|t| (t.binding_id.as_str(), t.version.as_str()))
        .collect();
    let active: HashMap<_, _> = b
        .bindings
        .iter()
        .map(|t| (t.binding_id.as_str(), t.version.as_str()))
        .collect();
    for t in &a.tombstones {
        if active.contains_key(t.binding_id.as_str())
            || !at_least(
                tombstones.get(t.binding_id.as_str()).copied().unwrap_or(""),
                &t.version,
            )
        {
            return Err(SnapshotError::Replay);
        }
    }
    for link in &a.bindings {
        if !active.contains_key(link.binding_id.as_str())
            && !at_least(
                tombstones
                    .get(link.binding_id.as_str())
                    .copied()
                    .unwrap_or(""),
                &link.version,
            )
        {
            return Err(SnapshotError::Replay);
        }
    }
    if !records_advance(&a.authorities, &b.authorities, true, |a| {
        (&a.authority_id, &a.version)
    }) || !records_advance(&a.sources, &b.sources, true, |s| (&s.source_id, &s.version))
        || !records_advance(&a.bindings, &b.bindings, false, |b| {
            (&b.binding_id, &b.version)
        })
    {
        return Err(SnapshotError::Replay);
    }
    Ok(Decision::Advance)
}

#[cfg(test)]
mod tests;
