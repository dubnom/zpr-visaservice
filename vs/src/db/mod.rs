//! General database functions.

mod actor;
mod db_fake;
mod db_redis;
mod link;
mod node;
mod policy;
mod visa;

pub use actor::{ActorRepo, Role, ServiceEntry};
pub use db_redis::RedisDb;
pub use link::LinkRepo;
pub use node::{Node, NodeRepo};
pub use policy::{
    PolicyOperationReceipt, PolicyOperationRequest, PolicyOperationStatus, PolicyRepo,
};
pub use visa::{NodeVisaState, VisaMetadata, VisaRepo};

#[cfg(test)]
pub use db_fake::{FakeDb, FaultMode};

use chrono::Utc;
use percent_encoding::CONTROLS;
use percent_encoding::{AsciiSet, percent_decode_str, utf8_percent_encode};
use std::collections::{HashMap, HashSet};
use std::hash::{Hash, Hasher};
use std::net::IpAddr;
use std::time::Duration;

// Encode '%' and ':' for KeyString type strings.
const KEY_ESCAPES: &AsciiSet = &CONTROLS.add(b'%').add(b':');

pub enum LockType {
    VsInstance,
}

pub type DbResult<T> = redis::RedisResult<T>;

pub struct HashGuard<'a> {
    pub key: &'a str,
    pub field: &'a str,
    pub expected: Option<&'a str>,
}

#[cfg(test)]
pub(crate) async fn assert_session_guard_contract(db: &dyn DbConnection) {
    let ops = [DbOp::SetBin {
        key: "session:result".into(),
        value: vec![0, 255],
    }];
    db.sadd("session:services", "directory").await.unwrap();
    let services = HashSet::from(["directory".into()]);
    for expected in [None, Some(""), Some("session-one")] {
        assert!(
            !db.atomic_pipeline_if_set_and_hash_match(
                "session:services",
                &services,
                Some(HashGuard {
                    key: "session:metadata",
                    field: "authentication",
                    expected,
                }),
                &ops,
            )
            .await
            .unwrap()
        );
        assert!(!db.exists("session:result").await.unwrap());
    }
    db.hset("session:metadata", "ctime", "original")
        .await
        .unwrap();
    let binary_safe = "session:\0'\\\n";
    for (stored, expected, matches) in [
        (None, None, true),
        (None, Some(""), false),
        (Some(""), None, false),
        (Some(""), Some(""), true),
        (Some(binary_safe), Some("old"), false),
        (Some(binary_safe), Some(binary_safe), true),
    ] {
        if let Some(value) = stored {
            db.hset("session:metadata", "authentication", value)
                .await
                .unwrap();
        }
        db.del("session:result").await.unwrap();
        assert_eq!(
            db.atomic_pipeline_if_set_and_hash_match(
                "session:services",
                &services,
                Some(HashGuard {
                    key: "session:metadata",
                    field: "authentication",
                    expected,
                }),
                &ops,
            )
            .await
            .unwrap(),
            matches
        );
        assert_eq!(db.exists("session:result").await.unwrap(), matches);
    }
    db.del("session:result").await.unwrap();
    assert!(
        !db.atomic_pipeline_if_set_and_hash_match(
            "session:services",
            &HashSet::from(["wrong".into()]),
            Some(HashGuard {
                key: "session:metadata",
                field: "authentication",
                expected: Some(binary_safe),
            }),
            &ops,
        )
        .await
        .unwrap()
    );
    assert!(!db.exists("session:result").await.unwrap());
    db.del("session:metadata").await.unwrap();
    db.set("session:metadata", "wrong-type").await.unwrap();
    assert!(
        db.atomic_pipeline_if_set_and_hash_match(
            "session:services",
            &services,
            Some(HashGuard {
                key: "session:metadata",
                field: "authentication",
                expected: None,
            }),
            &ops,
        )
        .await
        .is_err()
    );
    assert!(!db.exists("session:result").await.unwrap());
}

