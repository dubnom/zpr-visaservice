//! File-backed trusted-service attribute storage.

use arc_swap::ArcSwap;
use async_trait::async_trait;
use serde::Deserialize;
use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::{Duration, Instant};
use tracing::{debug, info};

use libeval::attribute::{Attribute, AttributeSource};

use crate::error::ServiceError;
use crate::logging::targets::TS;

use super::attribute_mapper::{AttrHint, AttributeMapper};
use super::{TrustedServiceInterface, next_revision};

/// Shortest TTL emitted on an attribute. A nearly stale snapshot refreshes early.
const MIN_ATTRIBUTE_TTL: Duration = Duration::from_secs(60);

/// File-based attributes encoded in JSON.
#[derive(Debug)]
pub(super) struct FileAttributeStore {
    id: String,
    mapper: AttributeMapper,
    ttl: Duration,
    fp: PathBuf,
    snapshot: ArcSwap<Snapshot>,
    /// Serializes concurrent reloads; reads remain lock-free via ArcSwap.
    refresh_lock: tokio::sync::Mutex<()>,
}

/// Attribute data, expiration, and revision swapped as one consistent unit.
#[derive(Debug)]
struct Snapshot {
    attributes: ActorAttributes,
    expires_at: Instant,
    revision: u64,
}

impl Snapshot {
    /// Return this snapshot's remaining validity, saturating at zero.
    fn remaining(&self) -> Duration {
        self.expires_at.saturating_duration_since(Instant::now())
    }
}

type IdentityKey = String;
type IdentityValue = String;
type AttributeName = String;
type AttributeValue = String;

/// One actor entry's attribute data: service attribute name -> values.
type AttributeMap = BTreeMap<AttributeName, Vec<AttributeValue>>;

/// Actor attributes decoded from the file's JSON representation.
///
/// Entries are keyed by identity attribute key AND value:
///
/// ```json
/// { "device.zpr.adapter.cn": { "alice.zpr.org": { "color": ["red"] } },
///   "user.sub":              { "10769150350006150715113082367": { "dept": ["eng"] } } }
/// ```
///
/// Keying on the (key, value) pair rather than a flat value map means a device CN and a
/// user subject can never collide in this file, and the store can tell which kind of
/// identity each entry describes.
#[derive(Debug, Clone)]
struct ActorAttributes(BTreeMap<IdentityKey, BTreeMap<IdentityValue, AttributeMap>>);

impl<'de> Deserialize<'de> for ActorAttributes {
    fn deserialize<D: serde::Deserializer<'de>>(deserializer: D) -> Result<Self, D::Error> {
        #[derive(Deserialize)]
        #[serde(transparent, bound(deserialize = "V: Deserialize<'de>"))]
        struct UniqueMap<V> {
            #[serde(deserialize_with = "super::deserialize_unique_map")]
            values: BTreeMap<String, V>,
        }

        let decoded = UniqueMap::<UniqueMap<UniqueMap<Vec<String>>>>::deserialize(deserializer)?;
        Ok(Self(
            decoded
                .values
                .into_iter()
                .map(|(key, subjects)| {
                    (
                        key,
                        subjects
                            .values
                            .into_iter()
                            .map(|(subject, attributes)| (subject, attributes.values))
                            .collect(),
                    )
                })
                .collect(),
        ))
    }
}

/// Load and decode actor attributes from a local JSON file.
fn load_actor_attributes_from_file(
    fp: &Path,
    source: &str,
) -> Result<ActorAttributes, ServiceError> {
    let contents = fs::read_to_string(fp).map_err(|error| {
        ServiceError::AttributesIndeterminate(format!(
            "{source}: failed to read attribute data file {fp:?}: {error}"
        ))
    })?;
    serde_json::from_str(&contents).map_err(|error| {
        ServiceError::AttributesIndeterminate(format!(
            "{source}: invalid attribute data file {fp:?}: {error}"
        ))
    })
}

