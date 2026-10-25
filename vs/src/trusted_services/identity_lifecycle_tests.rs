use super::*;
use std::collections::BTreeMap;
use std::sync::Arc;
use std::time::{Duration, SystemTime};
use zpr::policy_types::parse_attribute_mapping;

fn identity(source: &str, key: &str, values: &[&str], expired: bool) -> Attribute {
    AttributeSource::new(source)
        .builder(key)
        .expires(if expired {
            SystemTime::now() - Duration::from_secs(1)
        } else {
            SystemTime::now() + Duration::from_secs(600)
        })
        .values(values.iter().map(|value| value.to_string()))
}

#[test]
fn selection_uses_exact_keys_not_case_or_source_suffix_inference() {
    let attrs = [
        identity("issuer", "user.sub", &["lower"], false),
        identity("issuer", "User.Sub", &["upper"], false),
        identity("issuer", "user.sub__source__company", &["qualified"], false),
    ];
    assert_eq!(
        lookup_identities(&["user.sub"], attrs.iter()).unwrap(),
        [("user.sub".into(), "lower".into())]
    );
    assert_eq!(
        lookup_identities(&["user.sub__source__company"], attrs.iter()).unwrap(),
        [("user.sub__source__company".into(), "qualified".into())]
    );
}

#[test]
fn selected_identity_text_is_not_trimmed_casefolded_or_unicode_normalized() {
    for value in [
        "",
        " Alice ",
        "ALICE",
        "caf\u{e9}",
        "cafe\u{301}",
        "quote\"\\\n\u{1f680}",
        "\\ud800",
    ] {
        let attrs = [identity("issuer", "user.sub", &[value], false)];
        assert_eq!(
            lookup_identities(&["user.sub"], attrs.iter()).unwrap(),
            [("user.sub".into(), value.into())]
        );
    }
}

#[test]
fn zero_and_multiple_value_identity_claims_are_skipped_even_if_expired() {
    for expired in [false, true] {
        for values in [vec![], vec!["a", "b"], vec!["a", "a"]] {
            let attrs = [identity("issuer", "user.sub", &values, expired)];
            assert!(
                lookup_identities(&["user.sub"], attrs.iter())
                    .unwrap()
                    .is_empty()
            );
        }
    }
}

#[test]
fn expired_selected_scalar_aborts_entire_identity_set_in_either_order() {
    let mut attrs = [
        identity("live-issuer", "device.subject", &["live"], false),
        identity("expired-issuer", "user.sub", &["expired"], true),
    ];
    for _ in 0..2 {
        match lookup_identities(&["device.subject", "user.sub"], attrs.iter()) {
            Err(ServiceError::AttributesIndeterminate(message)) => {
                assert!(message.contains("user.sub") && message.contains("expired-issuer"))
            }
            result => panic!("expired identity accepted: {result:?}"),
        }
        attrs.reverse();
    }
}

#[test]
fn unselected_expired_claims_cannot_poison_valid_selection() {
    let attrs = [
        identity("issuer", "user.sub", &["live"], false),
        identity("old", "user.unselected", &["expired"], true),
    ];
    assert_eq!(
        lookup_identities(&["user.sub"], attrs.iter()).unwrap(),
        [("user.sub".into(), "live".into())]
    );
    assert!(lookup_identities(&[], attrs.iter()).unwrap().is_empty());
}

#[test]
fn repeated_policy_keys_do_not_duplicate_selected_identity() {
    let attrs = [identity("issuer", "user.sub", &["alice"], false)];
    assert_eq!(
        lookup_identities(&["user.sub", "user.sub"], attrs.iter()).unwrap(),
        [("user.sub".into(), "alice".into())]
    );
}

#[test]
fn issuer_provenance_is_not_invented_as_a_subject_link() {
    let attrs = [
        identity("issuer-a", "user.sub", &["same"], false),
        identity("issuer-b", "user.sub", &["same"], false),
    ];
    assert_eq!(
        lookup_identities(&["user.sub"], attrs.iter()).unwrap(),
        [
            ("user.sub".into(), "same".into()),
            ("user.sub".into(), "same".into()),
        ]
    );
}