pub struct PolicyCommit<'a> {
    pub operation_key: &'a str,
    pub current_key: &'a str,
    pub container_key: &'a str,
    pub expected_digest: &'a str,
    pub expected_vinst: &'a str,
    pub candidate_digest: &'a str,
    pub candidate_bytes: &'a [u8],
    pub next_vinst: &'a str,
    pub changed_at: &'a str,
    pub pending_receipt: &'a str,
}

// TODO: At some point remove this dead code and remove the unused functions.
#[allow(dead_code)]
#[async_trait::async_trait]
pub trait DbConnection: Send + Sync {
    /// May be called during a clean shutdown so that DB can do any necessary cleanup (e.g. releasing locks).
    async fn shutdown_cleanup(&self) -> DbResult<()>;

    async fn exists(&self, key: &str) -> DbResult<bool>;
    async fn set(&self, key: &str, value: &str) -> DbResult<()>;
    async fn set_ex(&self, key: &str, value: &str, seconds: u64) -> DbResult<()>;
    async fn get(&self, key: &str) -> DbResult<Option<String>>;

    /// Set a binary value directly. For atomic writes alongside other ops, use
    /// `DbOp::SetBin` in an `atomic_pipeline` instead.
    async fn set_bin(&self, key: &str, value: &[u8]) -> DbResult<()>;
    async fn set_bin_ex(&self, key: &str, value: &[u8], seconds: u64) -> DbResult<()>;
    async fn get_bin(&self, key: &str) -> DbResult<Vec<u8>>;
    async fn del(&self, key: &str) -> DbResult<()>;
    async fn smembers(&self, key: &str) -> DbResult<HashSet<String>>;
    async fn sismember(&self, key: &str, member: &str) -> DbResult<bool>;
    async fn hget(&self, key: &str, field: &str) -> DbResult<Option<String>>;
    async fn hgetall(&self, key: String) -> DbResult<HashMap<String, String>>;
    /// Read both hashes without interleaving writes. Missing hashes are empty;
    /// wrong Redis types and transport failures propagate as errors.
    async fn hash_pair_snapshot(
        &self,
        first: &str,
        second: &str,
    ) -> DbResult<(HashMap<String, String>, HashMap<String, String>)>;
    async fn hset(&self, key: &str, field: &str, value: &str) -> DbResult<()>;
    async fn hset_nx(&self, key: &str, field: &str, value: &str) -> DbResult<()>;
    async fn hset_multiple(&self, key: &str, field_values: &[(&str, &str)]) -> DbResult<()>;
    async fn sadd(&self, key: &str, member: &str) -> DbResult<()>;
    async fn incr(&self, key: &str, by: u64) -> DbResult<u64>;
    async fn expire(&self, key: &str, seconds: i64) -> DbResult<()>;

    /// Execute without interleaving using Redis MULTI/EXEC. Empty batches are
    /// no-ops. Queue-time errors abort the batch, but execution-time errors do
    /// not roll back other commands; a transport error may hide an applied batch.
    async fn atomic_pipeline(&self, ops: &[DbOp]) -> DbResult<()>;

    /// Compare the exact membership of `key` and execute the batch without
    /// interleaving. False means conflict and no mutation. The Redis Lua path
    /// stops at its first execution error without rolling back prior commands;
    /// transport errors remain ambiguous. This guards a set, not actor identity.
    async fn atomic_pipeline_if_set_matches(
        &self,
        key: &str,
        expected: &HashSet<String>,
        ops: &[DbOp],
    ) -> DbResult<bool> {
        self.atomic_pipeline_if_set_and_hash_match(key, expected, None, ops)
            .await
    }

    /// Additionally require an existing hash and an exact field value (or
    /// absence). All comparisons precede mutations under the same atomic boundary.
    async fn atomic_pipeline_if_set_and_hash_match(
        &self,
        key: &str,
        expected: &HashSet<String>,
        hash_guard: Option<HashGuard<'_>>,
        ops: &[DbOp],
    ) -> DbResult<bool>;

    /// Atomically create a pending policy operation only if the operation ID is
    /// unused and the persisted current policy matches the expected digest and
    /// vinst. Returns 0 for created, 1 for an identical replay, -1 for an
    /// operation-ID conflict, and -2 for a stale expected state.
    async fn begin_policy_operation(
        &self,
        operation_key: &str,
        current_key: &str,
        expected_digest: &str,
        expected_vinst: &str,
        pending_receipt: &str,
    ) -> DbResult<i64>;

