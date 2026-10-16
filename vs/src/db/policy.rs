//! Redis/ValKey operations related to policy.
//!
//! PHASH is a true content hash of the policy *container* artifact: the SHA-256
//! of the exact container bytes we received. It changes whenever the container
//! bytes change (recompiles, compiler metadata, signatures, or policy body).
//!
//! The container is the single source of truth; the inner decoded policy is
//! always re-derived from it. This updates:
//! - policies:<PHASH>:container maps to the raw Cap'n Proto `PolicyContainer` bytes.
//! - policy:current a hash which includes key 'phash' with the current policy PHASH.
//!

use libeval::pio;
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tracing::debug;
use zpr::policy_types::PolicyContainerBytes;

use crate::db;
use crate::db::{DbConnection, DbOp};
use crate::error::StoreError;
use crate::loaded_policy::LoadedPolicy;
use crate::logging::targets::DB;

const KEY_POLICIES: &str = "policies";
const KEY_POLICY: &str = "policy";

/// The identifiers of the current policy as persisted in `policy:current`:
/// the container content hash and its monotonic version instance number.
pub struct PolicyIdentifier {
    pub phash: String,
    pub vinst: u64,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[allow(dead_code)]
#[serde(rename_all = "snake_case")]
pub enum PolicyOperationStatus {
    Pending,
    Succeeded,
    Failed,
    Uncertain,
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[allow(dead_code)]
#[serde(deny_unknown_fields)]
pub struct PolicyOperationRequest {
    pub operation_id: String,
    pub target_id: String,
    pub source_revision: u64,
    pub candidate_digest: String,
    pub expected_current_digest: String,
    pub expected_current_vinst: u64,
}

impl PolicyOperationRequest {
    pub(crate) fn validate(&self) -> Result<(), StoreError> {
        validate_operation_request(self)?;
        self.expected_current_vinst
            .checked_add(1)
            .ok_or_else(|| StoreError::InvalidData("policy vinst is exhausted".into()))?;
        Ok(())
    }
}

#[derive(Clone, Debug, Deserialize, Serialize, PartialEq, Eq)]
#[allow(dead_code)]
pub struct PolicyOperationReceipt {
    pub operation_id: String,
    pub target_id: String,
    pub source_revision: u64,
    pub candidate_digest: String,
    pub expected_current_digest: String,
    pub expected_current_vinst: u64,
    pub request_fingerprint: String,
    pub status: PolicyOperationStatus,
    pub applied_digest: Option<String>,
    pub applied_vinst: Option<u64>,
    pub error: Option<String>,
}

pub struct PolicyRepo {
    db: Arc<dyn DbConnection>,
}

impl PolicyRepo {
    pub fn new(db: Arc<dyn DbConnection>) -> Self {
        PolicyRepo { db }
    }

    /// Set the current policy information into the database. Unless
    /// `force_overwrite` is true, only updates the policy data in the database
    /// if the current policy is different (by its phash) from the one already
    /// stored.
    ///
    /// Only the container half of `loaded` is persisted; the decoded policy is
    /// always re-derived from it on load. Taking `&LoadedPolicy` makes it
    /// impossible to persist bytes that never decoded into a valid artifact.
    ///
    /// The vinst is always persisted, even when the container is unchanged:
    /// `update_policy_internal` bumps vinst while leaving identical container
    /// bytes, and that bump must reach the DB so vinst survives a restart.
    pub async fn set_current_policy(
        &self,
        loaded: &LoadedPolicy,
        force_overwrite: bool,
    ) -> Result<(), StoreError> {
        let container_bytes = loaded.container().as_bytes();
        let phash = loaded.hash_container_bytes()?;
        let vinst = loaded.policy().vinst().to_string();
        let maybe_curhash: Option<String> = self.db.hget("policy:current", "phash").await?;
        let curhash = maybe_curhash.unwrap_or_default();
        let container_key = format!("{KEY_POLICIES}:{phash}:container");
        let key_current = format!("{KEY_POLICY}:current");

        // Require the container blob to exist, so a half-written state (current
        // pointer present, container missing) still forces a rewrite.
        let exists: bool = (curhash == phash) && self.db.exists(&container_key).await?;
        if !exists || force_overwrite {
            debug!(target: DB, "updating current policy in DB to phash {phash}");

            //
            // policies:<PHASH>:container -> <capn proto container bytes>
            // policy:current
            //          |- phash -> the string <PHASH> value
            //          |- vinst -> the version instance number as a string
            //          |- ctime -> string
            //
            // All written in one atomic pipeline so the current pointer, its
            // container, and its vinst can never drift apart.
            //
            let ops = vec![
                DbOp::SetBin {
                    key: container_key,
                    value: container_bytes.to_vec(),
                },
                DbOp::HSet {
                    hash_key: key_current.clone(),
                    field: "phash".to_string(),
                    value: phash.clone(),
                },
                DbOp::HSet {
                    hash_key: key_current.clone(),
                    field: "vinst".to_string(),
                    value: vinst,
                },
                DbOp::HSet {
                    hash_key: key_current,
                    field: "ctime".to_string(),
                    value: db::gen_timestamp(),
                },
            ];
            self.db.atomic_pipeline(&ops).await?;
        } else {
            // Container unchanged, but vinst may have advanced (an update that
            // reuses identical container bytes). Persist the vinst on its own so
            // the bump is not lost on restart; atomicity with the container is
            // irrelevant here since the container is unchanged.
            debug!(target: DB, "set_current_policy container unchanged, updating vinst, hash={phash}");
            self.db.hset(&key_current, "vinst", &vinst).await?;
        }

        Ok(())
    }

