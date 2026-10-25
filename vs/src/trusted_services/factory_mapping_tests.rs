use super::*;
use crate::loaded_policy::LoadedPolicy;
use crate::test_helpers::make_trusted_service_policy;
use zpr::policy_types::{PolicyContainerBytes, parse_attribute_mapping};

fn policy(api: &str, mappings: &[&str]) -> Arc<Policy> {
    LoadedPolicy::from_container(
        PolicyContainerBytes::from(make_trusted_service_policy(
            "directory",
            api,
            Some(600),
            mappings,
        )),
        &crate::config::POLICY_MIN_VERSION,
    )
    .unwrap()
    .policy()
}

fn assert_rejected(api: &str, mappings: &[&str], detail: &str) {
    let result = trusted_service_definitions(&policy(api, mappings));
    let Err(ServiceError::Param(message)) = result else {
        panic!("ambiguous mapping accepted: {api}/{mappings:?}: {result:?}");
    };
    assert!(
        message.contains("directory") && message.contains(detail),
        "{message}"
    );
}

#[test]
fn repeated_raw_name_cannot_choose_different_canonical_keys_by_order() {
    for api in ["file", "rest/1"] {
        for mappings in [
            [
                "department -> user.department",
                "department -> device.department",
            ],
            [
                "department -> device.department",
                "department -> user.department",
            ],
        ] {
            assert_rejected(api, &mappings, "department");
        }
    }
}

#[test]
fn repeated_raw_name_cannot_choose_cardinality_by_order() {
    for api in ["file", "rest/1"] {
        for mappings in [
            ["roles -> user.role", "roles -> user.role{}"],
            ["roles -> user.role{}", "roles -> user.role"],
        ] {
            assert_rejected(api, &mappings, "roles");
        }
    }
}

#[test]
fn canonical_aliases_cannot_disagree_on_cardinality() {
    for api in ["file", "rest/1"] {
        for mappings in [
            ["role -> user.role", "roles -> user.role{}"],
            ["roles -> user.role{}", "role -> user.role"],
        ] {
            assert_rejected(api, &mappings, "user.role");
        }
    }
}

#[test]
fn blank_wire_mapping_names_are_rejected_before_store_construction() {
    use zpr::policy_types::{JoinPolicy, PFlags, Service};
    use zpr::write_to::WriteTo;
    for api in ["file", "rest/1"] {
        for raw in ["", " ", "\t\r\n"] {
            let mut mapping = parse_attribute_mapping("name -> user.name").unwrap();
            mapping.service_attr_key = raw.into();
            let record = TrustedService {
                service_id: "directory".into(),
                expiration_seconds: 600,
                returns_attrs: vec![mapping],
                identity_attrs: vec![],
            };
            let join = JoinPolicy {
                conditions: vec![],
                flags: PFlags::default(),
                provides: Some(vec![Service {
                    id: "directory".into(),
                    endpoints: vec![],
                    kind: ServiceType::Trusted(api.into()),
                }]),
            };
            let mut message = capnp::message::Builder::new_default();
            {
                let mut policy = message.init_root::<zpr::policy::v1::policy::Builder>();
                join.write_to(&mut policy.reborrow().init_join_policies(1).get(0));
                record.write_to(&mut policy.reborrow().init_trusted_services(1).get(0));
            }
            let mut bytes = Vec::new();
            capnp::serialize::write_message(&mut bytes, &message).unwrap();
            let policy = Policy::new_from_policy_bytes(bytes::Bytes::from(bytes)).unwrap();
            assert!(
                matches!(
                    trusted_service_definitions(&policy),
                    Err(ServiceError::Param(_))
                ),
                "{api}: blank raw name {raw:?} accepted"
            );
        }
    }
}

async fn file_lookup(mappings: &[&str], body: &str) -> Vec<libeval::attribute::Attribute> {
    let dir = tempfile::tempdir().unwrap();
    std::fs::write(dir.path().join("directory.json"), body).unwrap();
    let stores = build_services(
        &trusted_service_definitions(&policy("file", mappings)).unwrap(),
        dir.path(),
    )
    .unwrap();
    stores[0]
        .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
        .await
        .unwrap()
}

#[tokio::test]
async fn identical_repeated_mapping_declarations_remain_compatible() {
    for api in ["file", "rest/1"] {
        assert_eq!(
            trusted_service_definitions(&policy(
                api,
                &[
                    "department -> user.department",
                    "department -> user.department",
                ]
            ))
            .unwrap()
            .len(),
            1
        );
    }
    let attrs = file_lookup(
        &[
            "department -> user.department",
            "department -> user.department",
        ],
        r#"{"user.sub":{"alice":{"department":["engineering"]}}}"#,
    )
    .await;
    assert_eq!(attrs.len(), 1);
    assert!(attrs[0].value_has("engineering"));
}

