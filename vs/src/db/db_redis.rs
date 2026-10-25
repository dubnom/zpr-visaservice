use redis::AsyncCommands;
use redis::Script;
use std::collections::{HashMap, HashSet};
use tokio::sync::Mutex;

use crate::db::{DbConnection, DbOp, DbResult, HashGuard, LockDescriptor, PolicyCommit};

pub struct RedisDb {
    mgr: redis::aio::ConnectionManager,
    locks: Mutex<HashSet<LockDescriptor>>,
}

impl RedisDb {
    pub fn new(mgr: redis::aio::ConnectionManager) -> Self {
        RedisDb {
            mgr,
            locks: Mutex::new(HashSet::new()),
        }
    }

    /// Clear a lock from redis.
    /// Returns TRUE if we were able to release a lock specified by the descriptor, or if no lock exists.
    /// Returns FALSE only if the lock exists and is not "ours".
    async fn release_lock_in_redis(&self, desc: &LockDescriptor) -> DbResult<bool> {
        let mut conn = self.mgr.clone();
        let script = Script::new(
            r"
                local current = redis.call('GET', KEYS[1])
                if current == false then
                    return 2
                elseif current == ARGV[1] then
                    return redis.call('DEL', KEYS[1])
                else
                    return 0
                end
        ",
        );
        // Script returns:
        //   0 if lock exists but is not ours
        //   1 if lock existed, was ours and was deleted
        //   2 if lock didn't exist to begin with

        let res: i64 = script
            .key(&desc.key)
            .arg(&desc.ident)
            .invoke_async(&mut conn)
            .await?;
        Ok(res >= 1)
    }
}

#[async_trait::async_trait]
impl DbConnection for RedisDb {
    async fn exists(&self, key: &str) -> DbResult<bool> {
        let mut conn = self.mgr.clone();
        let foo: bool = conn.exists(key).await?;
        Ok(foo)
    }

