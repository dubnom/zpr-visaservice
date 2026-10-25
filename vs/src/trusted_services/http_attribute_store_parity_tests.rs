use super::super::factory::{build_services, trusted_service_definitions};
use super::*;
use crate::loaded_policy::LoadedPolicy;
use crate::test_helpers::make_trusted_service_policy;
use zpr::policy_types::PolicyContainerBytes;

fn definitions(mappings: &[&str]) -> Vec<super::super::factory::TrustedServiceDefinition> {
    let loaded = LoadedPolicy::from_container(
        PolicyContainerBytes::from(make_trusted_service_policy(
            "directory",
            "file",
            Some(600),
            mappings,
        )),
        &crate::config::POLICY_MIN_VERSION,
    )
    .unwrap();
    trusted_service_definitions(&loaded.policy()).unwrap()
}

async fn https(mappings: &[&str], attributes: &str) -> Result<Vec<Attribute>, ServiceError> {
    let body = format!(r#"{{"attributes":{attributes}}}"#);
    let wire = format!(
        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len(),
    ).into_bytes();
    super::tests::lookup_raw_verified_https_with_mappings(wire, mappings)
        .await
        .0
}

fn normalized(
    attrs: Vec<Attribute>,
    started: SystemTime,
    ttl: u64,
) -> BTreeMap<String, Vec<String>> {
    let now = SystemTime::now();
    let count = attrs.len();
    let values: BTreeMap<_, _> = attrs
        .into_iter()
        .map(|attr| {
            assert_eq!(attr.get_source(), "directory");
            assert!(attr.get_expires() >= started + Duration::from_secs(ttl));
            assert!(attr.get_expires() <= now + Duration::from_secs(ttl));
            (attr.get_key().to_string(), attr.get_value().to_vec())
        })
        .collect();
    assert_eq!(values.len(), count, "duplicate canonical attributes");
    values
}

async fn compare(mappings: &[&str], attributes: &str, expected: Option<&[(&str, &[&str])]>) {
    let dir = tempfile::tempdir().unwrap();
    let started = SystemTime::now();
    std::fs::write(
        dir.path().join("directory.json"),
        format!(r#"{{"user.sub":{{"alice":{attributes}}}}}"#),
    )
    .unwrap();
    let file = match build_services(&definitions(mappings), dir.path()) {
        Ok(stores) => {
            stores[0]
                .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
                .await
        }
        Err(error) => Err(error),
    };
    let rest = https(mappings, attributes).await;
    if let Some(expected) = expected {
        let expected: BTreeMap<String, Vec<String>> = expected
            .iter()
            .map(|(key, values)| {
                (
                    key.to_string(),
                    values.iter().map(|value| value.to_string()).collect(),
                )
            })
            .collect();
        assert_eq!(normalized(file.unwrap(), started, 600), expected);
        assert_eq!(normalized(rest.unwrap(), started, 3600), expected);
    } else {
        for (kind, result) in [("file", file), ("REST", rest)] {
            let Err(error) = result else {
                panic!("{kind} accepted invalid observation: {attributes}");
            };
            assert!(error.to_string().contains("directory"), "{kind}: {error}");
        }
    }
}

#[tokio::test]
async fn explicit_empty_single_value_is_not_missing() {
    compare(
        &["department -> user.department"],
        r#"{"department":[""]}"#,
        Some(&[("user.department", &[""])]),
    )
    .await;
}

#[tokio::test]
async fn zero_single_values_reject_the_entire_observation() {
    compare(
        &["department -> user.department", "badge -> user.badge"],
        r#"{"badge":["valid"],"department":[]}"#,
        None,
    )
    .await;
}

#[tokio::test]
async fn multiple_single_values_reject_in_either_order() {
    for attributes in [
        r#"{"department":["engineering","sales"]}"#,
        r#"{"department":["sales","engineering"]}"#,
    ] {
        compare(&["department -> user.department"], attributes, None).await;
    }
}

#[tokio::test]
async fn identical_repeated_single_values_still_violate_cardinality() {
    compare(
        &["department -> user.department"],
        r#"{"department":["engineering","engineering"]}"#,
        None,
    )
    .await;
}

#[tokio::test]
async fn empty_multi_value_attribute_is_distinct_from_omission() {
    compare(
        &["roles -> user.role{}"],
        r#"{"roles":[]}"#,
        Some(&[("user.role", &[])]),
    )
    .await;
    compare(&["roles -> user.role{}"], "{}", Some(&[])).await;
}

#[tokio::test]
async fn multi_values_preserve_order_duplicates_and_empty_strings() {
    compare(
        &["roles -> user.role{}"],
        r#"{"roles":["b","","a","b"]}"#,
        Some(&[("user.role", &["b", "", "a", "b"])]),
    )
    .await;
}

#[tokio::test]
async fn tag_presence_is_valueless_regardless_of_payload() {
    for attributes in [r#"{"cleared":[]}"#, r#"{"cleared":["ignored","payload"]}"#] {
        compare(
            &["cleared -> #user.cleared"],
            attributes,
            Some(&[("user.zpr.tag.cleared", &[])]),
        )
        .await;
    }
}

#[tokio::test]
async fn agreeing_aliases_collapse_without_duplicate_claims() {
    compare(
        &["department -> user.department", "dept -> user.department"],
        r#"{"department":["engineering"],"dept":["engineering"]}"#,
        Some(&[("user.department", &["engineering"])]),
    )
    .await;
}

#[tokio::test]
async fn conflicting_aliases_reject_otherwise_valid_claims() {
    for attributes in [
        r#"{"badge":["valid"],"department":["engineering"],"dept":["sales"]}"#,
        r#"{"badge":["valid"],"department":["sales"],"dept":["engineering"]}"#,
    ] {
        compare(
            &[
                "badge -> user.badge",
                "department -> user.department",
                "dept -> user.department",
            ],
            attributes,
            None,
        )
        .await;
    }
}

#[tokio::test]
async fn reordered_multi_aliases_remain_conflicts_not_implicit_sets() {
    compare(
        &["roles -> user.role{}", "groups -> user.role{}"],
        r#"{"roles":["a","b"],"groups":["b","a"]}"#,
        None,
    )
    .await;
}

#[tokio::test]
async fn field_names_and_unicode_values_round_trip_exactly() {
    compare(
        &["Department -> user.upper", "department -> user.lower"],
        r#"{"Department":["Alice\"\\\n\ud83d\ude80"],"department":["alice"]}"#,
        Some(&[
            ("user.upper", &["Alice\"\\\n\u{1f680}"]),
            ("user.lower", &["alice"]),
        ]),
    )
    .await;
}

#[tokio::test]
async fn unmapped_fields_do_not_acquire_canonical_cardinality() {
    compare(
        &["department -> user.department"],
        r#"{"department":["engineering"],"unknown":[],"other":["a","b"]}"#,
        Some(&[("user.department", &["engineering"])]),
    )
    .await;
}

#[tokio::test]
async fn invalid_value_shapes_are_rejected_even_on_unmapped_fields() {
    for value in ["null", "true", "1", "\"text\"", "{}", "[null]", "[1]"] {
        compare(
            &["department -> user.department"],
            &format!(r#"{{"department":["engineering"],"unknown":{value}}}"#),
            None,
        )
        .await;
    }
}

#[tokio::test]
async fn duplicate_decoded_names_and_unpaired_surrogates_are_never_ignored() {
    for attributes in [
        r#"{"department":["engineering"],"depart\u006dent":["engineering"]}"#,
        r#"{"unknown":[],"unknown":[]}"#,
        r#"{"unknown":["\ud800"]}"#,
        r#"{"unknown":["\udfff"]}"#,
    ] {
        compare(&["department -> user.department"], attributes, None).await;
    }
}

#[tokio::test]
async fn malformed_reload_preserves_valid_file_snapshot_and_repair_replaces_it() {
    let mappings = ["department -> user.department"];
    let dir = tempfile::tempdir().unwrap();
    let started = SystemTime::now();
    let path = dir.path().join("directory.json");
    std::fs::write(&path, r#"{"user.sub":{"alice":{"department":["old"]}}}"#).unwrap();
    let stores = build_services(&definitions(&mappings), dir.path()).unwrap();
    let store = &stores[0];
    let revision = store.current_revision();
    let identities = [("user.sub".into(), "alice".into())];
    for malformed in [
        r#"{"department":["new"],"department":["other"]}"#,
        r#"{"department":null}"#,
        r#"{"department":["\ud800"]}"#,
    ] {
        std::fs::write(&path, format!(r#"{{"user.sub":{{"alice":{malformed}}}}}"#)).unwrap();
        assert!(store.flush().await.is_err());
        assert_eq!(store.current_revision(), revision);
        assert_eq!(
            normalized(
                store.get_attributes_for_actor(&identities).await.unwrap(),
                started,
                600
            ),
            BTreeMap::from([("user.department".into(), vec!["old".into()])])
        );
        assert!(matches!(
            https(&mappings, malformed).await,
            Err(ServiceError::AttributesIndeterminate(_))
        ));
    }
    std::fs::write(
        &path,
        r#"{"user.sub":{"alice":{"department":["repaired"]}}}"#,
    )
    .unwrap();
    store.flush().await.unwrap();
    assert!(store.current_revision() > revision);
    let file = normalized(
        store.get_attributes_for_actor(&identities).await.unwrap(),
        started,
        600,
    );
    let rest = normalized(
        https(&mappings, r#"{"department":["repaired"]}"#)
            .await
            .unwrap(),
        started,
        3600,
    );
    assert_eq!(file, rest);
    assert_eq!(file["user.department"], ["repaired"]);
}
