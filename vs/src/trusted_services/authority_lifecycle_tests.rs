use super::*;
use libeval::actor::Actor;
use std::collections::BTreeMap;
use std::time::{Duration, SystemTime};

fn expires(seconds: u64) -> SystemTime {
    SystemTime::UNIX_EPOCH + Duration::from_secs(2_000_000_000 + seconds)
}

fn claim(key: &str, seconds: u64) -> Attribute {
    AttributeSource::new("company")
        .builder(key)
        .expires(expires(seconds))
        .value("claim")
}

fn derived(attrs: &[Attribute]) -> BTreeMap<String, Attribute> {
    let authorities = derive_user_authority_attributes("company", attrs);
    let count = authorities.len();
    let map: BTreeMap<_, _> = authorities
        .into_iter()
        .map(|authority| {
            assert_eq!(authority.get_source(), "company");
            assert_eq!(authority.get_value(), &["company"]);
            (authority.get_key().to_string(), authority)
        })
        .collect();
    assert_eq!(map.len(), count);
    map
}

#[test]
fn qualified_only_observation_never_manufactures_legacy_authority() {
    let map = derived(&[claim("user.department__source__company", 600)]);
    assert_eq!(map.len(), 1);
    assert_eq!(
        map["user.zpr.authority__source__company"].get_expires(),
        expires(600)
    );
}

#[test]
fn legacy_only_observation_never_manufactures_qualified_authority() {
    let map = derived(&[claim("user.department", 600)]);
    assert_eq!(map.len(), 1);
    assert_eq!(map[key::USER_AUTHORITY].get_expires(), expires(600));
}

#[test]
fn mixed_aliases_share_earliest_supporting_user_deadline() {
    for legacy_first in [true, false] {
        let mut attrs = vec![
            claim("user.department", 600),
            claim("user.department__source__company", 120),
        ];
        if !legacy_first {
            attrs.reverse();
        }
        let map = derived(&attrs);
        assert_eq!(map.len(), 2);
        assert!(map.values().all(|attr| attr.get_expires() == expires(120)));
    }
}

#[test]
fn shorter_device_claim_does_not_cap_derived_user_authority() {
    let map = derived(&[claim("device.location", 1), claim("user.department", 600)]);
    assert_eq!(map[key::USER_AUTHORITY].get_expires(), expires(600));
}

#[test]
fn freshness_only_observation_is_not_a_user_record() {
    assert!(
        derived(&[
            claim(SOURCE_FRESHNESS_KEY, 120),
            claim("zpr.source_freshness__source__company", 120)
        ])
        .is_empty()
    );
}

#[test]
fn qualified_peer_user_claim_does_not_create_or_shorten_current_authority() {
    assert!(derived(&[claim("user.department__source__lab", 1)]).is_empty());
    let map = derived(&[
        claim("user.department__source__lab", 1),
        claim("user.department__source__company", 600),
    ]);
    assert_eq!(map.len(), 1);
    assert_eq!(
        map["user.zpr.authority__source__company"].get_expires(),
        expires(600)
    );
}

#[test]
fn similar_user_prefixes_and_source_suffixes_are_not_support() {
    assert!(
        derived(&[
            claim("username", 1),
            claim("users.department", 1),
            claim("user.department__source__company_extra", 1),
            claim("user.department__source__Company", 1),
        ])
        .is_empty()
    );
}

#[test]
fn explicit_legacy_authority_suppresses_derivation_without_rewriting_it() {
    let authority = AttributeSource::new("company")
        .builder(key::USER_AUTHORITY)
        .expires(expires(900))
        .value("custom-principal");
    let attrs = [
        authority,
        claim("user.department", 120),
        claim("user.department__source__company", 120),
    ];
    assert!(derived(&attrs).is_empty());
    assert_eq!(attrs[0].get_value(), &["custom-principal"]);
    assert_eq!(attrs[0].get_expires(), expires(900));
}

#[test]
fn explicit_qualified_authority_suppresses_derivation_without_rewriting_it() {
    let attrs = [
        claim("user.zpr.authority__source__company", 900),
        claim("user.department__source__company", 120),
    ];
    assert!(derived(&attrs).is_empty());
    assert_eq!(attrs[0].get_expires(), expires(900));
}

#[test]
fn expired_user_support_never_produces_fresh_derived_authority() {
    let expired = SystemTime::now() - Duration::from_secs(1);
    let attr = AttributeSource::new("company")
        .builder("user.department")
        .expires(expired)
        .value("old");
    let map = derived(&[attr]);
    assert_eq!(map[key::USER_AUTHORITY].get_expires(), expired);
    assert!(map[key::USER_AUTHORITY].is_expired());
}

