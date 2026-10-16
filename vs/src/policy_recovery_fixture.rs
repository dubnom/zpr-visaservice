// Coherent test assembly backed entirely by the owned RedisDb, not reconstructed FakeDb.
async fn seed_persistent_policy_crash_window(dir: &std::path::Path, mode: &str) {
    use crate::db::{PolicyOperationRequest, PolicyOperationStatus, PolicyRepo};

    assert!(matches!(
        mode,
        "pending-before-commit" | "committed-pending-finalization"
    ));
    let (asm, _events) = persistent_policy_interop_assembly(dir).await;
    let repo = PolicyRepo::new(asm.state_db.clone());
    let snapshot = asm.policy_mgr.get_current_snapshot();
    let initial_digest = hex::encode(openssl::sha::sha256(snapshot.container().as_bytes()));
    let initial_vinst = snapshot.vinst();
    let bundle = make_valid_policy_bundle();
    let candidate = bundle.decode().unwrap();
    let operation = PolicyOperationRequest {
        operation_id: if mode == "pending-before-commit" {
            "crash-window-pending-before-commit".into()
        } else {
            "crash-window-committed-pending-finalization".into()
        },
        target_id: "owned-rust-router".into(),
        source_revision: 3,
        candidate_digest: hex::encode(openssl::sha::sha256(&candidate)),
        expected_current_digest: initial_digest.clone(),
        expected_current_vinst: initial_vinst,
    };
    let receipt = repo.begin_operation(&operation).await.unwrap();
    assert_eq!(receipt.status, PolicyOperationStatus::Pending);
    assert_eq!(receipt.applied_digest, None);
    assert_eq!(receipt.applied_vinst, None);

    let seeded_receipt = if mode == "committed-pending-finalization" {
        let receipt = repo.commit_operation(&operation, &candidate).await.unwrap();
        assert_eq!(receipt.status, PolicyOperationStatus::Pending);
        assert_eq!(
            receipt.applied_digest.as_deref(),
            Some(operation.candidate_digest.as_str())
        );
        assert_eq!(receipt.applied_vinst, Some(initial_vinst + 1));
        receipt
    } else {
        receipt
    };
    let manifest = serde_json::json!({
        "mode": mode,
        "operation_id": operation.operation_id,
        "target_id": operation.target_id,
        "source_revision": operation.source_revision,
        "candidate_digest": operation.candidate_digest,
        "expected_current_digest": operation.expected_current_digest,
        "expected_current_vinst": operation.expected_current_vinst,
        "request_fingerprint": seeded_receipt.request_fingerprint,
        "seed_status": seeded_receipt.status,
        "seed_applied_digest": seeded_receipt.applied_digest,
        "seed_applied_vinst": seeded_receipt.applied_vinst,
        "initial_digest": initial_digest,
        "initial_vinst": initial_vinst,
    });
    std::fs::write(
        dir.join("crash-window-seed.json"),
        serde_json::to_vec(&manifest).unwrap(),
    )
    .unwrap();
}

#[tokio::test]
#[ignore = "requires the Go-owned private RedisDb directory and inherited loopback listener"]
async fn policy_apply_rust_crash_window_fixture() {
    use base64::{Engine, engine::general_purpose::STANDARD};
    use std::os::fd::FromRawFd;
    use std::time::Duration;

    let dir = std::path::PathBuf::from(
        std::env::var_os("ZPR_POLICY_INTEROP_DIR").expect("Go-owned fixture directory required"),
    );
    let trust: crate::config::GuardedPolicyConfig =
        serde_json::from_slice(&std::fs::read(dir.join("trust.json")).unwrap()).unwrap();
    validate_policy_approval_config(&trust).unwrap();
    assert!(dir.join("redis-address").exists());
    let seed_mode = std::env::var("ZPR_POLICY_CRASH_WINDOW_SEED").ok();
    let (mut asm, mut events) = persistent_policy_interop_assembly(&dir).await;
    if let Some(mode) = seed_mode.as_deref() {
        drop(asm);
        drop(events);
        seed_persistent_policy_crash_window(&dir, mode).await;
        (asm, events) = persistent_policy_interop_assembly(&dir).await;
    }
    asm.config.guarded_policy = Some(trust);
    let asm = Arc::new(asm);
    let write_key = setup_test_api_rw_key(&asm);
    let bundle = make_valid_policy_bundle();
    let candidate = bundle.decode().unwrap();
    let current = asm.policy_mgr.get_current_snapshot();
    let ready = serde_json::json!({
        "write_key": write_key,
        "bundle": bundle,
        "container": STANDARD.encode(&candidate),
        "initial_digest": hex::encode(openssl::sha::sha256(current.container().as_bytes())),
        "initial_vinst": current.vinst(),
        "receiver_pid": std::process::id(),
        "persistent_redis": true,
    });
    let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(asm))));
    let tls = rustls_tls_acceptor(&dir.join("server.key"), &dir.join("server.pem"));
    let socket = unsafe { std::net::TcpListener::from_raw_fd(3) };
    assert!(socket.local_addr().unwrap().ip().is_loopback());
    socket.set_nonblocking(true).unwrap();
    let listener = TcpListener::from_std(socket).unwrap();
    let (stop_events, mut event_stop) = tokio::sync::oneshot::channel::<()>();
    let event_task = tokio::spawn(async move {
        let mut count = 0;
        loop {
            tokio::select! {
                event = events.recv() => {
                    if event.is_none() { return count; }
                    count += 1;
                }
                _ = &mut event_stop => { events.close(); break; }
            }
        }
        while events.recv().await.is_some() {
            count += 1;
        }
        count
    });
    std::fs::write(
        dir.join("ready.pending"),
        serde_json::to_vec(&ready).unwrap(),
    )
    .unwrap();
    std::fs::rename(dir.join("ready.pending"), dir.join("ready.json")).unwrap();
    let mut connections = tokio::task::JoinSet::new();
    let deadline = tokio::time::sleep(Duration::from_secs(90));
    tokio::pin!(deadline);
    let mut poll_stop = tokio::time::interval(Duration::from_millis(20));
    loop {
        tokio::select! {
            _ = &mut deadline => panic!("Go crash-window owner exceeded fixture lifetime"),
            _ = poll_stop.tick() => {
                if dir.join("stop").exists() { break; }
            }
            Some(result) = connections.join_next(), if !connections.is_empty() => {
                result.unwrap();
            }
            accepted = listener.accept() => {
                let (socket, peer) = accepted.unwrap();
                assert!(peer.ip().is_loopback());
                let tls = tls.clone();
                let app = app.clone();
                connections.spawn(async move {
                    let Ok(stream) = tls.accept(socket).await else { return };
                    let service = hyper::service::service_fn(move |request: Request<Incoming>| {
                        let mut app = app.clone();
                        async move { app.call(request).await }
                    });
                    let _ = hyper_util::server::conn::auto::Builder::new(TokioExecutor::new())
                        .serve_connection_with_upgrades(TokioIo::new(stream), service)
                        .await;
                });
            }
        }
    }
    drop(listener);
    connections.abort_all();
    while let Some(result) = connections.join_next().await {
        assert!(result.is_ok() || result.unwrap_err().is_cancelled());
    }
    drop(app);
    stop_events.send(()).unwrap();
    let event_count = event_task.await.unwrap();
    assert!(
        event_count > 0,
        "no recovered policy application event was drained"
    );
    std::fs::write(dir.join("events.json"), event_count.to_string()).unwrap();
}

