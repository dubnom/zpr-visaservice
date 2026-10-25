use super::*;
use crate::subject_snapshot::{DOMAIN, Envelope};
use base64::{Engine, engine::general_purpose::URL_SAFE_NO_PAD};
use openssl::{pkey::PKey, sign::Signer};
use std::os::unix::fs::symlink;
use std::process::Command;

fn scope() -> Scope<'static> {
    Scope {
        organization: "org",
        recipient: "vs-1",
        epoch: "epoch-1",
    }
}

fn sign(key: &PKey<openssl::pkey::Private>, sequence: &str, expires: &str) -> Vec<u8> {
    let raw = serde_json::to_vec(&serde_json::json!({
        "version": 1, "organization_id": "org", "vs_id": "vs-1",
        "epoch": "epoch-1", "sequence": sequence, "issued_at": "1000",
        "expires_at": expires, "authorities": [], "sources": [],
        "bindings": [], "tombstones": []
    }))
    .unwrap();
    let mut signed = DOMAIN.to_vec();
    signed.extend_from_slice(&raw);
    let signature = Signer::new_without_digest(key)
        .unwrap()
        .sign_oneshot_to_vec(&signed)
        .unwrap();
    serde_json::to_vec(&Envelope {
        version: 1,
        key_id: "key".into(),
        payload: URL_SAFE_NO_PAD.encode(raw),
        signature: URL_SAFE_NO_PAD.encode(signature),
    })
    .unwrap()
}

fn pins(key: &PKey<openssl::pkey::Private>) -> HashMap<String, Vec<u8>> {
    HashMap::from([("key".into(), key.raw_public_key().unwrap())])
}

#[test]
fn explicit_initialization_advance_reopen_and_expired_ordering_floor() {
    let temp = tempfile::tempdir().unwrap();
    let dir = temp.path().join("checkpoint");
    let key = PKey::generate_ed25519().unwrap();
    let first = sign(&key, "1", "2000");
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    let mut store = Checkpoint::initialize(&dir, pins(&key), &scope(), &first, 1000).unwrap();
    assert_eq!(
        fs::metadata(&dir).unwrap().permissions().mode() & 0o777,
        0o700
    );
    assert_eq!(
        fs::metadata(dir.join("accepted.json"))
            .unwrap()
            .permissions()
            .mode()
            & 0o777,
        0o600
    );
    assert!(
        Checkpoint::open(&dir, pins(&key), &scope()).is_err(),
        "second writer acquired lock"
    );
    assert!(Checkpoint::initialize(&dir, pins(&key), &scope(), &first, 1000).is_err());
    let original = fs::read(dir.join("accepted.json")).unwrap();
    assert_eq!(
        store.install(&first, 1999).unwrap(),
        Decision::HistoricalReplay
    );
    assert_eq!(fs::read(dir.join("accepted.json")).unwrap(), original);
    assert!(
        store.install(&first, 2000).is_err(),
        "replay renewed expired lease"
    );
    assert!(store.current_at(2000).is_err());
    let next = sign(&key, "2", "3000");
    assert_eq!(store.install(&next, 2100).unwrap(), Decision::Advance);
    assert!(store.install(&first, 1500).is_err());
    drop(store);
    let store = Checkpoint::open(&dir, pins(&key), &scope()).unwrap();
    assert_eq!(store.state.sequence, "2");
    assert_eq!(store.state.envelope.as_bytes(), next);
    assert!(store.current_at(3000).is_err());
    drop(store);
    let mut store = Checkpoint::open(&dir, pins(&key), &scope()).unwrap();
    assert_eq!(
        store.install(&sign(&key, "3", "4000"), 3100).unwrap(),
        Decision::Advance
    );
}

#[test]
fn corruption_missing_scope_trust_and_permissions_fail_closed() {
    let temp = tempfile::tempdir().unwrap();
    let key = PKey::generate_ed25519().unwrap();
    let other = PKey::generate_ed25519().unwrap();
    let dir = temp.path().join("checkpoint");
    drop(
        Checkpoint::initialize(&dir, pins(&key), &scope(), &sign(&key, "1", "2000"), 1000).unwrap(),
    );
    let state_path = dir.join("accepted.json");
    let original = fs::read(&state_path).unwrap();
    assert!(Checkpoint::open(&dir, pins(&other), &scope()).is_err());
    assert!(
        Checkpoint::open(
            &dir,
            pins(&key),
            &Scope {
                epoch: "epoch-2",
                ..scope()
            }
        )
        .is_err()
    );
    for field in [
        "organization",
        "recipient",
        "epoch",
        "sequence",
        "issued_at",
        "expires_at",
        "digest",
    ] {
        let mut value: serde_json::Value = serde_json::from_slice(&original).unwrap();
        value[field] = "tampered".into();
        fs::write(&state_path, serde_json::to_vec(&value).unwrap()).unwrap();
        assert!(
            Checkpoint::open(&dir, pins(&key), &scope()).is_err(),
            "{field}"
        );
    }
    for bad in [
        b"null".as_slice(),
        b"{}",
        b"{",
        b"",
        b"{\"version\":1,\"version\":1}",
    ] {
        fs::write(&state_path, bad).unwrap();
        assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    }
    fs::write(&state_path, &original).unwrap();
    fs::set_permissions(&state_path, fs::Permissions::from_mode(0o644)).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    fs::set_permissions(&state_path, fs::Permissions::from_mode(0o600)).unwrap();
    fs::rename(&state_path, dir.join("saved.json")).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    symlink(dir.join("saved.json"), &state_path).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    fs::remove_file(&state_path).unwrap();
    fs::rename(dir.join("saved.json"), &state_path).unwrap();
    fs::remove_file(dir.join("writer.lock")).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    assert!(
        !dir.join("writer.lock").exists(),
        "open recreated missing lock"
    );
}

