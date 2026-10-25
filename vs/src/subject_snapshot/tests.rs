use super::*;
use std::path::Path;
use std::process::Command;

use openssl::sign::Signer;

#[derive(Deserialize, Serialize)]
struct Vector {
    name: String,
    envelope: String,
    pins: HashMap<String, String>,
    organization: String,
    recipient: String,
    epoch: String,
    now: u64,
    valid: bool,
    prior: String,
    decision: String,
    digest: String,
}

fn go_test(name: &str, env: &str, path: &Path) {
    go_package_test("./internal/subjectsnapshot", name, env, path);
}

fn go_package_test(package: &str, name: &str, env: &str, path: &Path) {
    let output = Command::new("go")
        .args(["test", package, "-run", &format!("^{name}$"), "-count=1"])
        .current_dir(Path::new(env!("CARGO_MANIFEST_DIR")).join("../zpr-dashboard"))
        .env(env, path)
        .output()
        .expect("Go is required for the live signed snapshot interoperability test");
    assert!(
        output.status.success(),
        "Go {name} failed:\n{}\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
}

fn go_vectors(dir: &Path) -> Vec<Vector> {
    let vectors_path = dir.join("vectors.json");
    go_test(
        "TestInteropExport",
        "ZPR_SNAPSHOT_INTEROP_EXPORT",
        &vectors_path,
    );
    let vectors: Vec<Vector> =
        serde_json::from_slice(&std::fs::read(&vectors_path).unwrap()).unwrap();
    assert!(vectors.len() >= 80, "missing contract boundary vectors");
    vectors
}

#[test]
fn live_go_vectors_and_rust_signature_round_trip() {
    let dir = tempfile::tempdir().unwrap();
    let vectors = go_vectors(dir.path());
    for v in &vectors {
        let pins: HashMap<_, _> = v
            .pins
            .iter()
            .map(|(id, key)| (id.clone(), hex::decode(key).unwrap()))
            .collect();
        let scope = Scope {
            organization: &v.organization,
            recipient: &v.recipient,
            epoch: &v.epoch,
        };
        let result = verify(v.envelope.as_bytes(), &pins, &scope, v.now);
        assert_eq!(result.is_ok(), v.valid, "vector {}: {:?}", v.name, result);
        if let Ok(verified) = result {
            assert_eq!(verified.digest(), v.digest, "digest for {}", v.name);
            if !v.prior.is_empty() {
                if v.decision == "reject-scope" {
                    assert!(verify(v.prior.as_bytes(), &pins, &scope, v.now).is_err());
                    let prior_scope = Scope {
                        epoch: "epoch-1",
                        ..scope
                    };
                    let prior = verify(v.prior.as_bytes(), &pins, &prior_scope, v.now).unwrap();
                    assert!(compare(&prior, &verified).is_err());
                    continue;
                }
                let prior = verify(v.prior.as_bytes(), &pins, &scope, v.now)
                    .unwrap_or_else(|e| panic!("prior {}: {e}", v.name));
                let result = compare(&prior, &verified);
                match v.decision.as_str() {
                    "reject" => assert!(result.is_err(), "transition {}", v.name),
                    "advance" => {
                        assert_eq!(result.unwrap(), Decision::Advance, "transition {}", v.name)
                    }
                    "historical-replay" => assert_eq!(
                        result.unwrap(),
                        Decision::HistoricalReplay,
                        "transition {}",
                        v.name
                    ),
                    other => panic!("unknown decision {other}"),
                }
            }
        }
    }

    let valid = vectors.iter().find(|v| v.name == "valid").unwrap();
    let original: Envelope = serde_json::from_str(&valid.envelope).unwrap();
    let raw = URL_SAFE_NO_PAD.decode(&original.payload).unwrap();
    // A different serialization is deliberately signed in Rust, proving neither
    // consumer verifies a reserialized Go/Rust representation.
    let payload: Payload = serde_json::from_slice(&raw).unwrap();
    let raw = serde_json::to_vec_pretty(&payload).unwrap();
    let key = PKey::generate_ed25519().unwrap();
    let mut signer = Signer::new_without_digest(&key).unwrap();
    let mut signed = DOMAIN.to_vec();
    signed.extend_from_slice(&raw);
    let signature = signer.sign_oneshot_to_vec(&signed).unwrap();
    let envelope = Envelope {
        version: 1,
        key_id: "rust-key".into(),
        payload: URL_SAFE_NO_PAD.encode(&raw),
        signature: URL_SAFE_NO_PAD.encode(signature),
    };
    let pins = HashMap::from([("rust-key".into(), key.raw_public_key().unwrap())]);
    let scope = Scope {
        organization: "org",
        recipient: "vs-1",
        epoch: "epoch-1",
    };
    let encoded = serde_json::to_string(&envelope).unwrap();
    let verified = verify(encoded.as_bytes(), &pins, &scope, 1000).unwrap();
    let reverse = Vector {
        name: "rust-signed-pretty-payload".into(),
        envelope: encoded,
        pins: pins
            .into_iter()
            .map(|(id, key)| (id, hex::encode(key)))
            .collect(),
        organization: "org".into(),
        recipient: "vs-1".into(),
        epoch: "epoch-1".into(),
        now: 1000,
        valid: true,
        prior: String::new(),
        decision: String::new(),
        digest: verified.digest().into(),
    };
    let reverse_path = dir.path().join("rust-signed.json");
    std::fs::write(&reverse_path, serde_json::to_vec(&reverse).unwrap()).unwrap();
    go_test(
        "TestInteropImport",
        "ZPR_SNAPSHOT_INTEROP_IMPORT",
        &reverse_path,
    );
}

#[cfg(unix)]
#[test]
fn live_go_snapshots_survive_durable_installation_and_restart() {
    use super::checkpoint::Checkpoint;

    let temp = tempfile::tempdir().unwrap();
    let vectors = go_vectors(temp.path());
    let mut exercised = HashSet::new();
    for v in &vectors {
        let directory = temp.path().join(&v.name);
        let pins: HashMap<_, _> = v
            .pins
            .iter()
            .map(|(id, key)| (id.clone(), hex::decode(key).unwrap()))
            .collect();
        let scope = Scope {
            organization: &v.organization,
            recipient: &v.recipient,
            epoch: &v.epoch,
        };
        if v.prior.is_empty() {
            let result = Checkpoint::initialize(
                &directory,
                pins.clone(),
                &scope,
                v.envelope.as_bytes(),
                v.now,
            );
            assert_eq!(result.is_ok(), v.valid, "initialization {}", v.name);
            if !v.valid {
                assert!(
                    !directory.exists(),
                    "invalid snapshot wrote state: {}",
                    v.name
                );
                continue;
            }
            drop(result.unwrap());
            let mut store = Checkpoint::open(&directory, pins, &scope).unwrap();
            assert_eq!(
                store.current_at(v.now).unwrap().digest(),
                v.digest,
                "restart {}",
                v.name
            );
            let before = std::fs::read(directory.join("accepted.json")).unwrap();
            assert_eq!(
                store.install(v.envelope.as_bytes(), v.now).unwrap(),
                Decision::HistoricalReplay,
                "retry {}",
                v.name
            );
            assert_eq!(
                std::fs::read(directory.join("accepted.json")).unwrap(),
                before
            );
            let state: serde_json::Value = serde_json::from_slice(&before).unwrap();
            assert_eq!(
                state["envelope"].as_str().unwrap(),
                v.envelope,
                "exact bytes {}",
                v.name
            );
            let expires: u64 = state["expires_at"].as_str().unwrap().parse().unwrap();
            assert!(
                store.current_at(expires).is_err(),
                "renewed expiry {}",
                v.name
            );
            assert!(store.install(v.envelope.as_bytes(), expires).is_err());
        } else {
            let prior_scope = Scope {
                epoch: if v.decision == "reject-scope" {
                    "epoch-1"
                } else {
                    scope.epoch
                },
                ..scope
            };
            let mut store = Checkpoint::initialize(
                &directory,
                pins.clone(),
                &prior_scope,
                v.prior.as_bytes(),
                v.now,
            )
            .unwrap();
            let prior_digest = store.current_at(v.now).unwrap().digest().to_owned();
            let before = std::fs::read(directory.join("accepted.json")).unwrap();
            let result = store.install(v.envelope.as_bytes(), v.now);
            let expected = match v.decision.as_str() {
                "advance" => {
                    assert_eq!(result.unwrap(), Decision::Advance, "install {}", v.name);
                    v.digest.as_str()
                }
                "historical-replay" => {
                    assert_eq!(result.unwrap(), Decision::HistoricalReplay);
                    assert_eq!(
                        std::fs::read(directory.join("accepted.json")).unwrap(),
                        before
                    );
                    prior_digest.as_str()
                }
                "reject" | "reject-scope" => {
                    assert!(result.is_err(), "accepted forbidden transition {}", v.name);
                    assert_eq!(
                        std::fs::read(directory.join("accepted.json")).unwrap(),
                        before
                    );
                    prior_digest.as_str()
                }
                other => panic!("unexpected decision {other}"),
            };
            drop(store);
            let mut reopened = Checkpoint::open(&directory, pins.clone(), &prior_scope).unwrap();
            assert_eq!(
                reopened.current_at(v.now).unwrap().digest(),
                expected,
                "restart {}",
                v.name
            );
            let retry = reopened.install(v.envelope.as_bytes(), v.now);
            if v.decision == "advance" || v.decision == "historical-replay" {
                assert_eq!(
                    retry.unwrap(),
                    Decision::HistoricalReplay,
                    "lost-result recovery {}",
                    v.name
                );
            } else {
                assert!(retry.is_err(), "restart lost rejection {}", v.name);
            }
            if v.decision == "advance" {
                assert!(
                    reopened.install(v.prior.as_bytes(), v.now).is_err(),
                    "restart lost ordering floor {}",
                    v.name
                );
            }
            drop(reopened);
            if v.decision == "reject-scope" {
                assert!(Checkpoint::open(&directory, pins, &scope).is_err());
            }
        }

        exercised.insert(v.name.as_str());
    }
    for required in [
        "valid",
        "overlap-old",
        "overlap-new",
        "retained-new",
        "payload-bound-1048576",
        "envelope-bound-1400000",
        "record-bound-1024",
        "compare-revoke",
        "compare-updated-binding",
        "historical-replay",
        "source-version-rollback",
        "authority-version-rollback",
        "binding-version-rollback",
        "compare-epoch-reset",
        "forget-tombstone",
        "resurrect-tombstone",
        "lower-tombstone-version",
    ] {
        assert!(
            exercised.contains(required),
            "missing durable case {required}"
        );
    }
}

#[cfg(unix)]
#[test]
fn retained_service_candidates_install_without_reserializing() {
    #[derive(Deserialize)]
    struct Candidates {
        first: String,
        second: String,
        first_digest: String,
        second_digest: String,
        public_key: String,
        organization: String,
        recipient: String,
        epoch: String,
        now: u64,
        binding_id: String,
    }
    let temp = tempfile::tempdir().unwrap();
    let export = temp.path().join("candidates.json");
    go_package_test(
        "./internal/changereview",
        "TestSubjectLinkCandidateInteropExport",
        "ZPR_CANDIDATE_INTEROP_EXPORT",
        &export,
    );
    let c: Candidates = serde_json::from_slice(&std::fs::read(&export).unwrap()).unwrap();
    let pins = HashMap::from([("fixture".into(), hex::decode(c.public_key).unwrap())]);
    let scope = Scope {
        organization: &c.organization,
        recipient: &c.recipient,
        epoch: &c.epoch,
    };
    let first = verify(c.first.as_bytes(), &pins, &scope, c.now).unwrap();
    let second = verify(c.second.as_bytes(), &pins, &scope, c.now).unwrap();
    assert_eq!(first.digest(), c.first_digest);
    assert_eq!(second.digest(), c.second_digest);
    assert_eq!(first.payload.sequence, "2");
    assert_eq!(second.payload.sequence, "4");
    assert_eq!(first.payload.bindings.len(), 1);
    assert_eq!(first.payload.bindings[0].binding_id, c.binding_id);
    assert_eq!(first.payload.authorities.len(), 1);
    assert_eq!(first.payload.sources.len(), 1);
    assert_eq!(compare(&first, &second).unwrap(), Decision::Advance);
    let directory = temp.path().join("checkpoint");
    let mut store = checkpoint::Checkpoint::initialize(
        &directory,
        pins.clone(),
        &scope,
        c.first.as_bytes(),
        c.now,
    )
    .unwrap();
    assert_eq!(
        store.install(c.second.as_bytes(), c.now).unwrap(),
        Decision::Advance
    );
    drop(store);
    let mut store = checkpoint::Checkpoint::open(&directory, pins, &scope).unwrap();
    let restored = store.current_at(c.now).unwrap();
    assert_eq!(restored.digest(), c.second_digest);
    assert!(restored.payload.bindings.is_empty());
    assert_eq!(restored.payload.tombstones.len(), 1);
    assert_eq!(restored.payload.tombstones[0].binding_id, c.binding_id);
    assert_eq!(restored.payload.tombstones[0].version, "2");
    assert_eq!(
        store.install(c.second.as_bytes(), c.now).unwrap(),
        Decision::HistoricalReplay
    );
    assert!(store.install(c.first.as_bytes(), c.now).is_err());
}

#[cfg(unix)]
#[test]
fn durable_go_revocation_child() {
    let Ok(root) = std::env::var("ZPR_GO_SNAPSHOT_REVOKE_CHILD") else {
        return;
    };
    let root = Path::new(&root);
    let v: Vector =
        serde_json::from_slice(&std::fs::read(root.join("revocation.json")).unwrap()).unwrap();
    let pins = v
        .pins
        .iter()
        .map(|(id, key)| (id.clone(), hex::decode(key).unwrap()))
        .collect();
    let scope = Scope {
        organization: &v.organization,
        recipient: &v.recipient,
        epoch: &v.epoch,
    };
    let mut store = checkpoint::Checkpoint::open(&root.join("checkpoint"), pins, &scope).unwrap();
    assert_eq!(
        store.install(v.envelope.as_bytes(), v.now).unwrap(),
        Decision::Advance
    );
    // Leave without destructors or communicating a delivery result to the
    // parent. A retry must recover the durable revocation, not apply it anew.
    std::process::exit(73);
}

#[cfg(unix)]
#[test]
fn go_revocation_recovers_after_process_loses_install_result() {
    let temp = tempfile::tempdir().unwrap();
    let vectors = go_vectors(temp.path());
    let v = vectors.iter().find(|v| v.name == "compare-revoke").unwrap();
    let pins = v
        .pins
        .iter()
        .map(|(id, key)| (id.clone(), hex::decode(key).unwrap()))
        .collect::<HashMap<_, _>>();
    let scope = Scope {
        organization: &v.organization,
        recipient: &v.recipient,
        epoch: &v.epoch,
    };
    let directory = temp.path().join("checkpoint");
    drop(
        checkpoint::Checkpoint::initialize(
            &directory,
            pins.clone(),
            &scope,
            v.prior.as_bytes(),
            v.now,
        )
        .unwrap(),
    );
    std::fs::write(
        temp.path().join("revocation.json"),
        serde_json::to_vec(v).unwrap(),
    )
    .unwrap();
    let output = Command::new(std::env::current_exe().unwrap())
        .args([
            "--exact",
            "subject_snapshot::tests::durable_go_revocation_child",
            "--nocapture",
        ])
        .env("ZPR_GO_SNAPSHOT_REVOKE_CHILD", temp.path())
        .output()
        .unwrap();
    assert_eq!(
        output.status.code(),
        Some(73),
        "{}\n{}",
        String::from_utf8_lossy(&output.stdout),
        String::from_utf8_lossy(&output.stderr)
    );
    let mut store = checkpoint::Checkpoint::open(&directory, pins, &scope).unwrap();
    let restored = store.current_at(v.now).unwrap();
    assert_eq!(restored.digest(), v.digest);
    assert!(restored.payload.bindings.is_empty());
    assert_eq!(restored.payload.tombstones.len(), 1);
    assert_eq!(restored.payload.tombstones[0].binding_id, "link-1");
    assert_eq!(restored.payload.tombstones[0].version, "2");
    assert_eq!(
        store.install(v.envelope.as_bytes(), v.now).unwrap(),
        Decision::HistoricalReplay
    );
    assert!(store.install(v.prior.as_bytes(), v.now).is_err());
}