    async fn set(&self, key: &str, value: &str) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.set(key, value).await?;
        Ok(())
    }

    async fn set_ex(&self, key: &str, value: &str, seconds: u64) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.set_ex(key, value, seconds).await?;
        Ok(())
    }

    async fn get(&self, key: &str) -> DbResult<Option<String>> {
        let mut conn = self.mgr.clone();
        let res: Option<String> = conn.get(key).await?;
        Ok(res)
    }

    async fn set_bin(&self, key: &str, value: &[u8]) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.set(key, value).await?;
        Ok(())
    }

    async fn set_bin_ex(&self, key: &str, value: &[u8], seconds: u64) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.set_ex(key, value, seconds).await?;
        Ok(())
    }

    async fn get_bin(&self, key: &str) -> DbResult<Vec<u8>> {
        let mut conn = self.mgr.clone();
        let res: Vec<u8> = conn.get(key).await?;
        Ok(res)
    }

    async fn del(&self, key: &str) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.del(key).await?;
        Ok(())
    }

    async fn smembers(&self, key: &str) -> DbResult<HashSet<String>> {
        let mut conn = self.mgr.clone();
        let res: HashSet<String> = conn.smembers(key).await?;
        Ok(res)
    }

    async fn sismember(&self, key: &str, member: &str) -> DbResult<bool> {
        let mut conn = self.mgr.clone();
        let res: bool = conn.sismember(key, member).await?;
        Ok(res)
    }

    async fn hget(&self, key: &str, field: &str) -> DbResult<Option<String>> {
        let mut conn = self.mgr.clone();
        let res: Option<String> = conn.hget(key, field).await?;
        Ok(res)
    }

    async fn hgetall(&self, key: String) -> DbResult<HashMap<String, String>> {
        let mut conn = self.mgr.clone();
        let res: HashMap<String, String> = conn.hgetall(key).await?;
        Ok(res)
    }

    async fn hset(&self, key: &str, field: &str, value: &str) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.hset(key, field, value).await?;
        Ok(())
    }

    async fn hash_pair_snapshot(
        &self,
        first: &str,
        second: &str,
    ) -> DbResult<(HashMap<String, String>, HashMap<String, String>)> {
        let mut conn = self.mgr.clone();
        redis::pipe()
            .atomic()
            .hgetall(first)
            .hgetall(second)
            .query_async(&mut conn)
            .await
    }

    async fn hset_nx(&self, key: &str, field: &str, value: &str) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.hset_nx(key, field, value).await?;
        Ok(())
    }

    async fn hset_multiple(&self, key: &str, field_values: &[(&str, &str)]) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.hset_multiple(key, field_values).await?;
        Ok(())
    }

    async fn sadd(&self, key: &str, member: &str) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.sadd(key, member).await?;
        Ok(())
    }

    async fn incr(&self, key: &str, by: u64) -> DbResult<u64> {
        let mut conn = self.mgr.clone();
        let res: u64 = conn.incr(key, by).await?;
        Ok(res)
    }

    async fn expire(&self, key: &str, seconds: i64) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.expire(key, seconds).await?;
        Ok(())
    }

    async fn atomic_pipeline(&self, ops: &[DbOp]) -> DbResult<()> {
        if ops.is_empty() {
            return Ok(());
        }
        let mut conn = self.mgr.clone();
        let mut piper = redis::pipe();
        piper.atomic();
        for op in ops {
            let (command, args) = op.command_parts();
            piper.cmd(command);
            for arg in args {
                piper.arg(arg);
            }
        }
        let _: () = piper.query_async(&mut conn).await?;
        Ok(())
    }

    async fn atomic_pipeline_if_set_and_hash_match(
        &self,
        key: &str,
        expected: &HashSet<String>,
        hash_guard: Option<HashGuard<'_>>,
        ops: &[DbOp],
    ) -> DbResult<bool> {
        let script = Script::new(
            r"
                if ARGV[1] == '1' then
                    if redis.call('EXISTS', KEYS[2]) == 0 then return 0 end
                    local current = redis.call('HGET', KEYS[2], ARGV[2])
                    if ARGV[3] == '1' then
                        if current ~= ARGV[4] then return 0 end
                    elseif current ~= false then
                        return 0
                    end
                end
                local count = tonumber(ARGV[5])
                local current = redis.call('SMEMBERS', KEYS[1])
                if #current ~= count then return 0 end
                local expected = {}
                for i = 1, count do expected[ARGV[i + 5]] = true end
                for _, member in ipairs(current) do
                    if not expected[member] then return 0 end
                end
                local cursor = count + 6
                while cursor <= #ARGV do
                    local size = tonumber(ARGV[cursor])
                    local command = {}
                    for i = 1, size do command[i] = ARGV[cursor + i] end
                    redis.call(unpack(command))
                    cursor = cursor + size + 1
                end
                return 1
            ",
        );
        let mut invocation = script.prepare_invoke();
        invocation.key(key);
        if let Some(guard) = hash_guard {
            invocation
                .key(guard.key)
                .arg(1)
                .arg(guard.field)
                .arg(u8::from(guard.expected.is_some()))
                .arg(guard.expected.unwrap_or(""));
        } else {
            invocation.arg(0).arg("").arg(0).arg("");
        }
        invocation.arg(expected.len());
        for member in expected {
            invocation.arg(member);
        }
        for op in ops {
            let (command, args) = op.command_parts();
            invocation.arg(args.len() + 1).arg(command);
            for arg in args {
                invocation.arg(arg);
            }
        }
        let mut conn = self.mgr.clone();
        let result: i64 = invocation.invoke_async(&mut conn).await?;
        Ok(result == 1)
    }

    async fn begin_policy_operation(
        &self,
        operation_key: &str,
        current_key: &str,
        expected_digest: &str,
        expected_vinst: &str,
        pending_receipt: &str,
    ) -> DbResult<i64> {
        let mut conn = self.mgr.clone();
        let script = Script::new(
            r"
                local existing = redis.call('GET', KEYS[1])
                if existing then
                    local old = cjson.decode(existing)
                    local requested = cjson.decode(ARGV[3])
                    if old.request_fingerprint == requested.request_fingerprint then
                        return 1
                    end
                    return -1
                end
                local digest = redis.call('HGET', KEYS[2], 'phash')
                local vinst = redis.call('HGET', KEYS[2], 'vinst')
                if digest ~= ARGV[1] or vinst ~= ARGV[2] then
                    return -2
                end
                redis.call('SET', KEYS[1], ARGV[3])
                return 0
        ",
        );
        script
            .key(operation_key)
            .key(current_key)
            .arg(expected_digest)
            .arg(expected_vinst)
            .arg(pending_receipt)
            .invoke_async(&mut conn)
            .await
    }

    async fn commit_policy_operation(&self, commit: PolicyCommit<'_>) -> DbResult<i64> {
        let PolicyCommit {
            operation_key,
            current_key,
            container_key,
            expected_digest,
            expected_vinst,
            candidate_digest,
            candidate_bytes,
            next_vinst,
            changed_at,
            pending_receipt,
        } = commit;
        let mut conn = self.mgr.clone();
        let script = Script::new(
            r"
                local existing = redis.call('GET', KEYS[1])
                if not existing then
                    return -1
                end
                local operation = cjson.decode(existing)
                local applied = cjson.decode(ARGV[7])
                if operation.request_fingerprint ~= applied.request_fingerprint then
                    return -1
                end
                if operation.status == 'succeeded' then
                    return 1
                end
                if operation.status ~= 'pending' and operation.status ~= 'uncertain' then
                    return -3
                end
                local digest = redis.call('HGET', KEYS[2], 'phash')
                local vinst = redis.call('HGET', KEYS[2], 'vinst')
                if digest == ARGV[3] and vinst == ARGV[4] then
                    return 1
                end
                if digest ~= ARGV[1] or vinst ~= ARGV[2] then
                    return -2
                end
                redis.call('SET', KEYS[3], ARGV[5])
                redis.call('HSET', KEYS[2], 'phash', ARGV[3], 'vinst', ARGV[4], 'ctime', ARGV[6])
                redis.call('SET', KEYS[1], ARGV[7])
                return 0
        ",
        );
        script
            .key(operation_key)
            .key(current_key)
            .key(container_key)
            .arg(expected_digest)
            .arg(expected_vinst)
            .arg(candidate_digest)
            .arg(next_vinst)
            .arg(candidate_bytes)
            .arg(changed_at)
            .arg(pending_receipt)
            .invoke_async(&mut conn)
            .await
    }

    async fn finish_policy_operation(
        &self,
        operation_key: &str,
        current_key: &str,
        expected_receipt: &str,
        receipt: &str,
    ) -> DbResult<i64> {
        let Some(updated) =
            super::policy::validate_operation_transition(expected_receipt, receipt)?
        else {
            return Ok(-3);
        };
        let mut conn = self.mgr.clone();
        let script = Script::new(
            r"
                local existing = redis.call('GET', KEYS[1])
                if not existing then
                    return -1
                end
                if existing ~= ARGV[1] then
                    return -4
                end
                local operation = cjson.decode(existing)
                if operation.status ~= 'pending' and operation.status ~= 'uncertain' then
                    return 1
                end
                local updated = cjson.decode(ARGV[2])
                if updated.status == 'succeeded' then
                    local digest = redis.call('HGET', KEYS[2], 'phash')
                    local vinst = redis.call('HGET', KEYS[2], 'vinst')
                    if digest ~= ARGV[3] or vinst ~= ARGV[4] then
                        return -2
                    end
                end
                redis.call('SET', KEYS[1], ARGV[2])
                return 0
        ",
        );
        script
            .key(operation_key)
            .key(current_key)
            .arg(expected_receipt)
            .arg(receipt)
            .arg(updated.applied_digest.as_deref().unwrap_or(""))
            .arg(
                updated
                    .applied_vinst
                    .map(|vinst| vinst.to_string())
                    .unwrap_or_default(),
            )
            .invoke_async(&mut conn)
            .await
    }

    async fn authorize_policy_operation(
        &self,
        nonce_key: &str,
        approval_key: &str,
        approval: &str,
    ) -> DbResult<i64> {
        let mut conn = self.mgr.clone();
        Script::new(
            r"
                if redis.call('EXISTS', KEYS[1]) == 1 then
                    return -1
                end
                local existing = redis.call('GET', KEYS[2])
                if existing and existing ~= ARGV[1] then
                    return -2
                end
                redis.call('SET', KEYS[1], KEYS[2])
                redis.call('SET', KEYS[2], ARGV[1])
                return 0
            ",
        )
        .key(nonce_key)
        .key(approval_key)
        .arg(approval)
        .invoke_async(&mut conn)
        .await
    }

    async fn scan_match_all(&self, pattern: String) -> DbResult<Vec<String>> {
        let mut conn = self.mgr.clone();
        let mut results = Vec::new();

        // Redis SCAN_MATCH can return duplicate keys so we de-dupe before returning.
        let mut seen = HashSet::new();

        let mut iter: redis::AsyncIter<String> = conn.scan_match(pattern).await?;
        while let Some(key_res) = iter.next_item().await {
            let key = key_res?;
            if seen.insert(key.clone()) {
                results.push(key);
            }
        }

        Ok(results)
    }

    async fn clear_state(&self) -> DbResult<()> {
        let mut conn = self.mgr.clone();
        let _: () = conn.flushdb().await?;
        Ok(())
    }

    async fn shutdown_cleanup(&self) -> DbResult<()> {
        // Note that this holds the lock across the redis I/O calls.
        for ldesc in self.locks.lock().await.drain() {
            // Best effort... this call produce an error.
            let _ = self.release_lock_in_redis(&ldesc).await;
        }
        Ok(())
    }

    async fn acquire_or_renew_lock(&self, desc: &LockDescriptor) -> DbResult<bool> {
        // Note that this holds the lock across the redis I/O call.
        let mut locks_guard = self.locks.lock().await;
        let mut conn = self.mgr.clone();
        let script = Script::new(
            r"
                local current = redis.call('GET', KEYS[1])
                if current == false then
                    -- Key doesn't exist, acquire it
                    redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
                    return 1
                elseif current == ARGV[1] then
                    -- We hold the lock, reset the TTL
                    redis.call('PEXPIRE', KEYS[1], ARGV[2])
                    return 1
                else
                    -- Someone else holds it
                    return 0
                end
            ",
        );

        let res: i64 = script
            .key(&desc.key)
            .arg(&desc.ident)
            .arg(desc.timeout.as_millis() as u64)
            .invoke_async(&mut conn)
            .await?;

        // Note: it is possible for the LUA to run, but to then lose the return message from redis.
        // Since we are running REDIS on localhost this is unlikely. If that were to happen we would
        // end up with the lock in redis but not in our local set.

        if res == 1 {
            locks_guard.insert(desc.clone());
        }
        Ok(res == 1)
    }

    async fn release_lock(&self, desc: &LockDescriptor) -> DbResult<bool> {
        // Note that this holds the lock across the redis I/O call.
        let mut locks_guard = self.locks.lock().await;
        let released = self.release_lock_in_redis(desc).await?;

        // May as well remove from our memory since it definately isn't in the DB.
        locks_guard.remove(desc);

        // Note: LUA script returns 2 when lock doesn't exist to begin with.
        Ok(released)
    }
}

