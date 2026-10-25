//! Pinned mutual-TLS pull of the current signed subject-link snapshot.
//!
//! Fetch, verify, durably checkpoint and report a receipt only. Nothing here
//! evaluates, enforces or otherwise consumes snapshot contents.

use std::collections::{HashMap, HashSet};
use std::io::Read;
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use base64::{Engine, engine::general_purpose::URL_SAFE_NO_PAD};
use serde::{Deserialize, Serialize};
use thiserror::Error;

use super::checkpoint::{Checkpoint, CheckpointError};
use super::{Decision, MAX_ENVELOPE, Scope, SnapshotError, Verified, id, verify};

pub const DELIVERY_PREFIX: &str = "/v1/subject-link-snapshots/";
const MAX_PINS_FILE: u64 = 65536;
const MAX_RECEIPT_RESPONSE: usize = 4096;

fn default_poll_seconds() -> u64 {
    60
}

/// `[subject_snapshot_fetch]` in the VS configuration. Absent disables fetching.
#[derive(Deserialize, Serialize, Debug, Clone)]
#[serde(deny_unknown_fields)]
pub struct FetchConfig {
    /// HTTPS origin of the Control-Service delivery listener.
    pub url: String,
    /// CA bundle that must have issued the delivery listener certificate.
    pub ca_cert: PathBuf,
    /// This VS's pinned client certificate and private key (PEM).
    pub client_cert: PathBuf,
    pub client_key: PathBuf,
    /// Expected signed scope. The server derives scope from the client pin; the
    /// VS independently requires these values in every accepted envelope.
    pub organization_id: String,
    pub vs_id: String,
    pub epoch: String,
    /// The same public signing pins file Control-Service uses
    /// (`{"version":1,"key_id":…,"pins":[{"key_id","public_key"}]}`).
    pub signing_pins_file: PathBuf,
    /// Absolute protected checkpoint directory. Created only by explicit initialization.
    pub checkpoint_dir: PathBuf,
    #[serde(default = "default_poll_seconds")]
    pub poll_seconds: u64,
}