    /// Read the current-policy identifier (phash + vinst) from
    /// `policy:current`.
    ///
    /// Returns `None` when no current policy is set (fresh DB). A present phash
    /// is always written alongside its vinst, so a phash with a missing or
    /// unparsable vinst is a corrupt state and an `InvalidData` error. Both
    /// fields are read together so startup never mixes a phash and vinst from
    /// separate writes.
    pub async fn get_current_identifier(&self) -> Result<Option<PolicyIdentifier>, StoreError> {
        let fields = self.db.hgetall("policy:current".into()).await?;
        if fields.is_empty() {
            return Ok(None);
        }
        let phash = fields
            .get("phash")
            .ok_or_else(|| StoreError::InvalidData("current policy has no phash field".into()))?;
        if phash.len() != 64
            || !phash
                .bytes()
                .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
        {
            return Err(StoreError::InvalidData(
                "invalid current policy digest".into(),
            ));
        }
        let vinst_str = fields.get("vinst").ok_or_else(|| {
            StoreError::InvalidData(format!("current policy {phash} has no vinst field"))
        })?;
        let vinst = vinst_str
            .parse::<u64>()
            .map_err(|e| StoreError::InvalidData(format!("unparsable vinst {vinst_str:?}: {e}")))?;
        Ok(Some(PolicyIdentifier {
            phash: phash.clone(),
            vinst,
        }))
    }

    /// Durably reserve a target-local operation ID after comparing the current
    /// policy state. Identical retries receive the existing receipt.
    #[allow(dead_code)]
    pub async fn begin_operation(
        &self,
        request: &PolicyOperationRequest,
    ) -> Result<PolicyOperationReceipt, StoreError> {
        validate_operation_request(request)?;
        let request_fingerprint = operation_fingerprint(request)?;
        let pending = PolicyOperationReceipt {
            operation_id: request.operation_id.clone(),
            target_id: request.target_id.clone(),
            source_revision: request.source_revision,
            candidate_digest: request.candidate_digest.clone(),
            expected_current_digest: request.expected_current_digest.clone(),
            expected_current_vinst: request.expected_current_vinst,
            request_fingerprint,
            status: PolicyOperationStatus::Pending,
            applied_digest: None,
            applied_vinst: None,
            error: None,
        };
        let serialized = serde_json::to_string(&pending)?;
        let result = self
            .db
            .begin_policy_operation(
                &operation_key(&request.operation_id),
                "policy:current",
                &request.expected_current_digest,
                &request.expected_current_vinst.to_string(),
                &serialized,
            )
            .await?;
        match result {
            0 | 1 => self.get_operation(&request.operation_id).await,
            -1 => Err(StoreError::InvalidData(
                "policy operation ID was already used for a different request".into(),
            )),
            -2 => Err(StoreError::InvalidData(
                "current policy changed; reload before applying this operation".into(),
            )),
            _ => Err(StoreError::InvalidData(format!(
                "unexpected policy operation reservation result {result}"
            ))),
        }
    }