async fn persistent_policy_interop_assembly(
    dir: &std::path::Path,
) -> (
    Assembly,
    tokio::sync::mpsc::Receiver<crate::event_mgr::VsEvent>,
) {
    use crate::db::{ActorRepo, LinkRepo, NodeRepo, PolicyRepo, RedisDb, VisaRepo};
    use crate::policy_mgr::PolicyMgr;
    use crate::test_helpers::FakeResolver;
    use crate::trusted_services::TrustedServicesMgr;

    let address = std::fs::read_to_string(dir.join("redis-address")).unwrap();
    let socket: std::net::SocketAddr = address.parse().unwrap();
    assert_eq!(socket.ip(), std::net::Ipv4Addr::LOCALHOST);
    let client = redis::Client::open(format!("redis://{socket}/")).unwrap();
    let manager = client.get_connection_manager().await.unwrap();
    let db = Arc::new(RedisDb::new(manager));
    let repo = PolicyRepo::new(db.clone());
    let ts_mgr = Arc::new(TrustedServicesMgr::new());
    let resolver = Arc::new(FakeResolver::ip_only());
    let policy_mgr = if repo.get_current_identifier().await.unwrap().is_some() {
        PolicyMgr::new_from_state(repo, resolver, ts_mgr.clone(), dir.into())
            .await
            .unwrap()
    } else {
        PolicyMgr::new_with_initial_policy(
            crate::assembly::tests::make_policy("2024-01-01T00:00:00Z", 1, Some("meta")),
            repo,
            resolver,
            ts_mgr.clone(),
            dir.into(),
        )
        .await
        .unwrap()
    };
    let counters = Arc::new(crate::counters::Counters::default());
    let (event_tx, event_rx) = tokio::sync::mpsc::channel(100);
    let (vreq_chan, _vreq_rx) = tokio::sync::mpsc::channel(100);
    let visa_repo = VisaRepo::new(db.clone(), 1).await.unwrap();
    (
        Assembly {
            config: crate::config::VSConfig::default(),
            counters: counters.clone(),
            system_start_time: std::time::Instant::now(),
            cc: crate::connection_control::ConnectionControl::new("owned-interop".into()),
            policy_mgr,
            actor_mgr: Arc::new(crate::actor_mgr::ActorMgr::new(
                ActorRepo::new(db.clone()),
                NodeRepo::new(db.clone()),
                counters,
            )),
            state_db: db.clone(),
            vreq_chan,
            visa_mgr: crate::visa_mgr::VisaMgr::new(visa_repo),
            vss_mgr: crate::vss_mgr::VssMgr::new(),
            net_mgr: Arc::new(crate::net_mgr::NetMgr::new_v6().unwrap()),
            event_mgr: crate::event_mgr::EventMgr::new(event_tx),
            admin_api_keys: Arc::new(crate::admin_apikeys::ReloadableApiKeys::default()),
            topo_mgr: crate::topology_mgr::TopologyMgr::new(LinkRepo::new(db)),
            ts_mgr,
            deny_log: Default::default(),
        },
        event_rx,
    )
}