#[test]
fn failed_write_poisoning_and_uncommitted_cleanup() {
    for phase in ["written", "file-synced", "renamed", "directory-synced"] {
        let temp = tempfile::tempdir().unwrap();
        let dir = temp.path().join("checkpoint");
        let key = PKey::generate_ed25519().unwrap();
        let first = sign(&key, "1", "2000");
        let next = sign(&key, "2", "3000");
        let mut store = Checkpoint::initialize(&dir, pins(&key), &scope(), &first, 1000).unwrap();
        assert!(
            store
                .install_with_hook(&next, 1000, |stage| {
                    if stage == phase {
                        Err(std::io::Error::other("injected I/O failure"))
                    } else {
                        Ok(())
                    }
                })
                .is_err()
        );
        assert!(matches!(
            store.current_at(1000),
            Err(CheckpointError::Uncertain)
        ));
        assert!(matches!(
            store.install(&first, 1000),
            Err(CheckpointError::Uncertain)
        ));
        drop(store);
        let mut reopened = Checkpoint::open(&dir, pins(&key), &scope()).unwrap();
        let renamed = phase == "renamed" || phase == "directory-synced";
        assert_eq!(reopened.state.sequence, if renamed { "2" } else { "1" });
        assert!(!dir.join("pending.json").exists());
        assert_eq!(
            reopened.install(&next, 1000).unwrap(),
            if renamed {
                Decision::HistoricalReplay
            } else {
                Decision::Advance
            }
        );
    }
}

#[test]
fn crash_child() {
    let Ok(dir) = std::env::var("ZPR_SNAPSHOT_CRASH_DIR") else {
        return;
    };
    let phase = std::env::var("ZPR_SNAPSHOT_CRASH_PHASE").unwrap();
    let pin = hex::decode(std::env::var("ZPR_SNAPSHOT_CRASH_PIN").unwrap()).unwrap();
    let mut store = Checkpoint::open(
        Path::new(&dir),
        HashMap::from([("key".into(), pin)]),
        &scope(),
    )
    .unwrap();
    let next = fs::read(Path::new(&dir).parent().unwrap().join("candidate.json")).unwrap();
    store
        .install_with_hook(&next, 1000, |stage| {
            if stage == phase {
                std::process::exit(73);
            }
            Ok(())
        })
        .unwrap();
    panic!("crash hook not reached");
}

#[test]
fn process_death_before_and_after_atomic_rename() {
    for phase in ["written", "file-synced", "renamed", "directory-synced"] {
        let temp = tempfile::tempdir().unwrap();
        let dir = temp.path().join("checkpoint");
        let key = PKey::generate_ed25519().unwrap();
        drop(
            Checkpoint::initialize(&dir, pins(&key), &scope(), &sign(&key, "1", "2000"), 1000)
                .unwrap(),
        );
        let next = sign(&key, "2", "3000");
        fs::write(temp.path().join("candidate.json"), &next).unwrap();
        let output = Command::new(std::env::current_exe().unwrap())
            .args([
                "--exact",
                "subject_snapshot::checkpoint::tests::crash_child",
                "--nocapture",
            ])
            .env("ZPR_SNAPSHOT_CRASH_DIR", &dir)
            .env("ZPR_SNAPSHOT_CRASH_PHASE", phase)
            .env(
                "ZPR_SNAPSHOT_CRASH_PIN",
                hex::encode(key.raw_public_key().unwrap()),
            )
            .output()
            .unwrap();
        assert_eq!(
            output.status.code(),
            Some(73),
            "{phase}: {}",
            String::from_utf8_lossy(&output.stderr)
        );
        let mut reopened = Checkpoint::open(&dir, pins(&key), &scope()).unwrap();
        let renamed = phase == "renamed" || phase == "directory-synced";
        assert_eq!(reopened.state.sequence, if renamed { "2" } else { "1" });
        assert_eq!(
            reopened.install(&next, 1000).unwrap(),
            if renamed {
                Decision::HistoricalReplay
            } else {
                Decision::Advance
            }
        );
        assert!(reopened.install(&sign(&key, "1", "2000"), 1000).is_err());
    }
}