    /// Atomically persist the candidate and current pointer with its pending
    /// receipt. The caller records success only after independent readback.
    #[allow(dead_code)]
    pub async fn commit_operation(
        &self,
        request: &PolicyOperationRequest,
        candidate_bytes: &[u8],
    ) -> Result<PolicyOperationReceipt, StoreError> {
        validate_operation_request(request)?;
        let candidate_digest = hex::encode(openssl::sha::sha256(candidate_bytes));
        if candidate_digest != request.candidate_digest {
            return Err(StoreError::InvalidData(
                "candidate bytes do not match the requested digest".into(),
            ));
        }
        let next_vinst = request
            .expected_current_vinst
            .checked_add(1)
            .ok_or_else(|| StoreError::InvalidData("policy vinst is exhausted".into()))?;
        let request_fingerprint = operation_fingerprint(request)?;
        let applied = PolicyOperationReceipt {
            operation_id: request.operation_id.clone(),
            target_id: request.target_id.clone(),
            source_revision: request.source_revision,
            candidate_digest: request.candidate_digest.clone(),
            expected_current_digest: request.expected_current_digest.clone(),
            expected_current_vinst: request.expected_current_vinst,
            request_fingerprint,
            status: PolicyOperationStatus::Pending,
            applied_digest: Some(request.candidate_digest.clone()),
            applied_vinst: Some(next_vinst),
            error: None,
        };
        let serialized = serde_json::to_string(&applied)?;
        let current_key = "policy:current";
        let expected_vinst = request.expected_current_vinst.to_string();
        let next_vinst = next_vinst.to_string();
        let container_key = format!("{KEY_POLICIES}:{}:container", request.candidate_digest);
        let changed_at = db::gen_timestamp();
        let result = self
            .db
            .commit_policy_operation(db::PolicyCommit {
                operation_key: &operation_key(&request.operation_id),
                current_key,
                container_key: &container_key,
                expected_digest: &request.expected_current_digest,
                expected_vinst: &expected_vinst,
                candidate_digest: &request.candidate_digest,
                candidate_bytes,
                next_vinst: &next_vinst,
                changed_at: &changed_at,
                pending_receipt: &serialized,
            })
            .await?;
        match result {
            0 | 1 => self.get_operation(&request.operation_id).await,
            -1 => Err(StoreError::InvalidData(
                "policy operation is missing or does not match the candidate".into(),
            )),
            -2 => Err(StoreError::InvalidData(
                "current policy changed before the candidate could be installed".into(),
            )),
            -3 => Err(StoreError::InvalidData(
                "policy operation is already in a terminal failure state".into(),
            )),
            _ => Err(StoreError::InvalidData(format!(
                "unexpected policy operation commit result {result}"
            ))),
        }
    }

    /// Persist a legal outcome without changing request identity or rewriting
    /// terminal receipts. Success additionally compares current state atomically;
    /// callers must reconcile on a conflict rather than assume finalization.
    #[allow(dead_code)]
    pub async fn finish_operation(
        &self,
        receipt: &PolicyOperationReceipt,
    ) -> Result<(), StoreError> {
        validate_operation_id(&receipt.operation_id)?;
        if receipt.status == PolicyOperationStatus::Pending {
            return Err(StoreError::InvalidData(
                "a policy operation cannot be finalized as pending".into(),
            ));
        }
        let key = operation_key(&receipt.operation_id);
        let existing = self.db.get(&key).await?.ok_or_else(|| {
            StoreError::NotFound(format!("policy operation {}", receipt.operation_id))
        })?;
        let result = self
            .db
            .finish_policy_operation(
                &key,
                "policy:current",
                &existing,
                &serde_json::to_string(receipt)?,
            )
            .await?;
        match result {
            0 | 1 => Ok(()),
            -1 => Err(StoreError::NotFound(format!(
                "policy operation {}",
                receipt.operation_id
            ))),
            -2 => Err(StoreError::InvalidData(
                "policy operation success does not match persisted current state".into(),
            )),
            -3 => Err(StoreError::InvalidData(
                "illegal policy operation receipt transition".into(),
            )),
            -4 => Err(StoreError::InvalidData(
                "policy operation receipt changed; reconcile before retrying".into(),
            )),
            _ => Err(StoreError::InvalidData(format!(
                "unexpected policy operation update result {result}"
            ))),
        }
    }

    /// Read the durable receipt by target-local operation ID.
    #[allow(dead_code)]
    pub async fn get_operation(
        &self,
        operation_id: &str,
    ) -> Result<PolicyOperationReceipt, StoreError> {
        validate_operation_id(operation_id)?;
        let json = self
            .db
            .get(&operation_key(operation_id))
            .await?
            .ok_or_else(|| StoreError::NotFound(format!("policy operation {operation_id}")))?;
        Ok(serde_json::from_str(&json)?)
    }

