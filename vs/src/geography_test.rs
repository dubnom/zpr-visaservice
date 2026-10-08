use crate::config::VSConfig;

#[test]
fn optional_node_coordinates_preserve_legacy_configuration() {
    let config: VSConfig = toml::from_str("[core]\n").unwrap();
    assert!(config.nodes.is_empty());
    let config: VSConfig = toml::from_str("[nodes.\"node.example\"]\n").unwrap();
    assert_eq!(config.nodes["node.example"].latitude, None);
    assert_eq!(config.nodes["node.example"].longitude, None);
}

#[test]
fn node_coordinates_accept_zero_and_boundary_values() {
    for (latitude, longitude) in [(0.0, 0.0), (-90.0, -180.0), (90.0, 180.0), (43.04, -87.91)] {
        let source =
            format!("[nodes.\"node.example\"]\nlatitude={latitude}\nlongitude={longitude}\n");
        let config: VSConfig = toml::from_str(&source).unwrap();
        assert_eq!(config.nodes["node.example"].latitude, Some(latitude));
        assert_eq!(config.nodes["node.example"].longitude, Some(longitude));
    }
}

#[test]
fn node_coordinates_reject_incomplete_invalid_and_unknown_properties() {
    for fields in [
        "latitude=1",
        "longitude=1",
        "latitude=91\nlongitude=1",
        "latitude=1\nlongitude=-181",
        "latitude=nan\nlongitude=1",
        "latitude=1\nlongitude=inf",
        "latitude=\"north\"\nlongitude=1",
        "latitude=1\nlongitude=1\nlocation=\"guessed\"",
    ] {
        let source = format!("[nodes.\"node.example\"]\n{fields}\n");
        assert!(toml::from_str::<VSConfig>(&source).is_err(), "{source}");
    }
}

#[tokio::test]
async fn node_coordinates_reach_admin_contract_without_changing_node_identity() {
    use crate::admin_service::build_node_record_brief;
    use crate::assembly::tests::new_assembly_for_tests;
    use crate::test_helpers::make_node_actor_defexp;

    let mut asm = new_assembly_for_tests(None).await;
    asm.config =
        toml::from_str("[nodes.\"node-located\"]\nlatitude=43.04\nlongitude=-87.91\n").unwrap();
    for (cn, addr, located) in [
        ("node-located", "fd5a:5052::10", true),
        ("node-legacy", "fd5a:5052::11", false),
    ] {
        let actor = make_node_actor_defexp(addr, cn, "[fd5a:5052::100]:1234");
        asm.actor_mgr.add_node(&actor, false).await.unwrap();
        let details = build_node_record_brief(&asm, actor).await.unwrap();
        let encoded = serde_json::to_value(&details).unwrap();
        if located {
            assert_eq!(details.latitude, Some(43.04));
            assert_eq!(details.longitude, Some(-87.91));
            assert_eq!(encoded["latitude"], 43.04);
            assert_eq!(encoded["longitude"], -87.91);
        } else {
            assert!(encoded.get("latitude").is_none());
            assert!(encoded.get("longitude").is_none());
        }
        let decoded: admin_api_types::NodeRecordBrief = serde_json::from_value(encoded).unwrap();
        assert_eq!(decoded.latitude, details.latitude);
        assert_eq!(decoded.longitude, details.longitude);
        assert_eq!(
            asm.actor_mgr
                .get_actor_by_zpr_addr(&addr.parse().unwrap())
                .await
                .unwrap()
                .unwrap()
                .get_cn(),
            Some(cn)
        );
    }
}
