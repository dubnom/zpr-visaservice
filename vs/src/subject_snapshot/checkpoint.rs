//! Single-writer offline checkpoint on a caller-protected local filesystem.

use std::collections::HashMap;
use std::fs::{self, DirBuilder, File, OpenOptions};
use std::io::{Read, Write};
use std::os::unix::fs::{DirBuilderExt, OpenOptionsExt, PermissionsExt};
use std::path::{Path, PathBuf};

use serde::{Deserialize, Serialize};
use thiserror::Error;

use super::{Decision, MAX_ENVELOPE, Scope, SnapshotError, Verified, compare, decimal, verify};

const MAX_STATE: usize = 4 * MAX_ENVELOPE + 4096;

#[derive(Debug, Error)]
pub enum CheckpointError {
    #[error("snapshot checkpoint I/O failure: {0}")]
    Io(#[from] std::io::Error),
    #[error("snapshot checkpoint validation failed: {0}")]
    Snapshot(#[from] SnapshotError),
    #[error("invalid, missing, or insecure snapshot checkpoint; explicit recovery required")]
    Recovery,
    #[error("snapshot checkpoint has an uncertain write outcome; close and reopen it")]
    Uncertain,
}

#[derive(Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
struct State {
    version: u8,
    organization: String,
    recipient: String,
    epoch: String,
    sequence: String,
    issued_at: String,
    expires_at: String,
    digest: String,
    envelope: String,
}

impl State {
    fn from_verified(envelope: &[u8], v: &Verified) -> Result<Self, CheckpointError> {
        Ok(Self {
            version: 1,
            organization: v.payload.organization_id.clone(),
            recipient: v.payload.vs_id.clone(),
            epoch: v.payload.epoch.clone(),
            sequence: v.payload.sequence.clone(),
            issued_at: v.payload.issued_at.clone(),
            expires_at: v.payload.expires_at.clone(),
            digest: v.digest.clone(),
            envelope: std::str::from_utf8(envelope)
                .map_err(|_| CheckpointError::Recovery)?
                .to_owned(),
        })
    }

    fn authenticate(
        &self,
        pins: &HashMap<String, Vec<u8>>,
        scope: &Scope<'_>,
    ) -> Result<Verified, CheckpointError> {
        // Authenticate at the signed issue instant to retain an expired ordering
        // floor. This is not current-time authorization or a renewed lease.
        let issued = decimal(&self.issued_at, i64::MAX as u64).ok_or(CheckpointError::Recovery)?;
        let verified = verify(self.envelope.as_bytes(), pins, scope, issued)?;
        let expected = Self::from_verified(self.envelope.as_bytes(), &verified)?;
        if self.version != 1
            || self.organization != expected.organization
            || self.recipient != expected.recipient
            || self.epoch != expected.epoch
            || self.sequence != expected.sequence
            || self.issued_at != expected.issued_at
            || self.expires_at != expected.expires_at
            || self.digest != expected.digest
        {
            return Err(CheckpointError::Recovery);
        }
        Ok(verified)
    }
}

/// Owns a process-held lock for its lifetime. This is not an adversarial-storage
/// rollback detector: protecting the directory and backups remains the caller's
/// responsibility. No open operation creates or resets missing state.
pub struct Checkpoint {
    directory: PathBuf,
    _lock: WriterLock,
    pins: HashMap<String, Vec<u8>>,
    state: State,
    verified: Verified,
    uncertain: bool,
}

fn protected(path: &Path, directory: bool) -> Result<(), CheckpointError> {
    let metadata = fs::symlink_metadata(path)?;
    if metadata.file_type().is_symlink()
        || metadata.is_dir() != directory
        || (!directory && !metadata.is_file())
        || metadata.permissions().mode() & 0o077 != 0
    {
        return Err(CheckpointError::Recovery);
    }
    Ok(())
}

struct WriterLock(File);

impl Drop for WriterLock {
    fn drop(&mut self) {
        // Release explicitly: a concurrently spawned child may briefly inherit
        // the open file description before close-on-exec runs.
        if let Err(error) = self.0.unlock() {
            tracing::error!(%error, "failed to release offline snapshot checkpoint lock");
        }
    }
}

fn lock(directory: &Path, create: bool) -> Result<WriterLock, CheckpointError> {
    protected(directory, true)?;
    let path = directory.join("writer.lock");
    if !create {
        protected(&path, false)?;
    }
    let file = OpenOptions::new()
        .read(true)
        .write(true)
        .create_new(create)
        .mode(0o600)
        .open(&path)?;
    file.try_lock()
        .map_err(|error| CheckpointError::Io(std::io::Error::other(error.to_string())))?;
    Ok(WriterLock(file))
}

impl Checkpoint {
    /// Explicit first installation, never a fallback from `open`. The directory
    /// must not exist. A failed/crashed initialization leaves evidence in place
    /// and requires explicit operator recovery rather than automatic retry/reset.
    pub fn initialize(
        directory: &Path,
        pins: HashMap<String, Vec<u8>>,
        scope: &Scope<'_>,
        envelope: &[u8],
        now: u64,
    ) -> Result<Self, CheckpointError> {
        if !directory.is_absolute() {
            return Err(CheckpointError::Recovery);
        }
        let verified = verify(envelope, &pins, scope, now)?;
        let state = State::from_verified(envelope, &verified)?;
        let parent = directory.parent().ok_or(CheckpointError::Recovery)?;
        let parent_file = File::open(parent)?;
        DirBuilder::new().mode(0o700).create(directory)?;
        parent_file.sync_all()?;
        let writer = lock(directory, true)?;
        let mut store = Self {
            directory: directory.to_owned(),
            _lock: writer,
            pins,
            state,
            verified,
            uncertain: true,
        };
        store.write(&store.state, |_| Ok(()))?;
        store.uncertain = false;
        Ok(store)
    }

    /// Reauthenticates the exact stored envelope under independently supplied
    /// pins and scope, even when its original lease has expired.
    pub fn open(
        directory: &Path,
        pins: HashMap<String, Vec<u8>>,
        scope: &Scope<'_>,
    ) -> Result<Self, CheckpointError> {
        if !directory.is_absolute() {
            return Err(CheckpointError::Recovery);
        }
        let writer = lock(directory, false)?;
        let path = directory.join("accepted.json");
        protected(&path, false)?;
        let mut bytes = Vec::new();
        File::open(path)?
            .take(MAX_STATE as u64 + 1)
            .read_to_end(&mut bytes)?;
        if bytes.len() > MAX_STATE {
            return Err(CheckpointError::Recovery);
        }
        let state: State = serde_json::from_slice(&bytes).map_err(|_| CheckpointError::Recovery)?;
        let verified = state.authenticate(&pins, scope)?;
        // Only a fully authenticated committed checkpoint authorizes discarding
        // an uncommitted temporary file. Never promote temporary state on reopen.
        let pending = directory.join("pending.json");
        match fs::symlink_metadata(&pending) {
            Ok(_) => {
                protected(&pending, false)?;
                fs::remove_file(pending)?;
            }
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => return Err(e.into()),
        }
        // Finish any rename whose directory sync was interrupted before replay
        // can report success from the recovered checkpoint.
        File::open(directory)?.sync_all()?;
        Ok(Self {
            directory: directory.to_owned(),
            _lock: writer,
            pins,
            state,
            verified,
            uncertain: false,
        })
    }

    fn scope(&self) -> Scope<'_> {
        Scope {
            organization: &self.state.organization,
            recipient: &self.state.recipient,
            epoch: &self.state.epoch,
        }
    }

    /// Current validity must be checked separately from restored ordering.
    pub fn current_at(&self, now: u64) -> Result<Verified, CheckpointError> {
        if self.uncertain {
            return Err(CheckpointError::Uncertain);
        }
        Ok(verify(
            self.state.envelope.as_bytes(),
            &self.pins,
            &self.scope(),
            now,
        )?)
    }

    pub fn install(&mut self, envelope: &[u8], now: u64) -> Result<Decision, CheckpointError> {
        self.install_with_hook(envelope, now, |_| Ok(()))
    }

    fn install_with_hook(
        &mut self,
        envelope: &[u8],
        now: u64,
        hook: impl FnMut(&str) -> std::io::Result<()>,
    ) -> Result<Decision, CheckpointError> {
        if self.uncertain {
            return Err(CheckpointError::Uncertain);
        }
        let next = verify(envelope, &self.pins, &self.scope(), now)?;
        let decision = compare(&self.verified, &next)?;
        if decision == Decision::HistoricalReplay {
            return Ok(decision);
        }
        let state = State::from_verified(envelope, &next)?;
        self.uncertain = true;
        self.write(&state, hook)?;
        self.state = state;
        self.verified = next;
        self.uncertain = false;
        Ok(decision)
    }

    fn write(
        &self,
        state: &State,
        mut hook: impl FnMut(&str) -> std::io::Result<()>,
    ) -> Result<(), CheckpointError> {
        let bytes = serde_json::to_vec(state).map_err(|_| CheckpointError::Recovery)?;
        if bytes.len() > MAX_STATE {
            return Err(CheckpointError::Recovery);
        }
        let pending = self.directory.join("pending.json");
        let mut file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .mode(0o600)
            .open(&pending)?;
        file.write_all(&bytes)?;
        hook("written")?;
        file.sync_all()?;
        hook("file-synced")?;
        fs::rename(&pending, self.directory.join("accepted.json"))?;
        hook("renamed")?;
        File::open(&self.directory)?.sync_all()?;
        hook("directory-synced")?;
        Ok(())
    }
}

#[cfg(test)]
mod tests;