    /// Load the current policy artifact and decode it into a [LoadedPolicy].
    ///
    /// Returns an error if there is no current policy set, or if the stored
    /// container bytes cannot be decoded into a valid policy.
    pub async fn get_current_loaded_policy(
        &self,
        min_version: &pio::Version,
    ) -> Result<LoadedPolicy, StoreError> {
        let maybe_curhash: Option<String> = self.db.hget("policy:current", "phash").await?;
        let curhash =
            maybe_curhash.ok_or_else(|| StoreError::NotFound("no current policy set".into()))?;
        let container_key = format!("{KEY_POLICIES}:{curhash}:container");
        let cbytes = self.db.get_bin(&container_key).await?;
        LoadedPolicy::from_container(PolicyContainerBytes::from(cbytes), min_version)
            .map_err(|e| StoreError::InvalidData(format!("failed to decode policy container: {e}")))
    }

    /// Verify the persisted current container after a commit. Re-reading the
    /// identifier after fetching the bundle prevents a concurrent policy
    /// change from being mistaken for a verified apply.
    #[allow(dead_code)]
    pub async fn current_matches(
        &self,
        expected_digest: &str,
        expected_vinst: u64,
    ) -> Result<bool, StoreError> {
        let Some(before) = self.get_current_identifier().await? else {
            return Ok(false);
        };
        if before.phash != expected_digest || before.vinst != expected_vinst {
            return Ok(false);
        }
        let container_key = format!("{KEY_POLICIES}:{expected_digest}:container");
        let container = self.db.get_bin(&container_key).await?;
        if hex::encode(openssl::sha::sha256(&container)) != expected_digest {
            return Ok(false);
        }
        let Some(after) = self.get_current_identifier().await? else {
            return Ok(false);
        };
        Ok(after.phash == expected_digest
            && after.vinst == expected_vinst
            && after.phash == before.phash
            && after.vinst == before.vinst)
    }
}

/// Shared by both backends before their atomic exact-receipt comparison.
/// Keep u64 validation in Rust rather than Lua's floating-point JSON numbers.
pub(super) fn validate_operation_transition(
    existing_json: &str,
    updated_json: &str,
) -> db::DbResult<Option<PolicyOperationReceipt>> {
    let decode = |json: &str| {
        serde_json::from_str::<PolicyOperationReceipt>(json).map_err(|_| {
            redis::RedisError::from((
                redis::ErrorKind::UnexpectedReturnType,
                "invalid policy operation receipt",
            ))
        })
    };
    let existing = decode(existing_json)?;
    let updated = decode(updated_json)?;
    if matches!(
        existing.status,
        PolicyOperationStatus::Succeeded | PolicyOperationStatus::Failed
    ) {
        return Ok((existing == updated).then_some(updated));
    }
    if updated.status == PolicyOperationStatus::Pending {
        return Ok(None);
    }
    let mut allowed = existing.clone();
    allowed.status = updated.status.clone();
    allowed.error = updated.error.clone();
    if updated.status == PolicyOperationStatus::Succeeded {
        let Some(next_vinst) = existing.expected_current_vinst.checked_add(1) else {
            return Ok(None);
        };
        allowed.applied_digest = Some(existing.candidate_digest.clone());
        allowed.applied_vinst = Some(next_vinst);
        if updated.error.is_some() {
            return Ok(None);
        }
    } else if updated
        .error
        .as_ref()
        .is_none_or(|error| error.trim().is_empty())
        || (updated.status == PolicyOperationStatus::Failed
            && (existing.applied_digest.is_some() || existing.applied_vinst.is_some()))
    {
        return Ok(None);
    }
    Ok((allowed == updated).then_some(updated))
}

#[allow(dead_code)]
fn operation_key(operation_id: &str) -> String {
    format!("policy:operations:{operation_id}")
}

#[allow(dead_code)]
fn validate_operation_id(operation_id: &str) -> Result<(), StoreError> {
    if operation_id.is_empty()
        || operation_id.len() > 128
        || !operation_id
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || byte == b'-' || byte == b'_')
    {
        return Err(StoreError::InvalidData(
            "policy operation ID must be 1-128 ASCII letters, digits, '-' or '_'".into(),
        ));
    }
    Ok(())
}

#[allow(dead_code)]
fn validate_digest(digest: &str, field: &str) -> Result<(), StoreError> {
    if digest.len() != 64
        || !digest
            .bytes()
            .all(|byte| byte.is_ascii_digit() || (b'a'..=b'f').contains(&byte))
    {
        return Err(StoreError::InvalidData(format!(
            "{field} must be a lowercase SHA-256 digest"
        )));
    }
    Ok(())
}

#[allow(dead_code)]
fn validate_operation_request(request: &PolicyOperationRequest) -> Result<(), StoreError> {
    validate_operation_id(&request.operation_id)?;
    if request.target_id.trim().is_empty() || request.target_id.len() > 256 {
        return Err(StoreError::InvalidData(
            "policy operation target ID is required and must be at most 256 bytes".into(),
        ));
    }
    if request.source_revision == 0 {
        return Err(StoreError::InvalidData(
            "policy source revision must be positive".into(),
        ));
    }
    validate_digest(&request.candidate_digest, "candidate digest")?;
    validate_digest(&request.expected_current_digest, "expected current digest")
}

#[allow(dead_code)]
fn operation_fingerprint(request: &PolicyOperationRequest) -> Result<String, StoreError> {
    let encoded = serde_json::to_vec(request)?;
    Ok(hex::encode(openssl::sha::sha256(&encoded)))
}

#[cfg(test)]
mod test {
    use super::*;
    use crate::config;
    use crate::db::db_fake::FakeDb;
    use zpr::policy::v1 as policy_capnp;