impl FileAttributeStore {
    /// Create a file store and load its initial snapshot.
    pub(super) fn new(
        id: String,
        mapper: AttributeMapper,
        ttl: Duration,
        fp: &Path,
    ) -> Result<Self, ServiceError> {
        if ttl <= MIN_ATTRIBUTE_TTL {
            return Err(ServiceError::Param(format!(
                "trusted service '{id}' ttl {ttl:?} must exceed minimum of {MIN_ATTRIBUTE_TTL:?}"
            )));
        }

        let attributes = load_actor_attributes_from_file(fp, &id).map_err(|error| {
            ServiceError::TrustedServiceInit(format!(
                "TS '{id}' failed to read attribute data file {fp:?}: {error}"
            ))
        })?;

        let snapshot = Snapshot {
            attributes,
            expires_at: Instant::now() + ttl,
            revision: next_revision(),
        };
        Ok(Self {
            id,
            mapper,
            ttl,
            fp: fp.into(),
            snapshot: ArcSwap::from_pointee(snapshot),
            refresh_lock: tokio::sync::Mutex::new(()),
        })
    }

    /// Return a live snapshot, refreshing once when the cached data approaches expiry.
    async fn live_snapshot(&self) -> Result<Arc<Snapshot>, ServiceError> {
        let snapshot = self.snapshot.load_full();
        if snapshot.remaining() >= MIN_ATTRIBUTE_TTL {
            return Ok(snapshot);
        }

        let _guard = self.refresh_lock.lock().await;

        // Another caller may have refreshed while this caller waited.
        let snapshot = self.snapshot.load_full();
        if snapshot.remaining() >= MIN_ATTRIBUTE_TTL {
            return Ok(snapshot);
        }

        info!(target: TS, "TS {} re-loading actor attributes", self.id);
        let fresh = Arc::new(Snapshot {
            attributes: load_actor_attributes_from_file(&self.fp, &self.id)?,
            expires_at: Instant::now() + self.ttl,
            revision: next_revision(),
        });
        self.snapshot.store(fresh.clone());
        Ok(fresh)
    }
}

#[async_trait]
impl TrustedServiceInterface for FileAttributeStore {
    /// Look up every identity the actor presented and return the UNION of the mapped
    /// attributes of every entry that matched. Two entries supplying the same
    /// attribute with differing values is a store misconfiguration; the lookup fails
    /// (and the caller treats that as indeterminate) rather than pick a winner, since
    /// silently choosing would make authorization depend on identity ordering.
    ///
    /// Conflicts are detected on the MAPPED (canonical ZPR) key, not the raw
    /// service-side name: two different service-side names can map to the same ZPR
    /// key (e.g. `department -> user.dept` and `dept -> user.dept`), and comparing
    /// raw names would let such a pair smuggle conflicting values past the check --
    /// the caller's `Actor::add_attribute` would then silently keep whichever came
    /// last instead of failing closed as this interface promises.
    /// Single-valued mappings require exactly one value; malformed claims fail
    /// the whole lookup rather than choosing a value or manufacturing an empty one.
    async fn get_attributes_for_actor(
        &self,
        identities: &[(String, String)],
    ) -> Result<Vec<Attribute>, ServiceError> {
        let snapshot = self.live_snapshot().await?;

        // Union the attribute maps of every matched identity, keyed by the canonical
        // ZPR key so conflicts are detected after mapping collapses aliased
        // service-side names. Unmapped names are filtered out here; they can never
        // become attributes, so they cannot conflict. Value alongside: the values,
        // the emission hint, and (identity, raw name) provenance for the error path.
        struct Merged<'a> {
            values: &'a Vec<String>,
            hint: AttrHint,
            ident: &'a str,
            raw_name: &'a str,
        }
        let mut merged: BTreeMap<String, Merged> = BTreeMap::new();
        for (ident_key, ident_value) in identities {
            let Some(attributes) = snapshot
                .attributes
                .0
                .get(ident_key)
                .and_then(|by_value| by_value.get(ident_value))
            else {
                debug!(
                    target: TS,
                    "{}: identity {ident_key}={ident_value} not found in the attribute store",
                    self.id
                );
                continue;
            };
            for (name, values) in attributes {
                let Some((zpr_key, hint)) = self.mapper.map_attribute(name) else {
                    continue;
                };
                match merged.get(&zpr_key) {
                    None => {
                        merged.insert(
                            zpr_key,
                            Merged {
                                values,
                                hint,
                                ident: ident_value,
                                raw_name: name,
                            },
                        );
                    }
                    Some(first) if first.values != values => {
                        return Err(ServiceError::Param(format!(
                            "{}: '{}' (identity '{}') and '{name}' (identity '{ident_value}') \
                             disagree on attribute '{zpr_key}'",
                            self.id, first.raw_name, first.ident
                        )));
                    }
                    Some(_) => {} // Same ZPR key, same values: one attribute, no conflict.
                }
            }
        }

