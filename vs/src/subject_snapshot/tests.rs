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
    let output = Command::new("go")
        .args([
            "test",
            "./internal/subjectsnapshot",
            "-run",
            &format!("^{name}$"),
            "-count=1",
        ])
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

#[test]
fn live_go_vectors_and_rust_signature_round_trip() {
    let dir = tempfile::tempdir().unwrap();
    let vectors_path = dir.path().join("vectors.json");
    go_test(
        "TestInteropExport",
        "ZPR_SNAPSHOT_INTEROP_EXPORT",
        &vectors_path,
    );
    let vectors: Vec<Vector> =
        serde_json::from_slice(&std::fs::read(&vectors_path).unwrap()).unwrap();
    assert!(vectors.len() >= 80, "missing contract boundary vectors");
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