    /// Build a [LoadedPolicy] by encoding a minimal policy, wrapping it in a
    /// container at the minimum supported compiler version, and decoding it.
    fn make_loaded(created: &str, version: u64, metadata: Option<&str>) -> LoadedPolicy {
        let mut msg = capnp::message::Builder::new_default();
        {
            let mut policy_bldr = msg.init_root::<policy_capnp::policy::Builder>();
            policy_bldr.set_created(created);
            policy_bldr.set_version(version);
            policy_bldr.set_metadata(metadata.unwrap_or(""));
        }
        let mut inner = Vec::new();
        capnp::serialize::write_message(&mut inner, &msg).unwrap();
        let container = crate::test_helpers::make_container_bytes(
            config::POLICY_MIN_COMPILER_MAJOR,
            config::POLICY_MIN_COMPILER_MINOR,
            config::POLICY_MIN_COMPILER_PATCH,
            &inner,
        );
        LoadedPolicy::from_container(
            PolicyContainerBytes::from(container),
            &config::POLICY_MIN_VERSION,
        )
        .unwrap()
    }

    #[tokio::test]
    /// An initial write stores the container blob and points policy:current at it.
    async fn test_set_current_policy_initial_write() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let loaded = make_loaded("2024-01-01T00:00:00Z", 1, Some("meta"));

        repo.set_current_policy(&loaded, false).await.unwrap();

        let phash = db.hget("policy:current", "phash").await.unwrap().unwrap();
        let container_key = format!("{KEY_POLICIES}:{phash}:container");
        let stored = db.get_bin(&container_key).await.unwrap();
        assert_eq!(stored, loaded.container().as_bytes());
    }

    #[tokio::test]
    /// Re-writing the same artifact is a no-op for the write branch. Probed via a
    /// sentinel `ctime`: the write branch always rewrites `ctime`, so its survival
    /// proves the branch did not run.
    async fn test_set_current_policy_no_change() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let loaded = make_loaded("2024-01-01T00:00:00Z", 2, Some("meta"));

        repo.set_current_policy(&loaded, false).await.unwrap();
        db.hset("policy:current", "ctime", "SENTINEL")
            .await
            .unwrap();
        repo.set_current_policy(&loaded, false).await.unwrap();