        let mut result = Vec::new();
        let src_builder = AttributeSource::new(self.id.clone());
        let remaining = snapshot.remaining();

        for (zpr_key, m) in merged {
            let builder = src_builder.builder(zpr_key.as_str()).expires_in(remaining);
            let attribute = match m.hint {
                AttrHint::SingleValued => {
                    let [value] = m.values.as_slice() else {
                        return Err(ServiceError::AttributesIndeterminate(format!(
                            "{}: expected one value for {}",
                            self.id, zpr_key
                        )));
                    };
                    builder.value(value.clone())
                }
                AttrHint::MultiValued => builder.values(m.values.clone()),
                // A tag is valueless: presence of the per-tag key is the tag.
                AttrHint::Tag => builder.values(Vec::<String>::new()),
            };
            result.push(attribute);
        }
        Ok(result)
    }

    /// Reload the file and atomically install a fresh snapshot.
    async fn flush(&self) -> Result<(), ServiceError> {
        info!(target: TS, "TS {} flushing cached actor attributes", self.id);
        let _guard = self.refresh_lock.lock().await;
        let attributes = load_actor_attributes_from_file(&self.fp, &self.id)?;
        self.snapshot.store(Arc::new(Snapshot {
            attributes,
            expires_at: Instant::now() + self.ttl,
            revision: next_revision(),
        }));
        Ok(())
    }

    /// Return the revision of the current file snapshot.
    fn current_revision(&self) -> u64 {
        self.snapshot.load().revision
    }

    /// Return the configured trusted-service identifier.
    fn get_source_id(&self) -> &str {
        &self.id
    }
}

#[cfg(test)]
#[path = "file_snapshot_lifecycle_tests.rs"]
mod snapshot_lifecycle_tests;

#[cfg(test)]
mod tests {
    use super::*;
    use crate::trusted_services::test_support::{test_mapper, write_fixture};
    use libeval::attribute::key;

    /// True when the snapshot holds an entry for `value` under the builtin CN
    /// identity key (the key the test fixtures use).
    fn has_cn_entry(snapshot: &Snapshot, value: &str) -> bool {
        snapshot
            .attributes
            .0
            .get(key::CN)
            .is_some_and(|by_value| by_value.contains_key(value))
    }

    #[test]
    fn duplicate_file_keys_are_rejected_at_every_level() {
        let dir = tempfile::tempdir().unwrap();
        let fp = dir.path().join("attributes.json");
        for json in [
            r#"{"user.sub":{"alice":{"color":["red"],"color":["blue"]}}}"#,
            r#"{"user.sub":{"alice":{"color":["red"],"color":["red"]}}}"#,
            r#"{"user.sub":{"alice":{"color":["red"],"co\u006cor":["blue"]}}}"#,
            r#"{"user.sub":{"alice":{"ignored":["a"],"ignored":["b"]}}}"#,
            r#"{"user.sub":{"alice":{"color":["red"]},"alice":{"color":["blue"]}}}"#,
            r#"{"user.sub":{"alice":{},"al\u0069ce":{}}}"#,
            r#"{"user.sub":{"alice":{}},"user.sub":{"bob":{}}}"#,
            r#"{"user.sub":{"alice":{}},"user.\u0073ub":{"bob":{}}}"#,
        ] {
            fs::write(&fp, json).unwrap();
            let result = FileAttributeStore::new(
                "directory".into(),
                test_mapper(),
                Duration::from_secs(600),
                &fp,
            );
            match result {
                Err(ServiceError::TrustedServiceInit(message)) => {
                    assert!(message.contains("directory"), "{message}");
                    assert!(message.contains("duplicate"), "{message}");
                }
                other => panic!("duplicate keys must fail initialization: {json}: {other:?}"),
            }
        }
    }