#[tokio::test]
async fn agreeing_distinct_aliases_remain_compatible() {
    for api in ["file", "rest/1"] {
        assert!(
            trusted_service_definitions(&policy(
                api,
                &["department -> user.department", "dept -> user.department",]
            ))
            .is_ok()
        );
    }
    let attrs = file_lookup(
        &["department -> user.department", "dept -> user.department"],
        r#"{"user.sub":{"alice":{"department":["engineering"],"dept":["engineering"]}}}"#,
    )
    .await;
    assert_eq!(attrs.len(), 1);
}

#[tokio::test]
async fn raw_field_names_remain_case_sensitive() {
    let attrs = file_lookup(
        &["Department -> user.upper", "department -> user.lower"],
        r#"{"user.sub":{"alice":{"Department":["upper"],"department":["lower"]}}}"#,
    )
    .await;
    assert_eq!(attrs.len(), 2);
    assert!(
        attrs
            .iter()
            .any(|attr| attr.get_key() == "user.upper" && attr.value_has("upper"))
    );
    assert!(
        attrs
            .iter()
            .any(|attr| attr.get_key() == "user.lower" && attr.value_has("lower"))
    );
}

#[tokio::test]
async fn same_shape_multi_aliases_and_tags_preserve_emission() {
    let attrs = file_lookup(&[
        "roles -> user.role{}", "groups -> user.role{}", "cleared -> #user.cleared",
    ], r#"{"user.sub":{"alice":{"roles":["reviewer","author"],"groups":["reviewer","author"],"cleared":[]}}}"#).await;
    assert_eq!(attrs.len(), 2);
    assert_eq!(
        attrs
            .iter()
            .find(|attr| attr.get_key() == "user.role")
            .unwrap()
            .get_value(),
        &["reviewer", "author"]
    );
    assert!(
        attrs
            .iter()
            .find(|attr| attr.get_key() == "user.zpr.tag.cleared")
            .unwrap()
            .get_value()
            .is_empty()
    );
}

#[test]
fn declaration_source_id_and_reserved_key_boundaries_remain_enforced() {
    for id in ["a".to_string(), "a".repeat(64), "corp-1_A".into()] {
        let loaded = LoadedPolicy::from_container(
            PolicyContainerBytes::from(make_trusted_service_policy(&id, "file", Some(600), &[])),
            &crate::config::POLICY_MIN_VERSION,
        )
        .unwrap();
        assert!(trusted_service_definitions(&loaded.policy()).is_ok());
    }
    for id in [
        "a".repeat(65),
        "a__source__b".into(),
        "../outside".into(),
        "caf\u{e9}".into(),
    ] {
        let loaded = LoadedPolicy::from_container(
            PolicyContainerBytes::from(make_trusted_service_policy(&id, "file", Some(600), &[])),
            &crate::config::POLICY_MIN_VERSION,
        )
        .unwrap();
        assert!(trusted_service_definitions(&loaded.policy()).is_err());
    }
    for mapping in [
        "field -> user.name__source__other",
        "field -> user.role__source__other{}",
    ] {
        assert_rejected("file", &[mapping], "reserved");
        assert_rejected("rest/1", &[mapping], "reserved");
    }
}

#[tokio::test]
async fn rejected_mapping_update_preserves_live_policy_container_revision_and_store() {
    use crate::db::{FakeDb, PolicyRepo};
    use crate::policy_mgr::PolicyMgr;
    use crate::test_helpers::FakeResolver;
    use crate::trusted_services::TrustedServicesMgr;

    let dir = tempfile::tempdir().unwrap();
    std::fs::write(
        dir.path().join("directory.json"),
        r#"{"user.sub":{"alice":{"department":["engineering"]}}}"#,
    )
    .unwrap();
    let good = make_trusted_service_policy(
        "directory",
        "file",
        Some(600),
        &["department -> user.department"],
    );
    let manager = Arc::new(TrustedServicesMgr::new());
    let policy_mgr = PolicyMgr::new_with_initial_policy(
        good.clone(),
        PolicyRepo::new(Arc::new(FakeDb::new())),
        Arc::new(FakeResolver::ip_only()),
        manager.clone(),
        dir.path().into(),
    )
    .await
    .unwrap();
    let address = "fd5a:5052::123".parse().unwrap();
    let revisions = manager.stale_sources_for_actor(&address);
    manager.record_revision(&address, "directory", revisions[0].1);
    for mappings in [
        vec![
            "department -> user.department",
            "department -> device.department",
        ],
        vec!["department -> user.department", "dept -> user.department{}"],
    ] {
        let bad = make_trusted_service_policy("directory", "file", Some(600), &mappings);
        assert!(
            policy_mgr
                .update_policy_from_container_bytes(bad)
                .await
                .is_err()
        );
        assert_eq!(policy_mgr.get_current_snapshot().vinst(), 1);
        assert_eq!(
            policy_mgr.get_current_container().as_bytes(),
            good.as_slice()
        );
        assert!(manager.stale_sources_for_actor(&address).is_empty());
        let results = manager
            .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
            .await;
        assert!(
            results[0]
                .1
                .as_ref()
                .unwrap()
                .iter()
                .any(
                    |attr| attr.get_key() == "user.department__source__directory"
                        && attr.value_has("engineering")
                )
        );
    }
}