#[derive(Debug, Error)]
pub enum FetchError {
    #[error("invalid subject snapshot fetch configuration: {0}")]
    Config(String),
    #[error("subject snapshot delivery transport failure: {0}")]
    Transport(String),
    #[error("subject snapshot delivery returned unexpected HTTP status {0}")]
    Status(u16),
    #[error("subject snapshot delivery has no current signed snapshot")]
    NoSnapshot,
    #[error("subject snapshot delivery response exceeds its size limit")]
    TooLarge,
    #[error("subject snapshot receipt was rejected or did not match the accepted snapshot")]
    Receipt,
    #[error(transparent)]
    Snapshot(#[from] SnapshotError),
    #[error(transparent)]
    Checkpoint(#[from] CheckpointError),
}

#[derive(Debug, PartialEq)]
pub enum SyncOutcome {
    /// The delivery listener withheld the snapshot; the checkpoint is unchanged.
    Withheld,
    /// A newer snapshot was durably installed and its receipt reported.
    Advanced { sequence: String, digest: String },
    /// The already accepted snapshot was delivered again.
    Unchanged { sequence: String, digest: String },
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct PinsFile {
    version: u8,
    key_id: String,
    pins: Vec<Pin>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Pin {
    key_id: String,
    public_key: String,
}

pub fn parse_signing_pins(bytes: &[u8]) -> Result<HashMap<String, Vec<u8>>, FetchError> {
    let invalid = || {
        FetchError::Config(
            "signing pins require version 1, an active key_id and 1-64 unique Ed25519 pins".into(),
        )
    };
    let file: PinsFile = serde_json::from_slice(bytes).map_err(|_| invalid())?;
    if file.version != 1 || !id(&file.key_id) || file.pins.is_empty() || file.pins.len() > 64 {
        return Err(invalid());
    }
    let mut pins = HashMap::new();
    let mut keys = HashSet::new();
    for pin in file.pins {
        let key = URL_SAFE_NO_PAD
            .decode(&pin.public_key)
            .map_err(|_| invalid())?;
        if !id(&pin.key_id)
            || key.len() != 32
            || URL_SAFE_NO_PAD.encode(&key) != pin.public_key
            || !keys.insert(key.clone())
            || pins.insert(pin.key_id, key).is_some()
        {
            return Err(invalid());
        }
    }
    if !pins.contains_key(&file.key_id) {
        return Err(invalid());
    }
    Ok(pins)
}

fn read_limited(path: &Path, limit: u64, purpose: &str) -> Result<Vec<u8>, FetchError> {
    if !path.is_absolute() {
        return Err(FetchError::Config(format!(
            "{purpose} path must be absolute"
        )));
    }
    let mut bytes = Vec::new();
    std::fs::File::open(path)
        .and_then(|file| file.take(limit + 1).read_to_end(&mut bytes))
        .map_err(|e| FetchError::Config(format!("reading {purpose} {path:?}: {e}")))?;
    if bytes.len() as u64 > limit {
        return Err(FetchError::Config(format!("{purpose} is too large")));
    }
    Ok(bytes)
}

fn unix_now() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

#[derive(Serialize)]
struct ReceiptRequest<'a> {
    sequence: &'a str,
    digest: &'a str,
}

#[derive(Deserialize)]
struct ReceiptScope {
    organization_id: String,
    vs_id: String,
    epoch: String,
}

#[derive(Deserialize)]
struct ReceiptResponse {
    scope: ReceiptScope,
    sequence: String,
    digest: String,
}

pub struct Fetcher {
    client: reqwest::Client,
    current: reqwest::Url,
    receipts: reqwest::Url,
    organization: String,
    recipient: String,
    epoch: String,
    pins: HashMap<String, Vec<u8>>,
    checkpoint_dir: PathBuf,
    poll: Duration,
}

impl Fetcher {
    pub fn new(config: &FetchConfig) -> Result<Self, FetchError> {
        let url = reqwest::Url::parse(&config.url)
            .map_err(|e| FetchError::Config(format!("invalid url: {e}")))?;
        if url.scheme() != "https"
            || url.host_str().is_none()
            || !url.username().is_empty()
            || url.password().is_some()
            || url.query().is_some()
            || url.fragment().is_some()
            || url.path() != "/"
        {
            return Err(FetchError::Config("url must be an https origin".into()));
        }
        if !id(&config.organization_id) || !id(&config.vs_id) || !id(&config.epoch) {
            return Err(FetchError::Config(
                "organization_id, vs_id and epoch must be 1-128 letters, digits, '_' or '-'".into(),
            ));
        }
        if !config.checkpoint_dir.is_absolute() {
            return Err(FetchError::Config("checkpoint_dir must be absolute".into()));
        }
        if !(10..=3600).contains(&config.poll_seconds) {
            return Err(FetchError::Config("poll_seconds must be 10-3600".into()));
        }
        let pins = parse_signing_pins(&read_limited(
            &config.signing_pins_file,
            MAX_PINS_FILE,
            "signing pins file",
        )?)?;
        let ca = reqwest::Certificate::from_pem_bundle(&read_limited(
            &config.ca_cert,
            MAX_PINS_FILE,
            "CA file",
        )?)
        .map_err(|e| FetchError::Config(format!("invalid CA: {e}")))?;
        if ca.is_empty() {
            return Err(FetchError::Config(
                "CA file contains no certificates".into(),
            ));
        }
        let mut identity = read_limited(&config.client_cert, MAX_PINS_FILE, "client certificate")?;
        identity.extend_from_slice(&read_limited(
            &config.client_key,
            MAX_PINS_FILE,
            "client private key",
        )?);
        let identity = reqwest::Identity::from_pem(&identity)
            .map_err(|e| FetchError::Config(format!("invalid client identity: {e}")))?;
        let client = reqwest::Client::builder()
            .tls_certs_only(ca)
            .identity(identity)
            .redirect(reqwest::redirect::Policy::none())
            .timeout(Duration::from_secs(15))
            .build()
            .map_err(|e| FetchError::Config(format!("HTTPS client: {e}")))?;
        let base = format!("{DELIVERY_PREFIX}{}/", config.epoch);
        let join = |tail: &str| {
            url.join(&format!("{base}{tail}"))
                .map_err(|e| FetchError::Config(e.to_string()))
        };
        Ok(Self {
            client,
            current: join("current")?,
            receipts: join("receipts")?,
            organization: config.organization_id.clone(),
            recipient: config.vs_id.clone(),
            epoch: config.epoch.clone(),
            pins,
            checkpoint_dir: config.checkpoint_dir.clone(),
            poll: Duration::from_secs(config.poll_seconds),
        })
    }

    fn scope(&self) -> Scope<'_> {
        Scope {
            organization: &self.organization,
            recipient: &self.recipient,
            epoch: &self.epoch,
        }
    }

    async fn fetch(&self) -> Result<Option<Vec<u8>>, FetchError> {
        let mut response = self
            .client
            .get(self.current.clone())
            .send()
            .await
            .map_err(|e| FetchError::Transport(e.to_string()))?;
        match response.status().as_u16() {
            200 => {}
            404 => return Ok(None),
            status => return Err(FetchError::Status(status)),
        }
        if response.content_length().unwrap_or(0) > MAX_ENVELOPE as u64 {
            return Err(FetchError::TooLarge);
        }
        let mut body = Vec::new();
        while let Some(chunk) = response
            .chunk()
            .await
            .map_err(|e| FetchError::Transport(e.to_string()))?
        {
            if body.len() + chunk.len() > MAX_ENVELOPE {
                return Err(FetchError::TooLarge);
            }
            body.extend_from_slice(&chunk);
        }
        Ok(Some(body))
    }

    async fn report(&self, verified: &Verified) -> Result<(), FetchError> {
        let sequence = &verified.payload.sequence;
        let digest = verified.digest();
        let mut response = self
            .client
            .post(self.receipts.clone())
            .json(&ReceiptRequest { sequence, digest })
            .send()
            .await
            .map_err(|e| FetchError::Transport(e.to_string()))?;
        if response.status().as_u16() != 200 {
            return Err(match response.status().as_u16() {
                409 => FetchError::Receipt,
                status => FetchError::Status(status),
            });
        }
        let mut body = Vec::new();
        while let Some(chunk) = response
            .chunk()
            .await
            .map_err(|e| FetchError::Transport(e.to_string()))?
        {
            if body.len() + chunk.len() > MAX_RECEIPT_RESPONSE {
                return Err(FetchError::TooLarge);
            }
            body.extend_from_slice(&chunk);
        }
        let receipt: ReceiptResponse =
            serde_json::from_slice(&body).map_err(|_| FetchError::Receipt)?;
        if receipt.scope.organization_id != self.organization
            || receipt.scope.vs_id != self.recipient
            || receipt.scope.epoch != self.epoch
            || receipt.sequence != *sequence
            || receipt.digest != digest
        {
            return Err(FetchError::Receipt);
        }
        Ok(())
    }

    /// Explicit first installation. Fails if the checkpoint directory exists or
    /// the listener has no current signed snapshot; never invoked by `run`.
    pub async fn initialize(&self, now: u64) -> Result<SyncOutcome, FetchError> {
        let envelope = self.fetch().await?.ok_or(FetchError::NoSnapshot)?;
        let verified = verify(&envelope, &self.pins, &self.scope(), now)?;
        Checkpoint::initialize(
            &self.checkpoint_dir,
            self.pins.clone(),
            &self.scope(),
            &envelope,
            now,
        )?;
        self.report(&verified).await?;
        Ok(SyncOutcome::Advanced {
            sequence: verified.payload.sequence.clone(),
            digest: verified.digest().to_owned(),
        })
    }

    /// Reopens existing state only; a missing checkpoint needs explicit initialization.
    pub fn open(&self) -> Result<Checkpoint, FetchError> {
        Ok(Checkpoint::open(
            &self.checkpoint_dir,
            self.pins.clone(),
            &self.scope(),
        )?)
    }

    /// One pull. A receipt is reported only for bytes that the checkpoint holds
    /// as current. `reported` suppresses repeats within this process; the server
    /// treats identical receipts idempotently.
    pub async fn sync(
        &self,
        checkpoint: &mut Checkpoint,
        reported: &mut Option<String>,
        now: u64,
    ) -> Result<SyncOutcome, FetchError> {
        let Some(envelope) = self.fetch().await? else {
            return Ok(SyncOutcome::Withheld);
        };
        let fetched = verify(&envelope, &self.pins, &self.scope(), now)?;
        let decision = checkpoint.install(&envelope, now)?;
        let current = checkpoint.current_at(now)?;
        if current.digest() != fetched.digest() {
            return Err(FetchError::Snapshot(SnapshotError::Replay));
        }
        if reported.as_deref() != Some(current.digest()) {
            self.report(&current).await?;
            *reported = Some(current.digest().to_owned());
        }
        let sequence = current.payload.sequence.clone();
        let digest = current.digest().to_owned();
        Ok(match decision {
            Decision::Advance => SyncOutcome::Advanced { sequence, digest },
            Decision::HistoricalReplay => SyncOutcome::Unchanged { sequence, digest },
        })
    }

    /// Supervised polling loop. Failures are logged and retried at the next
    /// interval; checkpoint failures close and reopen the checkpoint.
    pub async fn run(self) {
        let mut checkpoint: Option<Checkpoint> = None;
        let mut reported = None;
        let mut ticker = tokio::time::interval(self.poll);
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        loop {
            ticker.tick().await;
            if checkpoint.is_none() {
                match self.open() {
                    Ok(opened) => checkpoint = Some(opened),
                    Err(error) => {
                        tracing::error!(%error, dir = ?self.checkpoint_dir,
                            "subject snapshot checkpoint unavailable; run vs --initialize-subject-snapshot once or recover it explicitly");
                        continue;
                    }
                }
            }
            let Some(current) = checkpoint.as_mut() else {
                continue;
            };
            match self.sync(current, &mut reported, unix_now()).await {
                Ok(SyncOutcome::Advanced { sequence, digest }) => {
                    tracing::info!(%sequence, %digest, "installed subject snapshot checkpoint (not enforced)")
                }
                Ok(SyncOutcome::Unchanged { .. }) => {}
                Ok(SyncOutcome::Withheld) => {
                    tracing::warn!("subject snapshot delivery has no current signed snapshot")
                }
                Err(FetchError::Checkpoint(error)) => {
                    tracing::error!(%error, "subject snapshot checkpoint failed; reopening");
                    checkpoint = None;
                }
                Err(error) => tracing::warn!(%error, "subject snapshot pull failed"),
            }
        }
    }
}

/// One-shot explicit initialization for the `--initialize-subject-snapshot` flag.
pub async fn initialize_from_config(config: &FetchConfig) -> Result<SyncOutcome, FetchError> {
    Fetcher::new(config)?.initialize(unix_now()).await
}

#[cfg(test)]
mod tests;