    /// Atomically compare-and-swap the current policy and persist the still
    /// pending operation receipt. The caller must independently verify
    /// current-state readback before recording terminal success. Returns 0
    /// for applied, 1 if the candidate state is already present, -1 for a
    /// missing/mismatched operation, -2 for a stale current state, and -3 for
    /// a terminal non-success operation.
    async fn commit_policy_operation(&self, commit: PolicyCommit<'_>) -> DbResult<i64>;

    /// Compare the exact stored receipt before a legal transition. Success
    /// also requires an atomic current digest/version match. Returns 0 when
    /// updated, 1 for an identical terminal replay, -1 when absent, -2 when
    /// current state mismatches, -3 for an illegal transition, and -4 when
    /// the receipt changed concurrently.
    async fn finish_policy_operation(
        &self,
        operation_key: &str,
        current_key: &str,
        expected_receipt: &str,
        receipt: &str,
    ) -> DbResult<i64>;

    /// Consume a fresh proof nonce and bind its trusted identity/action to an
    /// operation atomically. Returns 0 when accepted, -1 for nonce reuse, and
    /// -2 when the operation already has a different approval binding.
    async fn authorize_policy_operation(
        &self,
        nonce_key: &str,
        approval_key: &str,
        approval: &str,
    ) -> DbResult<i64>;

    async fn scan_match_all(&self, pattern: String) -> DbResult<Vec<String>>;

    /// Remove all data from the database.
    async fn clear_state(&self) -> DbResult<()>;

    /// Returns TRUE if we were able to acquire or re-acquire a lock specified by the descriptor.
    /// Whent the lock is acquired or renewed the TTL will be extended by the timeout specified in the descriptor.
    async fn acquire_or_renew_lock(&self, desc: &LockDescriptor) -> DbResult<bool>;

    /// Returns TRUE if we were able to release a lock specified by the descriptor, or if no lock exists.
    /// Returns FALSE only if the lock exists but is not "ours".
    #[allow(dead_code)]
    async fn release_lock(&self, desc: &LockDescriptor) -> DbResult<bool>;
}

#[derive(Clone)]
pub struct LockDescriptor {
    key: String,
    ident: String,
    timeout: Duration,
}

impl LockDescriptor {
    /// Create LockDescriptor.
    ///
    /// The `ident` is a string that identifies the owner of the lock.  It can be any string,
    /// but should be unique to the owner (e.g. a UUID or hostname).
    ///
    /// The `timeout` is how long the lock should be held before it expires and can be acquired
    /// by someone else.
    pub fn new(ltype: LockType, ident: String, timeout: Duration) -> Self {
        let key = match ltype {
            LockType::VsInstance => format!("lock:vsinstance"),
        };
        LockDescriptor {
            key,
            ident,
            timeout,
        }
    }
}

impl Hash for LockDescriptor {
    fn hash<H: Hasher>(&self, state: &mut H) {
        self.key.hash(state);
        self.ident.hash(state);
    }
}

impl PartialEq for LockDescriptor {
    fn eq(&self, other: &Self) -> bool {
        self.key == other.key && self.ident == other.ident
    }
}

impl Eq for LockDescriptor {}

#[allow(dead_code)]
pub enum DbOp {
    Del(String),
    /// Set a binary value at `key`. Lets binary blobs join an atomic pipeline.
    SetBin {
        key: String,
        value: Vec<u8>,
    },
    SRem {
        set_key: String,
        member: String,
    },
    SAdd {
        set_key: String,
        member: String,
    },
    HSet {
        hash_key: String,
        field: String,
        value: String,
    },
    HSetMultiple {
        hash_key: String,
        field_values: Vec<(String, String)>,
    },
    Expire {
        key: String,
        seconds: i64,
    },
}

impl DbOp {
    fn command_parts(&self) -> (&'static str, Vec<Vec<u8>>) {
        match self {
            Self::Del(key) => ("DEL", vec![key.as_bytes().to_vec()]),
            Self::SetBin { key, value } => ("SET", vec![key.as_bytes().to_vec(), value.clone()]),
            Self::SRem { set_key, member } => (
                "SREM",
                vec![set_key.as_bytes().to_vec(), member.as_bytes().to_vec()],
            ),
            Self::SAdd { set_key, member } => (
                "SADD",
                vec![set_key.as_bytes().to_vec(), member.as_bytes().to_vec()],
            ),
            Self::HSet {
                hash_key,
                field,
                value,
            } => (
                "HSET",
                vec![
                    hash_key.as_bytes().to_vec(),
                    field.as_bytes().to_vec(),
                    value.as_bytes().to_vec(),
                ],
            ),
            Self::HSetMultiple {
                hash_key,
                field_values,
            } => {
                let mut args = vec![hash_key.as_bytes().to_vec()];
                for (field, value) in field_values {
                    args.push(field.as_bytes().to_vec());
                    args.push(value.as_bytes().to_vec());
                }
                ("HSET", args)
            }
            Self::Expire { key, seconds } => (
                "EXPIRE",
                vec![key.as_bytes().to_vec(), seconds.to_string().into_bytes()],
            ),
        }
    }
}

/// In the redis, we sometimes need to use a ZPR address as part of a key.
/// In that case we use this `ZAddr` type instead.  Only thing it it does
/// is replace colons in IPv6 addresses with dashes.
struct ZAddr(String);

/// In redis if we want to use user supllied text as part of a key we
/// want to remove any colons in there so we use percent encoding.
/// All colons and '%' in the original are encoded.
struct KeyString(String);

impl From<&str> for KeyString {
    fn from(s: &str) -> Self {
        let encoded = utf8_percent_encode(s, KEY_ESCAPES).to_string();
        KeyString(encoded)
    }
}

impl TryFrom<KeyString> for String {
    type Error = std::str::Utf8Error;
    fn try_from(ks: KeyString) -> Result<Self, Self::Error> {
        let decoded = percent_decode_str(&ks.0)
            .decode_utf8()
            .map(|cow| cow.into_owned())?;
        Ok(decoded)
    }
}

impl KeyString {
    /// Create a KeyString from a raw (already encoded) string.
    /// Caller must ensure that the string is properly encoded.
    fn from_raw(s: String) -> Self {
        KeyString(s)
    }