fn store(dir: &std::path::Path, json: &str) -> Arc<file_attribute_store::FileAttributeStore> {
    let path = dir.join("directory.json");
    std::fs::write(&path, json).unwrap();
    Arc::new(
        file_attribute_store::FileAttributeStore::new(
            "directory".into(),
            attribute_mapper::AttributeMapper {
                mappings: [
                    "department -> user.department",
                    "dept -> user.department",
                    "badge -> user.badge",
                    "roles -> user.role{}",
                    "tag -> #user.cleared",
                ]
                .iter()
                .map(|mapping| parse_attribute_mapping(mapping).unwrap())
                .collect(),
            },
            Duration::from_secs(600),
            &path,
        )
        .unwrap(),
    )
}

async fn values(
    store: &dyn TrustedServiceInterface,
    ids: &[(&str, &str)],
) -> Result<BTreeMap<String, Vec<String>>, ServiceError> {
    let identities: Vec<_> = ids
        .iter()
        .map(|(key, value)| (key.to_string(), value.to_string()))
        .collect();
    let attrs = store.get_attributes_for_actor(&identities).await?;
    let count = attrs.len();
    let result: BTreeMap<_, _> = attrs
        .into_iter()
        .map(|attr| {
            assert_eq!(attr.get_source(), "directory");
            assert!(!attr.is_expired());
            (attr.get_key().to_string(), attr.get_value().to_vec())
        })
        .collect();
    assert_eq!(count, result.len());
    Ok(result)
}

#[tokio::test]
async fn same_subject_under_distinct_identity_keys_never_cross_matches() {
    let dir = tempfile::tempdir().unwrap();
    let store = store(
        dir.path(),
        r#"{
        "user.sub":{"same":{"department":["user"]}},
        "device.subject":{"same":{"department":["device"]}}
    }"#,
    );
    for (key, expected) in [("user.sub", "user"), ("device.subject", "device")] {
        assert_eq!(
            values(store.as_ref(), &[(key, "same")]).await.unwrap()["user.department"],
            [expected]
        );
    }
    assert!(
        values(store.as_ref(), &[("User.Sub", "same")])
            .await
            .unwrap()
            .is_empty()
    );
}

#[tokio::test]
async fn exact_subject_values_distinguish_case_space_empty_and_unicode_forms() {
    let dir = tempfile::tempdir().unwrap();
    let subjects = [
        "Alice",
        "alice",
        " alice ",
        "",
        "caf\u{e9}",
        "cafe\u{301}",
        "\\ud800",
    ];
    let records: BTreeMap<_, _> = subjects
        .iter()
        .map(|subject| (*subject, serde_json::json!({"department": [subject]})))
        .collect();
    let store = store(
        dir.path(),
        &serde_json::json!({"user.sub": records}).to_string(),
    );
    for subject in subjects {
        assert_eq!(
            values(store.as_ref(), &[("user.sub", subject)])
                .await
                .unwrap()["user.department"],
            [subject]
        );
    }
    assert!(
        values(store.as_ref(), &[("user.sub", "ALICE")])
            .await
            .unwrap()
            .is_empty()
    );
}

fn permutations() -> [[usize; 3]; 6] {
    [
        [0, 1, 2],
        [0, 2, 1],
        [1, 0, 2],
        [1, 2, 0],
        [2, 0, 1],
        [2, 1, 0],
    ]
}

#[tokio::test]
async fn disjoint_multi_identity_union_is_invariant_under_every_order() {
    let dir = tempfile::tempdir().unwrap();
    let store = store(
        dir.path(),
        r#"{"user.sub":{
        "a":{"department":["engineering"]},"b":{"roles":["reviewer","author"]},"c":{"tag":[]}
    }}"#,
    );
    let ids = [("user.sub", "a"), ("user.sub", "b"), ("user.sub", "c")];
    let expected = BTreeMap::from([
        ("user.department".into(), vec!["engineering".into()]),
        ("user.role".into(), vec!["reviewer".into(), "author".into()]),
        ("user.zpr.tag.cleared".into(), vec![]),
    ]);
    for order in permutations() {
        assert_eq!(
            values(store.as_ref(), &order.map(|index| ids[index]))
                .await
                .unwrap(),
            expected
        );
    }
}