#[cfg(test)]
#[cfg(unix)]
mod redis_operation_tests {
    //! Opt in with `cargo test -p vs --bin vs redis_operation_tests -- --ignored`.
    //! Redis transaction/transport/Lua tests; proof signatures and expiry belong to receiver tests.

    use super::*;
    use crate::db::{PolicyOperationReceipt, PolicyOperationStatus};
    use std::fs::{self, File};
    use std::net::TcpListener;
    use std::os::unix::fs::PermissionsExt;
    use std::process::{Child, Command, Stdio};
    use std::time::Duration;
    use tempfile::TempDir;

    const CURRENT: &str = "policy:current";
    const OPERATION: &str = "policy:operations:test";
    const CONTAINER: &str = "policies:test:container";
    const CANDIDATE: &[u8] = b"isolated policy candidate";

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn trusted_source_control_persistence_contract() {
        let (mut server, db, _) = PrivateRedis::start().await;
        crate::trusted_services::control::assert_persistence_contract(&db).await;
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn session_guard_contract() {
        let (mut server, db, _) = PrivateRedis::start().await;
        crate::db::assert_session_guard_contract(&db).await;
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn device_authentication_roundtrips_refresh_and_reauthentication_on_redis() {
        use crate::db::ActorRepo;
        use crate::test_helpers::make_actor_with_services_defexp;
        use libeval::actor::{DeviceAuthentication, DeviceAuthority};
        use libeval::attribute::{Attribute, ROLE_ADAPTER, key};

        let (mut server, db, client) = PrivateRedis::start().await;
        let repo = ActorRepo::new(std::sync::Arc::new(db));
        let mut actor = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            "fd5a:5052::71",
            &["directory"],
            "verified-device",
        );
        let address = *actor.get_zpr_addr().unwrap();
        let identity = DeviceAuthentication::new(
            DeviceAuthority::AuthenticationService {
                service_id: "bas".into(),
                issuer: "zpr/bas".into(),
            },
            "verified-device".into(),
            uuid::Uuid::new_v4().to_string(),
            std::time::SystemTime::now() + Duration::from_secs(600),
        )
        .unwrap();
        actor.set_device_authentication(identity.clone()).unwrap();
        repo.add_actor(&actor).await.unwrap();
        let restored = ActorRepo::new(std::sync::Arc::new(independent_db(&client).await));
        let mut loaded = restored.get_actor_by_zpr_addr(&address).await.unwrap();
        assert_eq!(loaded.device_authentication(), Some(&identity));
        loaded
            .add_attribute(Attribute::builder(key::CN).value("enriched-cn"))
            .unwrap();
        restored.update_actor(&loaded).await.unwrap();
        let loaded = repo.get_actor_by_zpr_addr(&address).await.unwrap();
        assert_eq!(loaded.device_authentication(), Some(&identity));
        assert_eq!(loaded.get_cn(), Some("enriched-cn"));
        let replacement = DeviceAuthentication::new(
            DeviceAuthority::Bootstrap {
                visa_service: "vs.zpr/test".into(),
            },
            "replacement-device".into(),
            uuid::Uuid::new_v4().to_string(),
            std::time::SystemTime::now() + Duration::from_secs(300),
        )
        .unwrap();
        actor
            .set_device_authentication(replacement.clone())
            .unwrap();
        repo.update_authenticated_actor(&actor).await.unwrap();
        assert!(matches!(
            restored.update_actor(&loaded).await,
            Err(crate::error::StoreError::Conflict(_))
        ));
        let restarted = ActorRepo::new(std::sync::Arc::new(independent_db(&client).await));
        assert_eq!(
            restarted
                .get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .device_authentication(),
            Some(&replacement)
        );
        repo.rm_actor_by_zpr_addr(&address).await.unwrap();
        assert!(restored.get_actor_by_zpr_addr(&address).await.is_err());
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn atomic_pipeline_queue_error_aborts_all_commands() {
        let (mut server, db, _) = PrivateRedis::start().await;
        db.set("pipeline:kept", "original").await.unwrap();
        let result = db
            .atomic_pipeline(&[
                DbOp::Del("pipeline:kept".into()),
                // Empty HSET is a Redis queue-time arity error, not an
                // execution-time type error. EXEC must abort the entire batch.
                DbOp::HSetMultiple {
                    hash_key: "pipeline:invalid".into(),
                    field_values: vec![],
                },
                DbOp::SetBin {
                    key: "pipeline:not-created".into(),
                    value: b"uncommitted".to_vec(),
                },
            ])
            .await;
        assert!(result.is_err());
        assert_eq!(
            db.get("pipeline:kept").await.unwrap().as_deref(),
            Some("original")
        );
        assert!(!db.exists("pipeline:not-created").await.unwrap());
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn atomic_pipeline_commits_supported_operations_and_expiration() {
        let (mut server, db, client) = PrivateRedis::start().await;
        db.set("pipeline:deleted", "old").await.unwrap();
        db.sadd("pipeline:set", "removed").await.unwrap();
        db.sadd("pipeline:set", "kept").await.unwrap();
        db.atomic_pipeline(&[
            DbOp::Del("pipeline:deleted".into()),
            DbOp::SetBin {
                key: "pipeline:blob".into(),
                value: vec![0, 255, 1],
            },
            DbOp::Expire {
                key: "pipeline:blob".into(),
                seconds: 120,
            },
            DbOp::SRem {
                set_key: "pipeline:set".into(),
                member: "removed".into(),
            },
            DbOp::SAdd {
                set_key: "pipeline:set".into(),
                member: "added".into(),
            },
            DbOp::HSet {
                hash_key: "pipeline:hash".into(),
                field: "one".into(),
                value: "1".into(),
            },
            DbOp::HSetMultiple {
                hash_key: "pipeline:hash".into(),
                field_values: vec![("two".into(), "2".into()), ("three".into(), "3".into())],
            },
        ])
        .await
        .unwrap();
        let reconnected = independent_db(&client).await;
        assert!(!reconnected.exists("pipeline:deleted").await.unwrap());
        assert_eq!(
            reconnected.get_bin("pipeline:blob").await.unwrap(),
            [0, 255, 1]
        );
        assert_eq!(
            reconnected.smembers("pipeline:set").await.unwrap(),
            std::collections::HashSet::from(["kept".into(), "added".into()])
        );
        for (field, value) in [("one", "1"), ("two", "2"), ("three", "3")] {
            assert_eq!(
                reconnected
                    .hget("pipeline:hash", field)
                    .await
                    .unwrap()
                    .as_deref(),
                Some(value)
            );
        }
        let mut connection = client.get_multiplexed_async_connection().await.unwrap();
        let ttl: i64 = redis::cmd("TTL")
            .arg("pipeline:blob")
            .query_async(&mut connection)
            .await
            .unwrap();
        assert!((1..=120).contains(&ttl));
        reconnected.atomic_pipeline(&[]).await.unwrap();
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn atomic_pipeline_execution_error_is_not_a_rollback() {
        let (mut server, db, _) = PrivateRedis::start().await;
        db.set("pipeline:wrong-type", "string").await.unwrap();
        let result = db
            .atomic_pipeline(&[
                DbOp::SetBin {
                    key: "pipeline:before".into(),
                    value: b"applied".to_vec(),
                },
                DbOp::HSet {
                    hash_key: "pipeline:wrong-type".into(),
                    field: "field".into(),
                    value: "value".into(),
                },
                DbOp::SetBin {
                    key: "pipeline:after".into(),
                    value: b"also-applied".to_vec(),
                },
            ])
            .await;
        assert!(result.is_err());
        assert_eq!(
            db.get("pipeline:before").await.unwrap().as_deref(),
            Some("applied")
        );
        assert_eq!(
            db.get("pipeline:wrong-type").await.unwrap().as_deref(),
            Some("string")
        );
        assert_eq!(
            db.get("pipeline:after").await.unwrap().as_deref(),
            Some("also-applied")
        );
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn guarded_pipeline_rejects_equal_size_mismatch_and_preserves_binary_commands() {
        let (mut server, db, _) = PrivateRedis::start().await;
        db.sadd("guard:set", "a").await.unwrap();
        let ops = [
            DbOp::SetBin {
                key: "guard:blob".into(),
                value: vec![0, 255, 13, 10],
            },
            DbOp::Expire {
                key: "guard:blob".into(),
                seconds: 120,
            },
            DbOp::Del("guard:set".into()),
            DbOp::SAdd {
                set_key: "guard:set".into(),
                member: "b".into(),
            },
            DbOp::HSet {
                hash_key: "guard:hash".into(),
                field: "first".into(),
                value: "one".into(),
            },
            DbOp::HSetMultiple {
                hash_key: "guard:hash".into(),
                field_values: vec![("second".into(), "two".into())],
            },
            DbOp::SRem {
                set_key: "guard:peer".into(),
                member: "unused".into(),
            },
        ];
        assert!(
            !db.atomic_pipeline_if_set_matches(
                "guard:set",
                &HashSet::from(["different".into()]),
                &ops,
            )
            .await
            .unwrap()
        );
        assert!(!db.exists("guard:blob").await.unwrap());
        assert!(
            db.atomic_pipeline_if_set_matches("guard:set", &HashSet::from(["a".into()]), &ops,)
                .await
                .unwrap()
        );
        assert_eq!(db.get_bin("guard:blob").await.unwrap(), [0, 255, 13, 10]);
        assert_eq!(
            db.smembers("guard:set").await.unwrap(),
            HashSet::from(["b".into()])
        );
        assert_eq!(
            db.hget("guard:hash", "first").await.unwrap().as_deref(),
            Some("one")
        );
        assert_eq!(
            db.hget("guard:hash", "second").await.unwrap().as_deref(),
            Some("two")
        );
        assert!(
            !db.atomic_pipeline_if_set_matches(
                "guard:set",
                &HashSet::from(["a".into()]),
                &[DbOp::Del("guard:blob".into())],
            )
            .await
            .unwrap()
        );
        assert_eq!(db.get_bin("guard:blob").await.unwrap(), [0, 255, 13, 10]);
        assert!(
            db.atomic_pipeline_if_set_matches("guard:absent", &HashSet::new(), &[])
                .await
                .unwrap()
        );
        db.set("guard:wrong-type", "not a set").await.unwrap();
        assert!(
            db.atomic_pipeline_if_set_matches("guard:wrong-type", &HashSet::new(), &ops)
                .await
                .is_err()
        );
        assert_eq!(
            db.smembers("guard:set").await.unwrap(),
            HashSet::from(["b".into()])
        );
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn guarded_pipeline_has_exactly_one_winner_across_connections() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let peer = independent_db(&client).await;
        db.sadd("race:set", "old").await.unwrap();
        let expected = HashSet::from(["old".into()]);
        let first = [
            DbOp::Del("race:set".into()),
            DbOp::SAdd {
                set_key: "race:set".into(),
                member: "first".into(),
            },
            DbOp::SetBin {
                key: "race:winner".into(),
                value: b"first".to_vec(),
            },
        ];
        let second = [
            DbOp::Del("race:set".into()),
            DbOp::SAdd {
                set_key: "race:set".into(),
                member: "second".into(),
            },
            DbOp::SetBin {
                key: "race:winner".into(),
                value: b"second".to_vec(),
            },
        ];
        let (a, b) = tokio::join!(
            db.atomic_pipeline_if_set_matches("race:set", &expected, &first),
            peer.atomic_pipeline_if_set_matches("race:set", &expected, &second),
        );
        let (a, b) = (a.unwrap(), b.unwrap());
        assert_ne!(a, b);
        let winner = if a { "first" } else { "second" };
        assert_eq!(
            db.smembers("race:set").await.unwrap(),
            HashSet::from([winner.into()])
        );
        assert_eq!(
            peer.get("race:winner").await.unwrap().as_deref(),
            Some(winner)
        );
        let now = HashSet::from([winner.into()]);
        let retry = if a { &second } else { &first };
        assert!(
            peer.atomic_pipeline_if_set_matches("race:set", &now, retry)
                .await
                .unwrap()
        );
        let final_winner = if a { "second" } else { "first" };
        assert_eq!(
            db.get("race:winner").await.unwrap().as_deref(),
            Some(final_winner)
        );
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn guarded_pipeline_execution_error_stops_without_rollback() {
        let (mut server, db, _) = PrivateRedis::start().await;
        db.set("guard:wrong-type", "string").await.unwrap();
        let result = db
            .atomic_pipeline_if_set_matches(
                "guard:missing",
                &HashSet::new(),
                &[
                    DbOp::SetBin {
                        key: "guard:before".into(),
                        value: b"applied".to_vec(),
                    },
                    DbOp::HSet {
                        hash_key: "guard:wrong-type".into(),
                        field: "field".into(),
                        value: "value".into(),
                    },
                    DbOp::SetBin {
                        key: "guard:after".into(),
                        value: b"not-applied".to_vec(),
                    },
                ],
            )
            .await;
        assert!(result.is_err());
        assert_eq!(
            db.get("guard:before").await.unwrap().as_deref(),
            Some("applied")
        );
        assert_eq!(
            db.get("guard:wrong-type").await.unwrap().as_deref(),
            Some("string")
        );
        assert!(!db.exists("guard:after").await.unwrap());
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn hash_pair_snapshot_handles_missing_hashes_and_propagates_wrong_types() {
        let (mut server, db, _) = PrivateRedis::start().await;
        let (first, second) = db
            .hash_pair_snapshot("snapshot:first", "snapshot:second")
            .await
            .unwrap();
        assert!(first.is_empty());
        assert!(second.is_empty());
        db.hset("snapshot:first", "field", "value").await.unwrap();
        let (first, second) = db
            .hash_pair_snapshot("snapshot:first", "snapshot:second")
            .await
            .unwrap();
        assert_eq!(first.get("field").map(String::as_str), Some("value"));
        assert!(second.is_empty());
        db.set("snapshot:second", "wrong-type").await.unwrap();
        assert!(
            db.hash_pair_snapshot("snapshot:first", "snapshot:second")
                .await
                .is_err()
        );
        assert!(
            db.hash_pair_snapshot("snapshot:second", "snapshot:first")
                .await
                .is_err()
        );
        assert_eq!(
            db.hget("snapshot:first", "field").await.unwrap().as_deref(),
            Some("value")
        );
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn hash_pair_snapshot_never_mixes_generations_across_connections() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let writer = independent_db(&client).await;
        let write = |generation: usize| {
            [
                DbOp::HSet {
                    hash_key: "snapshot:metadata".into(),
                    field: "generation".into(),
                    value: generation.to_string(),
                },
                DbOp::HSet {
                    hash_key: "snapshot:attrs".into(),
                    field: "generation".into(),
                    value: generation.to_string(),
                },
            ]
        };
        writer.atomic_pipeline(&write(0)).await.unwrap();
        let ((), ()) = tokio::join!(
            async {
                for generation in 1..=200 {
                    writer.atomic_pipeline(&write(generation)).await.unwrap();
                }
            },
            async {
                for _ in 0..200 {
                    let (metadata, attrs) = db
                        .hash_pair_snapshot("snapshot:metadata", "snapshot:attrs")
                        .await
                        .unwrap();
                    assert!(metadata.contains_key("generation"));
                    assert_eq!(metadata.get("generation"), attrs.get("generation"));
                }
            },
        );
        let (metadata, attrs) = db
            .hash_pair_snapshot("snapshot:metadata", "snapshot:attrs")
            .await
            .unwrap();
        assert_eq!(metadata.get("generation").map(String::as_str), Some("200"));
        assert_eq!(metadata, attrs);
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn cn_lookup_snapshot_preserves_targeted_decode_on_redis() {
        use crate::db::ActorRepo;
        use crate::error::StoreError;
        use crate::test_helpers::make_actor_with_services_defexp;
        use libeval::attribute::ROLE_ADAPTER;
        let (mut server, db, client) = PrivateRedis::start().await;
        let db = std::sync::Arc::new(db);
        let repo = ActorRepo::new(db.clone());
        let address = "fd5a:5052::71".parse().unwrap();
        repo.add_actor(&make_actor_with_services_defexp(
            ROLE_ADAPTER,
            "fd5a:5052::71",
            &[],
            "snapshot.example",
        ))
        .await
        .unwrap();
        let independent = ActorRepo::new(std::sync::Arc::new(independent_db(&client).await));
        db.hset("actor:fd5a-5052--71:attrs", "unrelated", "bad JSON")
            .await
            .unwrap();
        db.hset("actor:fd5a-5052--71", "identity_keys", "bad JSON")
            .await
            .unwrap();
        assert_eq!(
            independent.get_cn_by_zpr_addr(&address).await.unwrap(),
            "snapshot.example"
        );
        assert!(independent.get_actor_by_zpr_addr(&address).await.is_err());
        db.del("actor:fd5a-5052--71").await.unwrap();
        assert!(matches!(
            independent.get_cn_by_zpr_addr(&address).await,
            Err(StoreError::NotFound(_))
        ));
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn actor_service_update_roundtrips_indexes_on_redis() {
        use crate::db::ActorRepo;
        use crate::test_helpers::make_actor_with_services_defexp;
        use libeval::attribute::ROLE_ADAPTER;

        let (mut server, db, client) = PrivateRedis::start().await;
        let repo = ActorRepo::new(std::sync::Arc::new(db));
        let address: std::net::IpAddr = "fd5a:5052::61".parse().unwrap();
        let peer: std::net::IpAddr = "fd5a:5052::62".parse().unwrap();
        for (addr, cn) in [(address, "first"), (peer, "peer")] {
            repo.add_actor(&make_actor_with_services_defexp(
                ROLE_ADAPTER,
                &addr.to_string(),
                &["shared:svc", "old%svc"],
                cn,
            ))
            .await
            .unwrap();
        }
        let updated = make_actor_with_services_defexp(
            ROLE_ADAPTER,
            &address.to_string(),
            &["new:svc", "shared:svc"],
            "first",
        );
        repo.update_actor(&updated).await.unwrap();
        let restored = ActorRepo::new(std::sync::Arc::new(independent_db(&client).await));
        assert_eq!(
            restored.get_zpr_addrs_for_service("old%svc").await.unwrap(),
            [peer]
        );
        assert_eq!(
            restored
                .get_zpr_addrs_for_service("shared:svc")
                .await
                .unwrap(),
            [address, peer]
        );
        assert_eq!(
            restored.get_zpr_addrs_for_service("new:svc").await.unwrap(),
            [address]
        );
        let mut names = restored.list_services_for_actor(&address).await.unwrap();
        names.sort();
        assert_eq!(names, ["new:svc", "shared:svc"]);
        let loaded = restored.get_actor_by_zpr_addr(&address).await.unwrap();
        assert!(loaded.provides("new:svc"));
        assert!(!loaded.provides("old%svc"));
        let empty =
            make_actor_with_services_defexp(ROLE_ADAPTER, &address.to_string(), &[], "first");
        restored.update_actor(&empty).await.unwrap();
        assert!(
            repo.list_services_for_actor(&address)
                .await
                .unwrap()
                .is_empty()
        );
        assert!(
            repo.get_zpr_addrs_for_service("new:svc")
                .await
                .unwrap()
                .is_empty()
        );
        assert_eq!(
            repo.get_zpr_addrs_for_service("shared:svc").await.unwrap(),
            [peer]
        );
        assert_eq!(
            repo.get_zpr_addrs_for_service("old%svc").await.unwrap(),
            [peer]
        );
        assert!(
            !repo
                .get_actor_by_zpr_addr(&address)
                .await
                .unwrap()
                .provides("shared:svc")
        );
        server.stop().unwrap();
    }

    struct PrivateRedis {
        child: Child,
        directory: Option<TempDir>,
    }

    impl PrivateRedis {
        async fn start() -> (Self, RedisDb, redis::Client) {
            let directory = tempfile::Builder::new()
                .prefix("vs-redis-")
                .tempdir_in("/tmp")
                .expect("create private Redis directory");
            fs::set_permissions(directory.path(), fs::Permissions::from_mode(0o700))
                .expect("protect Redis directory");
            let socket = directory.path().join("redis.sock");
            let listener = TcpListener::bind(("127.0.0.1", 0)).expect("reserve loopback port");
            let port = listener.local_addr().unwrap().port();
            let log_path = directory.path().join("redis.log");
            let log = File::create(&log_path).expect("create Redis diagnostic log");
            drop(listener);
            let child = Command::new("redis-server")
                .args([
                    "--bind",
                    "127.0.0.1",
                    "--port",
                    &port.to_string(),
                    "--protected-mode",
                    "yes",
                    "--save",
                    "",
                    "--appendonly",
                    "no",
                    "--daemonize",
                    "no",
                    "--unixsocketperm",
                    "700",
                    "--unixsocket",
                ])
                .arg(&socket)
                .arg("--dir")
                .arg(directory.path())
                .stdin(Stdio::null())
                .stdout(Stdio::from(log.try_clone().unwrap()))
                .stderr(Stdio::from(log))
                .spawn()
                .expect("redis-server must be available on PATH; no installation is attempted");
            let mut server = Self {
                child,
                directory: Some(directory),
            };
            // Use the private socket so a loopback port collision never contacts another Redis.
            let client = redis::Client::open(format!("unix://{}", socket.display())).unwrap();
            let deadline = tokio::time::Instant::now() + Duration::from_secs(10);
            loop {
                if let Some(status) = server.child.try_wait().expect("check Redis child") {
                    panic!(
                        "private Redis exited with {status}: {}",
                        fs::read_to_string(&log_path).unwrap()
                    );
                }
                if let Ok(Ok(mut connection)) = tokio::time::timeout(
                    Duration::from_millis(200),
                    client.get_multiplexed_async_connection(),
                )
                .await
                {
                    let info: String = redis::cmd("INFO")
                        .arg("server")
                        .query_async(&mut connection)
                        .await
                        .expect("identify private Redis before any writes");
                    assert!(
                        info.lines().any(|line| {
                            line.trim() == format!("process_id:{}", server.child.id())
                        }),
                        "Redis does not belong to the test's child"
                    );
                    let config: HashMap<String, String> = redis::cmd("CONFIG")
                        .arg("GET")
                        .arg("bind")
                        .arg("save")
                        .arg("appendonly")
                        .query_async(&mut connection)
                        .await
                        .unwrap();
                    assert_eq!(config["bind"], "127.0.0.1");
                    assert_eq!(config["save"], "");
                    assert_eq!(config["appendonly"], "no");
                    eprintln!(
                        "private Redis pid={} verified: loopback-only TCP, private socket, persistence disabled",
                        server.child.id()
                    );
                    let db = RedisDb::new(
                        redis::aio::ConnectionManager::new(client.clone())
                            .await
                            .unwrap(),
                    );
                    db.hset_multiple(
                        CURRENT,
                        &[
                            ("phash", &old_digest()),
                            ("vinst", "7"),
                            ("ctime", "before"),
                        ],
                    )
                    .await
                    .unwrap();
                    return (server, db, client);
                }
                assert!(
                    tokio::time::Instant::now() < deadline,
                    "private Redis startup timed out: {}",
                    fs::read_to_string(&log_path).unwrap()
                );
                tokio::time::sleep(Duration::from_millis(20)).await;
            }
        }

        fn stop(&mut self) -> std::io::Result<()> {
            if self.child.try_wait()?.is_none() {
                self.child.kill()?;
            }
            self.child.wait()?;
            assert!(
                self.child.try_wait()?.is_some(),
                "Redis child was not reaped"
            );
            if let Some(directory) = self.directory.take() {
                let path = directory.path().to_path_buf();
                directory.close()?;
                assert!(!path.exists(), "private Redis directory was not removed");
                eprintln!(
                    "private Redis pid={} reaped; private directory removed",
                    self.child.id()
                );
            }
            Ok(())
        }
    }

    impl Drop for PrivateRedis {
        fn drop(&mut self) {
            if let Err(error) = self.stop() {
                if std::thread::panicking() {
                    eprintln!("private Redis cleanup failed: {error}");
                } else {
                    panic!("private Redis cleanup failed: {error}");
                }
            }
        }
    }

    fn old_digest() -> String {
        hex::encode(openssl::sha::sha256(b"previous policy"))
    }

    fn pending() -> PolicyOperationReceipt {
        PolicyOperationReceipt {
            operation_id: "test".into(),
            target_id: "isolated".into(),
            source_revision: 1,
            candidate_digest: hex::encode(openssl::sha::sha256(CANDIDATE)),
            expected_current_digest: old_digest(),
            expected_current_vinst: 7,
            request_fingerprint: "test-request-fingerprint".into(),
            status: PolicyOperationStatus::Pending,
            applied_digest: None,
            applied_vinst: None,
            error: None,
        }
    }

    fn encoded(receipt: &PolicyOperationReceipt) -> String {
        serde_json::to_string(receipt).unwrap()
    }

    fn applied() -> PolicyOperationReceipt {
        let mut receipt = pending();
        receipt.applied_digest = Some(receipt.candidate_digest.clone());
        receipt.applied_vinst = Some(8);
        receipt
    }

    async fn reserve(db: &RedisDb, receipt: &str) -> i64 {
        db.begin_policy_operation(OPERATION, CURRENT, &old_digest(), "7", receipt)
            .await
            .unwrap()
    }

    async fn commit(db: &RedisDb, receipt: &str, changed_at: &str) -> i64 {
        db.commit_policy_operation(PolicyCommit {
            operation_key: OPERATION,
            current_key: CURRENT,
            container_key: CONTAINER,
            expected_digest: &old_digest(),
            expected_vinst: "7",
            candidate_digest: &pending().candidate_digest,
            candidate_bytes: CANDIDATE,
            next_vinst: "8",
            changed_at,
            pending_receipt: receipt,
        })
        .await
        .unwrap()
    }

    async fn independent_db(client: &redis::Client) -> RedisDb {
        RedisDb::new(
            redis::aio::ConnectionManager::new(client.clone())
                .await
                .unwrap(),
        )
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn duplicate_reservation_is_atomic() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let other = independent_db(&client).await;
        let receipt = encoded(&pending());
        let (left, right) = tokio::join!(reserve(&db, &receipt), reserve(&other, &receipt));
        assert!(matches!((left, right), (0, 1) | (1, 0)));
        let mut conflicting = pending();
        conflicting.request_fingerprint = "different-request".into();
        assert_eq!(reserve(&db, &encoded(&conflicting)).await, -1);
        assert_eq!(db.get(OPERATION).await.unwrap(), Some(receipt));
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn stale_reservation_and_commit_preserve_state() {
        let (mut server, db, _) = PrivateRedis::start().await;
        let receipt = encoded(&pending());
        assert_eq!(
            db.begin_policy_operation(OPERATION, CURRENT, &old_digest(), "6", &receipt)
                .await
                .unwrap(),
            -2
        );
        assert_eq!(
            db.begin_policy_operation(OPERATION, CURRENT, "different-digest", "7", &receipt)
                .await
                .unwrap(),
            -2
        );
        assert!(!db.exists(OPERATION).await.unwrap());
        assert_eq!(reserve(&db, &receipt).await, 0);
        db.hset(CURRENT, "vinst", "9").await.unwrap();
        assert_eq!(commit(&db, &encoded(&applied()), "after").await, -2);
        assert_eq!(db.get(OPERATION).await.unwrap(), Some(receipt));
        assert!(!db.exists(CONTAINER).await.unwrap());
        assert_eq!(db.hget(CURRENT, "phash").await.unwrap(), Some(old_digest()));
        assert_eq!(
            db.hget(CURRENT, "vinst").await.unwrap().as_deref(),
            Some("9")
        );
        assert_eq!(
            db.hget(CURRENT, "ctime").await.unwrap().as_deref(),
            Some("before")
        );
        db.hset_multiple(CURRENT, &[("phash", "different-digest"), ("vinst", "7")])
            .await
            .unwrap();
        assert_eq!(commit(&db, &encoded(&applied()), "after").await, -2);
        assert_eq!(
            db.hget(CURRENT, "phash").await.unwrap().as_deref(),
            Some("different-digest")
        );
        assert_eq!(
            db.hget(CURRENT, "vinst").await.unwrap().as_deref(),
            Some("7")
        );
        assert_eq!(db.get(OPERATION).await.unwrap(), Some(encoded(&pending())));
        assert!(!db.exists(CONTAINER).await.unwrap());
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn lost_response_replay_does_not_reapply() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let receipt = encoded(&pending());
        let applied = encoded(&applied());
        assert_eq!(reserve(&db, &receipt).await, 0);
        // Discard the application reply, then reconcile through a new connection.
        let _ = commit(&db, &applied, "first-apply").await;
        let reconnected = independent_db(&client).await;
        assert_eq!(reserve(&reconnected, &receipt).await, 1);
        assert_eq!(commit(&reconnected, &applied, "retry").await, 1);
        assert_eq!(reconnected.get(OPERATION).await.unwrap(), Some(applied));
        assert_eq!(reconnected.get_bin(CONTAINER).await.unwrap(), CANDIDATE);
        assert_eq!(
            reconnected.hget(CURRENT, "phash").await.unwrap(),
            Some(pending().candidate_digest)
        );
        assert_eq!(
            reconnected.hget(CURRENT, "vinst").await.unwrap().as_deref(),
            Some("8")
        );
        assert_eq!(
            reconnected.hget(CURRENT, "ctime").await.unwrap().as_deref(),
            Some("first-apply")
        );
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn competing_finalizers_compare_exact_receipts() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let other = independent_db(&client).await;
        let pending = encoded(&pending());
        let applied_receipt = applied();
        let applied = encoded(&applied_receipt);
        let mut success_receipt = applied_receipt.clone();
        success_receipt.status = PolicyOperationStatus::Succeeded;
        let success = encoded(&success_receipt);
        let mut uncertain_receipt = applied_receipt;
        uncertain_receipt.status = PolicyOperationStatus::Uncertain;
        uncertain_receipt.error = Some("readback interrupted".into());
        let uncertain = encoded(&uncertain_receipt);
        assert_eq!(reserve(&db, &pending).await, 0);
        assert_eq!(commit(&db, &applied, "applied").await, 0);
        let (success_result, uncertain_result) = tokio::join!(
            db.finish_policy_operation(OPERATION, CURRENT, &applied, &success),
            other.finish_policy_operation(OPERATION, CURRENT, &applied, &uncertain)
        );
        let outcomes = (success_result.unwrap(), uncertain_result.unwrap());
        assert!(matches!(outcomes, (0, -4) | (-4, 0)));
        if outcomes.1 == 0 {
            assert_eq!(db.get(OPERATION).await.unwrap(), Some(uncertain.clone()));
            assert_eq!(
                db.finish_policy_operation(OPERATION, CURRENT, &uncertain, &success)
                    .await
                    .unwrap(),
                0
            );
        }
        assert_eq!(
            db.finish_policy_operation(OPERATION, CURRENT, &applied, &uncertain)
                .await
                .unwrap(),
            -4
        );
        assert_eq!(
            db.finish_policy_operation(OPERATION, CURRENT, &success, &success)
                .await
                .unwrap(),
            1
        );
        assert_eq!(db.get(OPERATION).await.unwrap(), Some(success));
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn success_finalizer_checks_current_policy_atomically() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let other = independent_db(&client).await;
        let pending = encoded(&pending());
        let applied_receipt = applied();
        let applied = encoded(&applied_receipt);
        let mut success_receipt = applied_receipt.clone();
        success_receipt.status = PolicyOperationStatus::Succeeded;
        let success = encoded(&success_receipt);
        assert_eq!(reserve(&db, &pending).await, 0);
        assert_eq!(commit(&db, &applied, "applied").await, 0);
        db.hset(CURRENT, "vinst", "9").await.unwrap();
        assert_eq!(
            db.finish_policy_operation(OPERATION, CURRENT, &applied, &success)
                .await
                .unwrap(),
            -2
        );
        assert_eq!(db.get(OPERATION).await.unwrap(), Some(applied.clone()));
        db.hset(CURRENT, "vinst", "8").await.unwrap();
        let mut next = applied_receipt;
        next.operation_id = "next".into();
        next.request_fingerprint = "next-fingerprint".into();
        next.expected_current_digest = next.candidate_digest.clone();
        next.expected_current_vinst = 8;
        next.candidate_digest = hex::encode(openssl::sha::sha256(b"next policy"));
        next.applied_digest = None;
        next.applied_vinst = None;
        let next_pending = encoded(&next);
        assert_eq!(
            db.begin_policy_operation(
                "policy:operations:next",
                CURRENT,
                &next.expected_current_digest,
                "8",
                &next_pending
            )
            .await
            .unwrap(),
            0
        );
        next.applied_digest = Some(next.candidate_digest.clone());
        next.applied_vinst = Some(9);
        let next_applied = encoded(&next);
        let (finished, committed) = tokio::join!(
            db.finish_policy_operation(OPERATION, CURRENT, &applied, &success),
            other.commit_policy_operation(PolicyCommit {
                operation_key: "policy:operations:next",
                current_key: CURRENT,
                container_key: "policies:next:container",
                expected_digest: &next.expected_current_digest,
                expected_vinst: "8",
                candidate_digest: &next.candidate_digest,
                candidate_bytes: b"next policy",
                next_vinst: "9",
                changed_at: "next",
                pending_receipt: &next_applied,
            })
        );
        assert_eq!(committed.unwrap(), 0);
        let finished = finished.unwrap();
        assert!(matches!(finished, 0 | -2));
        assert_eq!(
            db.get(OPERATION).await.unwrap(),
            Some(if finished == 0 { success } else { applied })
        );
        assert_eq!(
            db.hget(CURRENT, "phash").await.unwrap(),
            Some(next.candidate_digest)
        );
        assert_eq!(
            db.hget(CURRENT, "vinst").await.unwrap().as_deref(),
            Some("9")
        );
        server.stop().unwrap();
    }

    #[tokio::test]
    #[ignore = "requires redis-server on PATH; starts and reaps a private loopback child"]
    async fn proof_nonce_replay_and_approval_have_no_ttl() {
        let (mut server, db, client) = PrivateRedis::start().await;
        let other = independent_db(&client).await;
        let nonce = "policy:approval-nonces:test";
        let approval = "policy:approvals:test";
        let binding = r#"{"actor":"named-test-operator","action":"install"}"#;
        let (left, right) = tokio::join!(
            db.authorize_policy_operation(nonce, approval, binding),
            other.authorize_policy_operation(nonce, approval, binding)
        );
        assert!(matches!((left.unwrap(), right.unwrap()), (0, -1) | (-1, 0)));
        assert_eq!(
            db.authorize_policy_operation("nonce-conflict", approval, "other-binding")
                .await
                .unwrap(),
            -2
        );
        assert!(!db.exists("nonce-conflict").await.unwrap());
        assert_eq!(
            db.authorize_policy_operation("nonce-fresh", approval, binding)
                .await
                .unwrap(),
            0
        );
        let reconnected = independent_db(&client).await;
        assert_eq!(
            reconnected
                .authorize_policy_operation(nonce, "approval-other", binding)
                .await
                .unwrap(),
            -1
        );
        assert!(!db.exists("approval-other").await.unwrap());
        let mut connection = client.get_multiplexed_async_connection().await.unwrap();
        // Proof validity is checked by the receiver; these stored keys have no expiry.
        for key in [nonce, approval, "nonce-fresh"] {
            let ttl: i64 = redis::cmd("TTL")
                .arg(key)
                .query_async(&mut connection)
                .await
                .unwrap();
            assert_eq!(ttl, -1, "{key} must retain the current no-TTL behavior");
        }
        assert_eq!(db.get(nonce).await.unwrap().as_deref(), Some(approval));
        assert_eq!(db.get(approval).await.unwrap().as_deref(), Some(binding));
        server.stop().unwrap();
    }
}