        let ctime = db.hget("policy:current", "ctime").await.unwrap().unwrap();
        assert_eq!(ctime, "SENTINEL", "write branch must not run for no change");
    }

    #[tokio::test]
    /// force_overwrite rewrites even when the artifact is unchanged. Probed via a
    /// sentinel `ctime` the write branch overwrites.
    async fn test_set_current_policy_force_overwrite() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let loaded = make_loaded("2024-01-01T00:00:00Z", 3, Some("meta"));

        repo.set_current_policy(&loaded, false).await.unwrap();
        db.hset("policy:current", "ctime", "SENTINEL")
            .await
            .unwrap();
        repo.set_current_policy(&loaded, true).await.unwrap();

        let ctime = db.hget("policy:current", "ctime").await.unwrap().unwrap();
        assert_ne!(ctime, "SENTINEL", "force write branch must run");
    }

    #[tokio::test]
    /// A missing container blob forces a rewrite even though the current pointer
    /// still matches the phash.
    async fn test_set_current_policy_rewrites_when_container_missing() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let loaded = make_loaded("2024-01-01T00:00:00Z", 4, Some("meta"));

        repo.set_current_policy(&loaded, false).await.unwrap();

        let phash = db.hget("policy:current", "phash").await.unwrap().unwrap();
        let container_key = format!("{KEY_POLICIES}:{phash}:container");
        db.del(&container_key).await.unwrap();

        repo.set_current_policy(&loaded, false).await.unwrap();
        assert!(db.exists(&container_key).await.unwrap());
    }

    #[tokio::test]
    /// Changing only the container bytes changes the PHASH and stores a distinct
    /// artifact under a new key.
    async fn test_set_current_policy_distinct_phash_per_container() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());

        let loaded_a = make_loaded("2024-01-01T00:00:00Z", 1, Some("meta-a"));
        let loaded_b = make_loaded("2024-01-01T00:00:00Z", 1, Some("meta-b"));

        let phash_a = loaded_a.hash_container_bytes().unwrap();
        let phash_b = loaded_b.hash_container_bytes().unwrap();
        assert_ne!(phash_a, phash_b);

        repo.set_current_policy(&loaded_a, false).await.unwrap();
        repo.set_current_policy(&loaded_b, false).await.unwrap();

        // Both artifacts are stored under their own keys.
        assert!(
            db.exists(&format!("{KEY_POLICIES}:{phash_a}:container"))
                .await
                .unwrap()
        );
        assert!(
            db.exists(&format!("{KEY_POLICIES}:{phash_b}:container"))
                .await
                .unwrap()
        );
        // Current points at the most recent write.
        let cur = db.hget("policy:current", "phash").await.unwrap().unwrap();
        assert_eq!(cur, phash_b);
    }

    #[tokio::test]
    /// set_current_policy followed by get_current_loaded_policy round-trips
    /// through LoadedPolicy: same container bytes, same decoded policy version.
    async fn test_round_trip_through_loaded_policy() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db);
        let loaded = make_loaded("2024-01-01T00:00:00Z", 7, Some("meta"));

        repo.set_current_policy(&loaded, false).await.unwrap();

        let got = repo
            .get_current_loaded_policy(&config::POLICY_MIN_VERSION)
            .await
            .unwrap();
        assert_eq!(got.container().as_bytes(), loaded.container().as_bytes());
        assert_eq!(got.policy().get_version(), Some(7));
    }

    #[tokio::test]
    /// get_current_loaded_policy errors when no current policy is set.
    async fn test_get_current_loaded_policy_not_found() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db);
        let res = repo
            .get_current_loaded_policy(&config::POLICY_MIN_VERSION)
            .await;
        assert!(matches!(res, Err(StoreError::NotFound(_))));
    }

    #[tokio::test]
    /// The write branch persists the vinst; get_current_identifier round-trips
    /// both phash and vinst.
    async fn test_set_current_policy_persists_vinst() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db);
        let mut loaded = make_loaded("2024-01-01T00:00:00Z", 1, Some("meta"));
        loaded.set_vinst(5);

        repo.set_current_policy(&loaded, false).await.unwrap();

        let ident = repo.get_current_identifier().await.unwrap().unwrap();
        assert_eq!(ident.phash, loaded.hash_container_bytes().unwrap());
        assert_eq!(ident.vinst, 5);
    }

    #[tokio::test]
    /// A vinst bump on identical container bytes still reaches the DB even though
    /// the container write branch is skipped.
    async fn test_set_current_policy_updates_vinst_when_container_unchanged() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db);
        let mut loaded = make_loaded("2024-01-01T00:00:00Z", 1, Some("meta"));
        loaded.set_vinst(1);
        repo.set_current_policy(&loaded, false).await.unwrap();

        // Same container bytes, higher vinst: the no-change branch must still
        // persist the bumped vinst.
        loaded.set_vinst(2);
        repo.set_current_policy(&loaded, false).await.unwrap();

        let ident = repo.get_current_identifier().await.unwrap().unwrap();
        assert_eq!(ident.vinst, 2);
    }

    #[tokio::test]
    /// get_current_identifier returns None on a fresh DB.
    async fn test_get_current_identifier_none_on_fresh_db() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db);
        assert!(repo.get_current_identifier().await.unwrap().is_none());
    }

    #[tokio::test]
    /// A phash with no vinst field is a corrupt state, not a valid default: it is
    /// an InvalidData error, since a phash is always written alongside its vinst.
    async fn test_get_current_identifier_errors_on_missing_vinst() {
        let db = Arc::new(FakeDb::new());
        db.hset("policy:current", "phash", "deadbeef")
            .await
            .unwrap();
        let repo = PolicyRepo::new(db);

        let res = repo.get_current_identifier().await;
        assert!(matches!(res, Err(StoreError::InvalidData(_))));
    }

    #[tokio::test]
    /// An unparsable vinst is an InvalidData error, not a silent default.
    async fn test_get_current_identifier_errors_on_garbage_vinst() {
        let db = Arc::new(FakeDb::new());
        db.hset("policy:current", "phash", "deadbeef")
            .await
            .unwrap();
        db.hset("policy:current", "vinst", "not-a-number")
            .await
            .unwrap();
        let repo = PolicyRepo::new(db);

        let res = repo.get_current_identifier().await;
        assert!(matches!(res, Err(StoreError::InvalidData(_))));
    }

    fn operation_request(
        operation_id: &str,
        current: &LoadedPolicy,
        candidate: &LoadedPolicy,
    ) -> PolicyOperationRequest {
        PolicyOperationRequest {
            operation_id: operation_id.into(),
            target_id: "visa-service-fixture".into(),
            source_revision: 7,
            candidate_digest: candidate.hash_container_bytes().unwrap(),
            expected_current_digest: current.hash_container_bytes().unwrap(),
            expected_current_vinst: current.policy().vinst(),
        }
    }

    #[tokio::test]
    async fn test_policy_operation_reservation_is_idempotent_and_rejects_reuse() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let mut current = make_loaded("2024-01-01T00:00:00Z", 1, Some("current"));
        current.set_vinst(3);
        repo.set_current_policy(&current, false).await.unwrap();
        let candidate = make_loaded("2024-01-02T00:00:00Z", 2, Some("candidate"));
        let request = operation_request("operation-1", &current, &candidate);

        let first = repo.begin_operation(&request).await.unwrap();
        let replay = repo.begin_operation(&request).await.unwrap();
        assert_eq!(first.status, PolicyOperationStatus::Pending);
        assert_eq!(first, replay);

        let mut different_request = request.clone();
        different_request.source_revision += 1;
        assert!(matches!(
            repo.begin_operation(&different_request).await,
            Err(StoreError::InvalidData(_))
        ));

        let mut stale_request = request;
        stale_request.operation_id = "stale-operation".into();
        stale_request.expected_current_vinst = 2;
        assert!(matches!(
            repo.begin_operation(&stale_request).await,
            Err(StoreError::InvalidData(_))
        ));
        assert!(matches!(
            repo.get_operation("stale-operation").await,
            Err(StoreError::NotFound(_))
        ));
    }

    #[tokio::test]
    async fn test_policy_operation_commit_requires_readback_before_terminal_success() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let mut current = make_loaded("2024-01-01T00:00:00Z", 1, Some("current"));
        current.set_vinst(3);
        repo.set_current_policy(&current, false).await.unwrap();
        let candidate = make_loaded("2024-01-02T00:00:00Z", 2, Some("candidate"));
        let request = operation_request("operation-2", &current, &candidate);

        repo.begin_operation(&request).await.unwrap();
        let applied = repo
            .commit_operation(&request, candidate.container().as_bytes())
            .await
            .unwrap();
        assert_eq!(applied.status, PolicyOperationStatus::Pending);
        assert_eq!(
            applied.applied_digest.as_deref(),
            Some(request.candidate_digest.as_str())
        );
        assert!(
            repo.current_matches(&request.candidate_digest, 4)
                .await
                .unwrap()
        );

        let replay = repo
            .commit_operation(&request, candidate.container().as_bytes())
            .await
            .unwrap();
        assert_eq!(replay.status, PolicyOperationStatus::Pending);
        assert_eq!(
            repo.get_current_identifier().await.unwrap().unwrap().vinst,
            4
        );

        let mut succeeded = replay;
        succeeded.status = PolicyOperationStatus::Succeeded;
        repo.finish_operation(&succeeded).await.unwrap();
        let reconstructed_repo = PolicyRepo::new(db);
        assert_eq!(
            reconstructed_repo
                .get_operation("operation-2")
                .await
                .unwrap()
                .status,
            PolicyOperationStatus::Succeeded
        );
        assert!(
            !reconstructed_repo
                .current_matches(&request.expected_current_digest, 3)
                .await
                .unwrap()
        );
    }

    #[tokio::test]
    async fn test_policy_operation_success_requires_atomic_current_state_match() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let mut current = make_loaded("2024-01-01T00:00:00Z", 1, Some("current"));
        current.set_vinst(3);
        repo.set_current_policy(&current, false).await.unwrap();
        let candidate = make_loaded("2024-01-02T00:00:00Z", 2, Some("candidate"));
        let request = operation_request("guarded-success", &current, &candidate);
        let pending = repo.begin_operation(&request).await.unwrap();
        let mut succeeded = pending.clone();
        succeeded.status = PolicyOperationStatus::Succeeded;
        succeeded.applied_digest = Some(request.candidate_digest.clone());
        succeeded.applied_vinst = Some(4);

        assert!(repo.finish_operation(&succeeded).await.is_err());
        assert_eq!(
            repo.get_operation(&request.operation_id).await.unwrap(),
            pending
        );

        let applied = repo
            .commit_operation(&request, candidate.container().as_bytes())
            .await
            .unwrap();
        assert!(
            repo.current_matches(&request.candidate_digest, 4)
                .await
                .unwrap()
        );

        // Change current state after independent readback, before finalization.
        for (field, value) in [
            ("phash", request.expected_current_digest.clone()),
            ("vinst", "5".into()),
        ] {
            db.hset("policy:current", field, &value).await.unwrap();
            assert!(repo.finish_operation(&succeeded).await.is_err());
            assert_eq!(
                repo.get_operation(&request.operation_id).await.unwrap(),
                applied
            );
            db.hset("policy:current", "phash", &request.candidate_digest)
                .await
                .unwrap();
            db.hset("policy:current", "vinst", "4").await.unwrap();
        }

        let mut uncertain = applied;
        uncertain.status = PolicyOperationStatus::Uncertain;
        uncertain.error = Some("readback interrupted".into());
        repo.finish_operation(&uncertain).await.unwrap();
        db.hset("policy:current", "vinst", "5").await.unwrap();
        assert!(repo.finish_operation(&succeeded).await.is_err());
        assert_eq!(
            repo.get_operation(&request.operation_id).await.unwrap(),
            uncertain
        );
        db.hset("policy:current", "vinst", "4").await.unwrap();
        repo.finish_operation(&succeeded).await.unwrap();
        repo.finish_operation(&succeeded).await.unwrap();
        assert!(repo.finish_operation(&uncertain).await.is_err());
        assert_eq!(
            repo.get_operation(&request.operation_id).await.unwrap(),
            succeeded
        );
    }

    #[tokio::test]
    async fn test_policy_operation_finalizer_rejects_forged_fields_and_illegal_transitions() {
        let db = Arc::new(FakeDb::new());
        let repo = PolicyRepo::new(db.clone());
        let mut current = make_loaded("2024-01-01T00:00:00Z", 1, None);
        current.set_vinst(3);
        repo.set_current_policy(&current, false).await.unwrap();
        let candidate = make_loaded("2024-01-02T00:00:00Z", 2, None);
        let request = operation_request("guarded-transition", &current, &candidate);
        let pending = repo.begin_operation(&request).await.unwrap();
        let key = operation_key(&request.operation_id);
        let original_json = serde_json::to_string(&pending).unwrap();
        let mut failed = pending.clone();
        failed.status = PolicyOperationStatus::Failed;
        failed.error = Some("candidate validation failed".into());

        let mut forged = failed.clone();
        forged.source_revision += 1;
        assert_eq!(
            db.finish_policy_operation(
                &key,
                "policy:current",
                &original_json,
                &serde_json::to_string(&forged).unwrap()
            )
            .await
            .unwrap(),
            -3
        );
        let mut malformed_success = pending.clone();
        malformed_success.status = PolicyOperationStatus::Succeeded;
        assert!(repo.finish_operation(&malformed_success).await.is_err());
        malformed_success.applied_digest = Some(request.expected_current_digest.clone());
        malformed_success.applied_vinst = Some(3);
        assert!(repo.finish_operation(&malformed_success).await.is_err());
        assert_eq!(
            db.finish_policy_operation(&key, "policy:current", &original_json, &original_json)
                .await
                .unwrap(),
            -3
        );
        assert_eq!(
            repo.get_operation(&request.operation_id).await.unwrap(),
            pending
        );

        repo.finish_operation(&failed).await.unwrap();
        repo.finish_operation(&failed).await.unwrap();
        let mut uncertain = pending;
        uncertain.status = PolicyOperationStatus::Uncertain;
        uncertain.error = Some("outcome unknown".into());
        assert!(repo.finish_operation(&uncertain).await.is_err());
        assert_eq!(
            db.finish_policy_operation(
                &key,
                "policy:current",
                &original_json,
                &serde_json::to_string(&failed).unwrap()
            )
            .await
            .unwrap(),
            -4
        );
        assert_eq!(
            repo.get_operation(&request.operation_id).await.unwrap(),
            failed
        );
    }
}