    /// Get the string representation of this KeyString.
    pub fn as_str(&self) -> &str {
        &self.0
    }
}

impl From<IpAddr> for ZAddr {
    fn from(addr: IpAddr) -> Self {
        match addr {
            IpAddr::V4(a4) => ZAddr(a4.to_string()),
            IpAddr::V6(a6) => {
                let ipv6str = a6.to_string();
                ZAddr(ipv6str.replace(":", "-"))
            }
        }
    }
}

impl From<&IpAddr> for ZAddr {
    fn from(addr: &IpAddr) -> Self {
        match addr {
            IpAddr::V4(a4) => ZAddr(a4.to_string()),
            IpAddr::V6(a6) => {
                let ipv6str = a6.to_string();
                ZAddr(ipv6str.replace(":", "-"))
            }
        }
    }
}

impl TryFrom<ZAddr> for IpAddr {
    type Error = std::net::AddrParseError;
    fn try_from(zaddr: ZAddr) -> Result<Self, Self::Error> {
        if zaddr.0.contains('-') {
            // IPv6
            let ipv6str = zaddr.0.replace("-", ":");
            Ok(IpAddr::V6(ipv6str.parse()?))
        } else {
            // IPv4
            Ok(IpAddr::V4(zaddr.0.parse()?))
        }
    }
}

impl std::fmt::Display for ZAddr {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl std::fmt::Display for KeyString {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.0)
    }
}

impl ZAddr {
    /// If you already have a "zaddr" encoded string, use this to create a ZAddr.
    pub fn new_from_encoded(s: &str) -> Self {
        ZAddr(s.into())
    }
}

/// All database string timestamps look like this.
pub fn gen_timestamp() -> String {
    let now = Utc::now();
    now.to_rfc3339_opts(chrono::SecondsFormat::Secs, true)
}
