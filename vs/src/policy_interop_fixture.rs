// Included only inside admin_service's cfg(test) module.
include!("policy_recovery_fixture.rs");

/// Opt-in, bounded HTTPS fixture for the Go policy client; never a production entrypoint.
#[tokio::test]
#[ignore = "requires the Go-owned private state directory and inherited loopback listener"]
async fn policy_apply_rust_interop_fixture() {
    use base64::{Engine, engine::general_purpose::STANDARD};
    use std::os::fd::FromRawFd;
    use std::time::Duration;

    let dir = std::path::PathBuf::from(
        std::env::var_os("ZPR_POLICY_INTEROP_DIR").expect("Go-owned fixture directory required"),
    );
    let trust: crate::config::GuardedPolicyConfig =
        serde_json::from_slice(&std::fs::read(dir.join("trust.json")).unwrap()).unwrap();
    validate_policy_approval_config(&trust).unwrap();
    let persistent = dir.join("redis-address").exists();
    let (mut asm, mut events) = if persistent {
        persistent_policy_interop_assembly(&dir).await
    } else {
        new_assembly_with_event_rx(None).await
    };
    asm.config.guarded_policy = Some(trust);
    let asm = Arc::new(asm);
    let mut keys = Vec::new();
    for (id, permission) in [(1, Permission::ReadWrite), (2, Permission::Read)] {
        let mut secret = [0u8; 32];
        openssl::rand::rand_bytes(&mut secret).unwrap();
        let key = ApiKey::new(id, secret);
        asm.admin_api_keys.insert_for_test(
            key.key_id_hex(),
            ApiKeyRecord {
                owner: "isolated-interop".into(),
                permission,
                status: KeyStatus::Active,
                created: "fixture".into(),
                secret_hash: key.secret_hash().unwrap(),
                description: "generated ephemeral fixture key".into(),
            },
        );
        keys.push(key.to_key_string());
    }
    let bundle = make_valid_policy_bundle();
    let candidate = bundle.decode().unwrap();
    let current = asm.policy_mgr.get_current_snapshot();
    let ready = serde_json::json!({
        "write_key": keys[0],
        "read_key": keys[1],
        "bundle": bundle,
        "container": STANDARD.encode(&candidate),
        "initial_digest": hex::encode(openssl::sha::sha256(current.container().as_bytes())),
        "initial_vinst": current.vinst(),
        "receiver_pid": std::process::id(),
        "persistent_redis": persistent,
    });
    let app = admin_app(Arc::new(tokio::sync::RwLock::new(AdminState::new(asm))));
    let tls = rustls_tls_acceptor(&dir.join("server.key"), &dir.join("server.pem"));
    // FD 3 is the already-bound listener inherited from the owning Go process.
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
            _ = &mut deadline => panic!("Go interop owner exceeded fixture lifetime"),
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
                let dir = dir.clone();
                connections.spawn(async move {
                    let Ok(stream) = tls.accept(socket).await else { return };
                    let service = hyper::service::service_fn(move |request: Request<Incoming>| {
                        let mut app = app.clone();
                        let dir = dir.clone();
                        async move {
                            let apply = request.method() == axum::http::Method::POST
                                && request.uri().path() == POLICY_OPERATION_ROUTE;
                            let response = app.call(request).await?;
                            // Suppress only a completed successful application reply, not packets.
                            if apply && response.status() == StatusCode::OK
                                && std::fs::remove_file(dir.join("suppress-next-reply")).is_ok()
                            {
                                Ok::<_, std::convert::Infallible>(
                                    StatusCode::SERVICE_UNAVAILABLE.into_response(),
                                )
                            } else {
                                Ok(response)
                            }
                        }
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
        "no actual policy application events drained"
    );
    std::fs::write(dir.join("events.json"), event_count.to_string()).unwrap();
}