#[tokio::test]
async fn multi_identity_conflict_rejects_every_order_without_partial_union() {
    let dir = tempfile::tempdir().unwrap();
    let store = store(
        dir.path(),
        r#"{"user.sub":{
        "a":{"department":["engineering"]},"b":{"dept":["sales"]},"c":{"badge":["valid"]}
    }}"#,
    );
    let ids = [("user.sub", "a"), ("user.sub", "b"), ("user.sub", "c")];
    for order in permutations() {
        let error = values(store.as_ref(), &order.map(|index| ids[index]))
            .await
            .unwrap_err();
        assert!(matches!(error, ServiceError::Param(_)));
        assert!(error.to_string().contains("directory"));
    }
}

#[tokio::test]
async fn duplicate_identities_and_agreeing_aliases_collapse_canonical_claims() {
    let dir = tempfile::tempdir().unwrap();
    let store = store(
        dir.path(),
        r#"{"user.sub":{
        "a":{"department":["engineering"]},"b":{"dept":["engineering"]}
    }}"#,
    );
    let expected = BTreeMap::from([("user.department".into(), vec!["engineering".into()])]);
    for ids in [
        vec![("user.sub", "a"), ("user.sub", "a")],
        vec![("user.sub", "a"), ("user.sub", "b"), ("user.sub", "a")],
        vec![("user.sub", "b"), ("user.sub", "a")],
    ] {
        assert_eq!(values(store.as_ref(), &ids).await.unwrap(), expected);
    }
}

#[tokio::test]
async fn unmatched_semantic_cardinality_errors_do_not_poison_other_subjects() {
    let dir = tempfile::tempdir().unwrap();
    let store = store(
        dir.path(),
        r#"{"user.sub":{
        "bad":{"department":[]},"good":{"department":["engineering"]}
    }}"#,
    );
    assert_eq!(
        values(
            store.as_ref(),
            &[("user.sub", "unknown"), ("user.sub", "good")]
        )
        .await
        .unwrap()["user.department"],
        ["engineering"]
    );
    assert!(values(store.as_ref(), &[]).await.unwrap().is_empty());
}

#[tokio::test]
async fn matched_malformed_identity_rejects_union_with_valid_sibling() {
    let dir = tempfile::tempdir().unwrap();
    for malformed in ["[]", r#"["a","b"]"#, r#"["a","a"]"#] {
        let store = store(
            dir.path(),
            &format!(
                r#"{{"user.sub":{{
            "bad":{{"department":{malformed}}},"good":{{"badge":["valid"]}}
        }}}}"#
            ),
        );
        for ids in [
            [("user.sub", "bad"), ("user.sub", "good")],
            [("user.sub", "good"), ("user.sub", "bad")],
        ] {
            assert!(matches!(
                values(store.as_ref(), &ids).await,
                Err(ServiceError::AttributesIndeterminate(_))
            ));
        }
    }
}

#[tokio::test]
async fn conflict_repair_via_flush_recovers_without_changing_identity_set() {
    let dir = tempfile::tempdir().unwrap();
    let store = store(
        dir.path(),
        r#"{"user.sub":{
        "a":{"department":["engineering"]},"b":{"dept":["sales"]}
    }}"#,
    );
    let ids = [("user.sub", "a"), ("user.sub", "b")];
    let revision = store.current_revision();
    assert!(values(store.as_ref(), &ids).await.is_err());
    assert_eq!(store.current_revision(), revision);
    std::fs::write(
        dir.path().join("directory.json"),
        r#"{"user.sub":{
        "a":{"department":["engineering"]},"b":{"dept":["engineering"],"badge":["cleared"]}
    }}"#,
    )
    .unwrap();
    store.flush().await.unwrap();
    assert!(store.current_revision() > revision);
    assert_eq!(
        values(store.as_ref(), &ids).await.unwrap(),
        BTreeMap::from([
            ("user.department".into(), vec!["engineering".into()]),
            ("user.badge".into(), vec!["cleared".into()]),
        ])
    );
}
