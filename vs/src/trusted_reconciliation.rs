//! Durable, coalescing reconciliation requests. In-memory events are only hints.
use crate::assembly::Assembly;
use crate::db::{DbConnection, DbResult};
use crate::logging::targets::EVENT;
use percent_encoding::{NON_ALPHANUMERIC, percent_decode_str, utf8_percent_encode};
use serde::Serialize;
use std::sync::Arc;
use tracing::error;

const PREFIX: &str = "trusted-reconciliation:requested:";

fn request_key(source: &str) -> String {
    format!("{PREFIX}{}", utf8_percent_encode(source, NON_ALPHANUMERIC))
}
fn progress_key(source: &str) -> String {
    format!(
        "trusted-reconciliation:progress:{}",
        utf8_percent_encode(source, NON_ALPHANUMERIC)
    )
}
#[derive(Debug, Serialize)]
pub(crate) struct Status {
    pub source_id: String,
    pub state: &'static str,
    pub requested: u64,
    pub completed: u64,
    pub error: Option<String>,
}
fn generation(value: Option<String>) -> DbResult<u64> {
    value.unwrap_or_else(|| "0".into()).parse().map_err(|_| {
        redis::RedisError::from((
            redis::ErrorKind::UnexpectedReturnType,
            "invalid reconciliation generation",
        ))
    })
}
pub(crate) async fn enqueue(db: &dyn DbConnection, source: &str) -> DbResult<u64> {
    // No expiry: only a successful pass may acknowledge a persisted generation.
    // Repeated refreshes coalesce and converge on the current source data.
    db.incr(&request_key(source), 1).await
}
pub(crate) async fn status(db: &dyn DbConnection, source: &str) -> DbResult<Status> {
    let requested = generation(db.get(&request_key(source)).await?)?;
    let key = progress_key(source);
    let completed = generation(db.hget(&key, "completed").await?)?;
    let error = db
        .hget(&key, "error")
        .await?
        .filter(|value| !value.is_empty());
    let state = if requested == completed {
        "idle"
    } else if error.is_some() {
        "failed"
    } else {
        "pending"
    };
    Ok(Status {
        source_id: source.into(),
        state,
        requested,
        completed,
        error,
    })
}
async fn finish(
    db: &dyn DbConnection,
    source: &str,
    requested: u64,
    success: bool,
) -> DbResult<()> {
    let key = progress_key(source);
    if success {
        db.hset_multiple(
            &key,
            &[("completed", &requested.to_string()), ("error", "")],
        )
        .await
    } else {
        db.hset(
            &key,
            "error",
            "trusted source reconciliation failed; retry pending",
        )
        .await
    }
}
pub(crate) async fn process_pending(asm: &Arc<Assembly>) {
    if let Err(error) = process(asm).await {
        error!(target: EVENT, "durable trusted reconciliation unavailable: {}", error);
    }
}
async fn process(asm: &Arc<Assembly>) -> DbResult<()> {
    let db = asm.state_db.as_ref();
    for key in db.scan_match_all(format!("{PREFIX}*")).await? {
        let Some(encoded) = key.strip_prefix(PREFIX) else {
            continue;
        };
        let Ok(source) = percent_decode_str(encoded).decode_utf8() else {
            continue;
        };
        let current = status(db, &source).await?;
        if current.requested == current.completed {
            continue;
        }
        // Reload on restart, not just the request path: persisted work must not
        // depend on the previous process's cache or revision.
        let success = asm.ts_mgr.flush_one(&source).await.is_ok()
            && crate::event_mgr::handle_trusted_service_change(asm).await;
        finish(db, &source, current.requested, success).await?;
    }
    Ok(())
}
#[cfg(test)]
mod tests {
    use super::*;
    use crate::db::FakeDb;
    #[tokio::test]
    async fn durable_reconciliation_retry_duplicate_and_inflight_request() {
        let db = FakeDb::new();
        let source = "source:a/%";
        assert_eq!(enqueue(&db, source).await.unwrap(), 1);
        assert_eq!(enqueue(&db, source).await.unwrap(), 2);
        assert_eq!(status(&db, source).await.unwrap().state, "pending");
        finish(&db, source, 2, false).await.unwrap();
        let failed = status(&db, source).await.unwrap();
        assert_eq!(failed.state, "failed");
        assert_eq!(failed.completed, 0);
        let keys = db.scan_match_all(format!("{PREFIX}*")).await.unwrap();
        assert_eq!(keys, vec![request_key(source)]);
        enqueue(&db, source).await.unwrap();
        finish(&db, source, 2, true).await.unwrap();
        assert_eq!(status(&db, source).await.unwrap().state, "pending");
        finish(&db, source, 3, true).await.unwrap();
        finish(&db, source, 3, true).await.unwrap();
        let done = status(&db, source).await.unwrap();
        assert_eq!((done.requested, done.completed, done.state), (3, 3, "idle"));
        assert!(done.error.is_none());
        assert_eq!(status(&db, "other").await.unwrap().requested, 0);
    }
    #[tokio::test]
    async fn durable_reconciliation_enqueue_failure_is_not_acceptance() {
        let db = FakeDb::new();
        db.hset(&request_key("broken"), "wrong-type", "value")
            .await
            .unwrap();
        assert!(enqueue(&db, "broken").await.is_err());
    }
}
