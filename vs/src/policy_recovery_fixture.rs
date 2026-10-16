// Coherent test assembly backed entirely by the owned RedisDb, not reconstructed FakeDb.
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