#[test]
fn competing_writer_child() {
    let Ok(dir) = std::env::var("ZPR_SNAPSHOT_LOCK_DIR") else {
        return;
    };
    let pin = hex::decode(std::env::var("ZPR_SNAPSHOT_LOCK_PIN").unwrap()).unwrap();
    assert!(
        Checkpoint::open(
            Path::new(&dir),
            HashMap::from([("key".into(), pin)]),
            &scope()
        )
        .is_err()
    );
}

#[test]
fn competing_process_cannot_acquire_writer_lock() {
    let temp = tempfile::tempdir().unwrap();
    let dir = temp.path().join("checkpoint");
    let key = PKey::generate_ed25519().unwrap();
    let store =
        Checkpoint::initialize(&dir, pins(&key), &scope(), &sign(&key, "1", "2000"), 1000).unwrap();
    let output = Command::new(std::env::current_exe().unwrap())
        .args([
            "--exact",
            "subject_snapshot::checkpoint::tests::competing_writer_child",
            "--nocapture",
        ])
        .env("ZPR_SNAPSHOT_LOCK_DIR", &dir)
        .env(
            "ZPR_SNAPSHOT_LOCK_PIN",
            hex::encode(key.raw_public_key().unwrap()),
        )
        .output()
        .unwrap();
    assert!(
        output.status.success(),
        "{}",
        String::from_utf8_lossy(&output.stderr)
    );
    drop(store);
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_ok());
}

#[test]
fn pin_retirement_requires_checkpoint_signed_by_retained_key() {
    let temp = tempfile::tempdir().unwrap();
    let dir = temp.path().join("checkpoint");
    let old = PKey::generate_ed25519().unwrap();
    let new = PKey::generate_ed25519().unwrap();
    let both = HashMap::from([
        ("key".into(), old.raw_public_key().unwrap()),
        ("new".into(), new.raw_public_key().unwrap()),
    ]);
    let retained = HashMap::from([("new".into(), new.raw_public_key().unwrap())]);
    let first = sign(&old, "1", "2000");
    drop(Checkpoint::initialize(&dir, both.clone(), &scope(), &first, 1000).unwrap());
    let mut store = Checkpoint::open(&dir, both.clone(), &scope()).unwrap();
    let rename_key = |bytes: Vec<u8>| {
        let mut envelope: Envelope = serde_json::from_slice(&bytes).unwrap();
        envelope.key_id = "new".into();
        serde_json::to_vec(&envelope).unwrap()
    };
    assert_eq!(
        store
            .install(&rename_key(sign(&new, "1", "2000")), 1000)
            .unwrap(),
        Decision::HistoricalReplay
    );
    drop(store);
    assert!(
        Checkpoint::open(&dir, retained.clone(), &scope()).is_err(),
        "historical replay unexpectedly replaced durable signing evidence"
    );
    let mut store = Checkpoint::open(&dir, both, &scope()).unwrap();
    assert_eq!(
        store
            .install(&rename_key(sign(&new, "2", "3000")), 1000)
            .unwrap(),
        Decision::Advance
    );
    drop(store);
    assert!(
        Checkpoint::open(&dir, retained, &scope())
            .unwrap()
            .current_at(1000)
            .is_ok()
    );
}

#[test]
fn insecure_paths_and_pending_files_never_replace_committed_state() {
    let temp = tempfile::tempdir().unwrap();
    let dir = temp.path().join("checkpoint");
    let alias = temp.path().join("alias");
    let key = PKey::generate_ed25519().unwrap();
    let first = sign(&key, "1", "2000");
    drop(Checkpoint::initialize(&dir, pins(&key), &scope(), &first, 1000).unwrap());
    symlink(&dir, &alias).unwrap();
    assert!(Checkpoint::open(&alias, pins(&key), &scope()).is_err());
    fs::set_permissions(&dir, fs::Permissions::from_mode(0o755)).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    fs::set_permissions(&dir, fs::Permissions::from_mode(0o700)).unwrap();
    symlink(dir.join("accepted.json"), dir.join("pending.json")).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    fs::remove_file(dir.join("pending.json")).unwrap();
    let bytes = fs::read(dir.join("accepted.json")).unwrap();
    let oversized = OpenOptions::new()
        .write(true)
        .open(dir.join("accepted.json"))
        .unwrap();
    oversized.set_len(MAX_STATE as u64 + 1).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    fs::write(dir.join("accepted.json"), &bytes).unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_ok());
}

#[test]
fn interrupted_initialization_is_not_promoted_or_reset() {
    let temp = tempfile::tempdir().unwrap();
    let dir = temp.path().join("checkpoint");
    DirBuilder::new().mode(0o700).create(&dir).unwrap();
    drop(lock(&dir, true).unwrap());
    fs::write(dir.join("pending.json"), "uncommitted").unwrap();
    fs::set_permissions(dir.join("pending.json"), fs::Permissions::from_mode(0o600)).unwrap();
    let key = PKey::generate_ed25519().unwrap();
    assert!(Checkpoint::open(&dir, pins(&key), &scope()).is_err());
    assert!(dir.join("pending.json").exists());
    assert!(
        Checkpoint::initialize(&dir, pins(&key), &scope(), &sign(&key, "1", "2000"), 1000).is_err()
    );
}