    #[tokio::test]
    async fn invalid_file_reload_is_indeterminate_preserves_revision_and_recovers() {
        let dir = tempfile::tempdir().unwrap();
        let fp = dir.path().join("attributes.json");
        let valid = r#"{"user.sub":{"alice":{"color":["red"]}}}"#;
        for invalid in [
            Some(r#"{"user.sub":{"alice":{"color":["red"],"color":["blue"]}}}"#),
            Some(r#"{"user.sub":{"alice":{"color":null}}}"#),
            Some(r#"{"user.sub":{"alice":{}}"#),
            None,
        ] {
            fs::write(&fp, valid).unwrap();
            let store = FileAttributeStore::new(
                "directory".into(),
                test_mapper(),
                Duration::from_secs(600),
                &fp,
            )
            .unwrap();
            let identities = vec![("user.sub".into(), "alice".into())];
            let revision = store.current_revision();
            let original = store.snapshot.load_full();
            if let Some(json) = invalid {
                fs::write(&fp, json).unwrap();
            } else {
                fs::remove_file(&fp).unwrap();
            }
            assert!(matches!(
                store.flush().await,
                Err(ServiceError::AttributesIndeterminate(_))
            ));
            assert_eq!(store.current_revision(), revision);
            assert!(Arc::ptr_eq(&original, &store.snapshot.load_full()));
            // A rejected explicit flush does not replace a still-valid snapshot.
            assert!(store.get_attributes_for_actor(&identities).await.unwrap()[0].value_has("red"));

            let old = store.snapshot.load_full();
            store.snapshot.store(Arc::new(Snapshot {
                attributes: old.attributes.clone(),
                expires_at: Instant::now() - Duration::from_secs(1),
                revision,
            }));
            let expired = store.snapshot.load_full();
            match store.get_attributes_for_actor(&identities).await {
                Err(ServiceError::AttributesIndeterminate(message)) => {
                    assert!(message.contains("directory"), "{message}");
                    assert!(message.contains("attributes.json"), "{message}");
                }
                other => panic!("expired invalid snapshot must not be served: {other:?}"),
            }
            assert_eq!(store.current_revision(), revision);
            assert!(Arc::ptr_eq(&expired, &store.snapshot.load_full()));
            fs::write(&fp, r#"{"user.sub":{"alice":{"color":["blue"]}}}"#).unwrap();
            let attrs = store.get_attributes_for_actor(&identities).await.unwrap();
            assert!(attrs[0].value_has("blue"));
            assert_ne!(store.current_revision(), revision);
        }
    }

    #[tokio::test]
    async fn malformed_file_claims_deny_requests_revoke_existing_visas_and_recover() {
        use crate::visa_reconciler::{SweepReason, revalidate_visas};
        use crate::visareq_worker::{VisaDecision, process_visa_request_for_test};
        use zpr::policy_types::parse_attribute_mapping;
        use zpr::vsapi_types::PacketDesc;

        for source_endpoint in [true, false] {
            for values in ["[]", r#"["false","true"]"#] {
                let (asm, node, _) =
                    crate::test_helpers::build_ttl_refresh_asm(source_endpoint).await;
                let dir = tempfile::tempdir().unwrap();
                let fp = dir.path().join("attributes.json");
                let valid = r#"{"device.zpr.adapter.cn":{"src":{"blocked":["false"]},"dst":{"blocked":["false"]}}}"#;
                fs::write(&fp, valid).unwrap();
                let store = Arc::new(
                    FileAttributeStore::new(
                        "hr".into(),
                        AttributeMapper {
                            mappings: vec![
                                parse_attribute_mapping("blocked -> user.blocked").unwrap(),
                            ],
                        },
                        Duration::from_secs(600),
                        &fp,
                    )
                    .unwrap(),
                );
                asm.ts_mgr.update_services(vec![store.clone()]);
                let packet = || {
                    PacketDesc::new_tcp("fd5a:5052:4000::a", "fd5a:5052:4000::b", 1234, 80).unwrap()
                };
                let VisaDecision::Allow(visa, _) =
                    process_visa_request_for_test(asm.clone(), node, packet()).await
                else {
                    panic!("valid file claims must allow");
                };
                let subject = if source_endpoint { "src" } else { "dst" };
                fs::write(
                    &fp,
                    format!(
                        r#"{{"device.zpr.adapter.cn":{{"{subject}":{{"blocked":{values}}}}}}}"#
                    ),
                )
                .unwrap();
                store.flush().await.unwrap();
                assert!(matches!(
                    process_visa_request_for_test(asm.clone(), node, packet()).await,
                    VisaDecision::Deny(_)
                ));
                let psnap = asm.policy_mgr.get_current_snapshot();
                assert!(revalidate_visas(&asm, &psnap, SweepReason::AttributeChange).await);
                assert_eq!(
                    asm.visa_mgr
                        .get_pending_revoke_visa_ids_for_node(&node)
                        .await
                        .unwrap(),
                    vec![visa.issuer_id]
                );
                fs::write(&fp, valid).unwrap();
                store.flush().await.unwrap();
                assert!(matches!(
                    process_visa_request_for_test(asm.clone(), node, packet()).await,
                    VisaDecision::Allow(_, _)
                ));
                assert!(
                    asm.visa_mgr
                        .get_pending_revoke_visa_ids_for_node(&node)
                        .await
                        .unwrap()
                        .contains(&visa.issuer_id)
                );
            }
        }
    }

    #[tokio::test]
    async fn single_valued_claims_reject_missing_or_multiple_values_and_recover() {
        let dir = tempfile::tempdir().unwrap();
        let fp = dir.path().join("attributes.json");
        let valid = r#"{"user.sub":{"alice":{"color":["red"],"roles":["a","b"],"lazy":[]}}}"#;
        fs::write(&fp, valid).unwrap();
        let store = FileAttributeStore::new(
            "directory".into(),
            test_mapper(),
            Duration::from_secs(3600),
            &fp,
        )
        .unwrap();
        let identities = vec![("user.sub".into(), "alice".into())];

        for values in [
            "[]",
            r#"["red","blue"]"#,
            r#"["blue","red"]"#,
            r#"["red","red"]"#,
        ] {
            fs::write(
                &fp,
                format!(r#"{{"user.sub":{{"alice":{{"color":{values},"roles":["a","b"]}}}}}}"#),
            )
            .unwrap();
            store.flush().await.unwrap();
            match store.get_attributes_for_actor(&identities).await {
                Err(ServiceError::AttributesIndeterminate(message)) => {
                    assert!(message.contains("directory"), "{message}");
                    assert!(message.contains("user.color"), "{message}");
                }
                other => panic!("invalid single-valued claim must fail: {values}: {other:?}"),
            }

            fs::write(&fp, valid).unwrap();
            store.flush().await.unwrap();
            let attrs = store.get_attributes_for_actor(&identities).await.unwrap();
            assert_eq!(attrs.len(), 3);
            assert!(
                attrs
                    .iter()
                    .any(|a| a.get_key() == "user.color" && a.value_has("red"))
            );
            let roles = attrs.iter().find(|a| a.get_key() == "user.role").unwrap();
            assert_eq!(roles.get_value(), vec!["a".to_string(), "b".to_string()]);
            let tag = attrs
                .iter()
                .find(|a| a.get_key() == "user.zpr.tag.lazy")
                .unwrap();
            assert!(tag.get_value().is_empty());
        }

        fs::write(
            &fp,
            r#"{"user.sub":{"alice":{"color":[""],"roles":[],"lazy":[]},"unmatched":{"color":["red","blue"]}}}"#,
        )
        .unwrap();
        store.flush().await.unwrap();
        let attrs = store.get_attributes_for_actor(&identities).await.unwrap();
        assert_eq!(attrs.len(), 3);
        let color = attrs.iter().find(|a| a.get_key() == "user.color").unwrap();
        assert_eq!(color.get_value(), vec![String::new()]);
        assert!(
            attrs
                .iter()
                .find(|a| a.get_key() == "user.role")
                .unwrap()
                .get_value()
                .is_empty()
        );
    }

    /// Construction rejects TTLs that would force a reload on every request.
    #[test]
    fn test_new_rejects_ttl_below_floor() {
        let fp = write_fixture("vs-fas-short-ttl.json", "{}");
        let result = FileAttributeStore::new(
            "test".to_string(),
            test_mapper(),
            MIN_ATTRIBUTE_TTL - Duration::from_secs(1),
            &fp,
        );
        assert!(matches!(result, Err(ServiceError::Param(_))));

        let result =
            FileAttributeStore::new("test".to_string(), test_mapper(), MIN_ATTRIBUTE_TTL, &fp);
        assert!(matches!(result, Err(ServiceError::Param(_))));

        assert!(
            FileAttributeStore::new(
                "test".to_string(),
                test_mapper(),
                MIN_ATTRIBUTE_TTL + Duration::from_secs(1),
                &fp
            )
            .is_ok()
        );
        fs::remove_file(&fp).unwrap();
    }

    /// An unexpired store stays cached until an explicit flush installs new data.
    #[tokio::test]
    async fn test_flush_forces_reload_on_next_read() {
        let fp = write_fixture(
            "vs-fas-flush.json",
            r#"{"device.zpr.adapter.cn": {"alice": {"color": ["red"]}}}"#,
        );
        let store = FileAttributeStore::new(
            "test".to_string(),
            test_mapper(),
            Duration::from_secs(3600),
            &fp,
        )
        .unwrap();

        let snapshot = store.live_snapshot().await.unwrap();
        assert!(has_cn_entry(&snapshot, "alice"));

        fs::write(
            &fp,
            r#"{"device.zpr.adapter.cn": {"bob": {"color": ["blue"]}}}"#,
        )
        .unwrap();
        let snapshot = store.live_snapshot().await.unwrap();
        assert!(has_cn_entry(&snapshot, "alice"));
        assert!(!has_cn_entry(&snapshot, "bob"));

        store.flush().await.unwrap();
        let snapshot = store.live_snapshot().await.unwrap();
        assert!(has_cn_entry(&snapshot, "bob"));
        assert!(!has_cn_entry(&snapshot, "alice"));
        assert!(snapshot.remaining() >= MIN_ATTRIBUTE_TTL);

        fs::remove_file(&fp).unwrap();
    }

    /// A source matching several of the actor's identities returns the UNION of their
    /// attributes, and two matched identities disagreeing on one attribute key fail the
    /// lookup (fail closed) rather than silently picking a winner.
    #[tokio::test]
    async fn test_multi_identity_union_and_conflict() {
        // A device CN entry and two user-subject entries for the same actor.
        let fp = write_fixture(
            "vs-fas-multi-ident.json",
            r#"{
                "device.zpr.adapter.cn": {
                    "dev.zpr.org": { "color": ["red"] }
                },
                "user.sub": {
                    "sub-123": { "roles": ["a", "b"] },
                    "sub-conflict": { "color": ["blue"] }
                }
            }"#,
        );
        let store = FileAttributeStore::new(
            "test".to_string(),
            test_mapper(),
            Duration::from_secs(3600),
            &fp,
        )
        .unwrap();

        // Union: both matched identities contribute their (non-overlapping) attributes.
        let attrs = store
            .get_attributes_for_actor(&[
                (key::CN.to_string(), "dev.zpr.org".to_string()),
                ("user.sub".to_string(), "sub-123".to_string()),
            ])
            .await
            .unwrap();
        let keys: Vec<&str> = attrs.iter().map(|a| a.get_key()).collect();
        assert!(keys.contains(&"user.color"));
        assert!(keys.contains(&"user.role"));

        // Conflict: both identities supply `color` with different values -> Err.
        let result = store
            .get_attributes_for_actor(&[
                (key::CN.to_string(), "dev.zpr.org".to_string()),
                ("user.sub".to_string(), "sub-conflict".to_string()),
            ])
            .await;
        assert!(matches!(result, Err(ServiceError::Param(_))));

        // An unknown identity alone matches nothing: empty result, not an error.
        let attrs = store
            .get_attributes_for_actor(&[("user.sub".to_string(), "nobody".to_string())])
            .await
            .unwrap();
        assert!(attrs.is_empty());

        fs::remove_file(&fp).unwrap();
    }

    /// Conflicts are detected on the mapped ZPR key: two DIFFERENT service-side
    /// names mapping to the same ZPR key must fail the lookup when they carry
    /// different values, and collapse to one attribute when they agree.
    #[tokio::test]
    async fn test_conflict_detected_on_mapped_key() {
        use zpr::policy_types::parse_attribute_mapping;

        // `department` and `dept` are aliases for the same canonical ZPR key.
        let mapper = AttributeMapper {
            mappings: ["department -> user.dept", "dept -> user.dept"]
                .iter()
                .map(|mapping| parse_attribute_mapping(mapping).unwrap())
                .collect(),
        };
        let fp = write_fixture(
            "vs-fas-mapped-conflict.json",
            r#"{
                "device.zpr.adapter.cn": {
                    "dev.zpr.org": { "department": ["eng"] }
                },
                "user.sub": {
                    "sub-agree": { "dept": ["eng"] },
                    "sub-conflict": { "dept": ["sales"] }
                }
            }"#,
        );
        let store =
            FileAttributeStore::new("test".to_string(), mapper, Duration::from_secs(3600), &fp)
                .unwrap();

        // Different raw names, same mapped key, conflicting values -> fail closed.
        let result = store
            .get_attributes_for_actor(&[
                (key::CN.to_string(), "dev.zpr.org".to_string()),
                ("user.sub".to_string(), "sub-conflict".to_string()),
            ])
            .await;
        assert!(matches!(result, Err(ServiceError::Param(_))));

        // Different raw names, same mapped key, same values -> one attribute.
        let attrs = store
            .get_attributes_for_actor(&[
                (key::CN.to_string(), "dev.zpr.org".to_string()),
                ("user.sub".to_string(), "sub-agree".to_string()),
            ])
            .await
            .unwrap();
        assert_eq!(attrs.len(), 1);
        assert_eq!(attrs[0].get_key(), "user.dept");

        fs::remove_file(&fp).unwrap();
    }

    /// A failed flush preserves the current snapshot and revision.
    #[tokio::test]
    async fn test_flush_failure_keeps_snapshot_and_revision() {
        let fp = write_fixture(
            "vs-fas-flush-fail.json",
            r#"{"device.zpr.adapter.cn": {"alice": {"color": ["red"]}}}"#,
        );
        let store = FileAttributeStore::new(
            "test".to_string(),
            test_mapper(),
            Duration::from_secs(3600),
            &fp,
        )
        .unwrap();
        let revision = store.current_revision();

        fs::write(&fp, "not json").unwrap();
        assert!(store.flush().await.is_err());
        assert_eq!(store.current_revision(), revision);
        assert!(has_cn_entry(&store.snapshot.load(), "alice"));

        fs::write(
            &fp,
            r#"{"device.zpr.adapter.cn": {"bob": {"color": ["blue"]}}}"#,
        )
        .unwrap();
        store.flush().await.unwrap();
        assert!(store.current_revision() > revision);
        assert!(has_cn_entry(&store.snapshot.load(), "bob"));

        fs::remove_file(&fp).unwrap();
    }
}