async fn file_observation(
    mappings: &[&str],
    raw: &str,
    multiple: bool,
) -> (Vec<Attribute>, Vec<Attribute>) {
    use crate::loaded_policy::LoadedPolicy;
    use crate::test_helpers::make_trusted_service_policy;
    use zpr::policy_types::PolicyContainerBytes;
    let dir = tempfile::tempdir().unwrap();
    let mut stores = Vec::new();
    for source in if multiple {
        vec!["company", "lab"]
    } else {
        vec!["company"]
    } {
        let loaded = LoadedPolicy::from_container(
            PolicyContainerBytes::from(make_trusted_service_policy(
                source,
                "file",
                Some(600),
                mappings,
            )),
            &crate::config::POLICY_MIN_VERSION,
        )
        .unwrap();
        std::fs::write(
            dir.path().join(format!("{source}.json")),
            format!(r#"{{"user.sub":{{"alice":{raw}}}}}"#),
        )
        .unwrap();
        stores.extend(
            factory::build_services(
                &factory::trusted_service_definitions(&loaded.policy()).unwrap(),
                dir.path(),
            )
            .unwrap(),
        );
    }
    let manager = TrustedServicesMgr::new();
    manager.update_services(stores);
    let mut claims = Vec::new();
    let mut authorities = Vec::new();
    for (source, result) in manager
        .get_attributes_for_actor(&[("user.sub".into(), "alice".into())])
        .await
    {
        let attrs = result.unwrap();
        let derived = derive_user_authority_attributes(&source, &attrs);
        assert!(
            derived
                .iter()
                .all(|authority| authority.get_source() == source
                    && authority.get_value() == &[source.clone()])
        );
        claims.extend(attrs);
        authorities.extend(derived);
    }
    (claims, authorities)
}

#[tokio::test]
async fn single_source_real_store_derives_both_authority_aliases() {
    let (claims, authorities) = file_observation(
        &["department -> user.department"],
        r#"{"department":["engineering"]}"#,
        false,
    )
    .await;
    assert_eq!(claims.len(), 2);
    assert_eq!(authorities.len(), 2);
    assert!(
        authorities
            .iter()
            .all(|authority| authority.get_expires() == claims[0].get_expires())
    );
    let keys: Vec<_> = authorities.iter().map(Attribute::get_key).collect();
    assert!(keys.contains(&key::USER_AUTHORITY));
    assert!(keys.contains(&"user.zpr.authority__source__company"));
}

#[tokio::test]
async fn two_real_sources_derive_separate_authorities_without_legacy_aliases() {
    let (claims, authorities) = file_observation(
        &["department -> user.department"],
        r#"{"department":["engineering"]}"#,
        true,
    )
    .await;
    assert_eq!(authorities.len(), 2);
    for source in ["company", "lab"] {
        let authority = authorities
            .iter()
            .find(|attr| attr.get_source() == source)
            .unwrap();
        assert_eq!(
            authority.get_key(),
            format!("user.zpr.authority__source__{source}")
        );
        let support = claims
            .iter()
            .find(|attr| attr.get_source() == source)
            .unwrap();
        assert_eq!(authority.get_expires(), support.get_expires());
    }
}

#[tokio::test]
async fn real_user_tags_and_empty_multivalues_support_authority_but_device_tags_do_not() {
    for mappings in [vec!["field -> #user.cleared"], vec!["field -> user.role{}"]] {
        for multiple in [false, true] {
            let (claims, authorities) =
                file_observation(&mappings, r#"{"field":[]}"#, multiple).await;
            assert_eq!(authorities.len(), 2);
            assert!(claims.iter().all(|attr| attr.get_value().is_empty()));
            for authority in authorities {
                let support = claims
                    .iter()
                    .find(|claim| claim.get_source() == authority.get_source())
                    .unwrap();
                assert_eq!(authority.get_expires(), support.get_expires());
            }
        }
    }
    for multiple in [false, true] {
        let (_, authorities) =
            file_observation(&["field -> #device.cleared"], r#"{"field":[]}"#, multiple).await;
        assert!(authorities.is_empty());
        let (_, authorities) = file_observation(&["field -> user.role{}"], "{}", multiple).await;
        assert!(authorities.is_empty());
    }
}

#[tokio::test]
async fn explicit_authority_from_real_store_is_preserved_in_both_source_modes() {
    for multiple in [false, true] {
        let (claims, authorities) = file_observation(
            &[
                "issuer -> user.zpr.authority",
                "department -> user.department",
            ],
            r#"{"issuer":["custom-authority"],"department":["engineering"]}"#,
            multiple,
        )
        .await;
        assert!(authorities.is_empty());
        let explicit: Vec<_> = claims
            .iter()
            .filter(|attr| attr.get_key().starts_with(key::USER_AUTHORITY))
            .collect();
        assert_eq!(explicit.len(), 2);
        assert!(
            explicit
                .iter()
                .all(|attr| attr.value_has("custom-authority"))
        );
    }
}

#[test]
fn derived_authority_bounds_authentication_and_visa_even_without_condition_reference() {
    use crate::test_helpers::make_policy_with_com_conditions;
    use libeval::eval_result::{Direction, Hit};
    let expires = SystemTime::now() + Duration::from_secs(120);
    let support = AttributeSource::new("company")
        .builder("user.department")
        .expires(expires)
        .value("engineering");
    let mut actor = Actor::new();
    actor.add_attribute(support.clone()).unwrap();
    for authority in derive_user_authority_attributes("company", &[support]) {
        actor.add_attribute(authority).unwrap();
    }
    assert_eq!(actor.get_authentication_expiration(), Some(expires));
    let policy = make_policy_with_com_conditions(&[], &[]);
    for direction in [Direction::Forward, Direction::Reverse] {
        for user_source in [true, false] {
            let other = Actor::new();
            let (src, dst) = if user_source {
                (&actor, &other)
            } else {
                (&other, &actor)
            };
            assert_eq!(
                crate::visareq_worker::compute_expiration(
                    src,
                    dst,
                    &policy,
                    &Hit::new_no_signal(0, direction)
                )
                .unwrap(),
                expires
            );
        }
    }
}
