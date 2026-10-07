package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayConfigStoreSavesPrivateOrganizationScopedRevisions(t *testing.T) {
	store, err := newGatewayConfigStore(filepath.Join(t.TempDir(), "gateway-store"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Save("northstar", "public-egress", 0, json.RawMessage(`{"revision":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.CurrentRevision != 1 || len(first.Revisions) != 1 {
		t.Fatalf("first save = %+v", first)
	}
	second, err := store.Save("northstar", "public-egress", 1, json.RawMessage(`{"revision":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if second.CurrentRevision != 2 || len(second.Revisions) != 2 {
		t.Fatalf("second save = %+v", second)
	}
	if _, err := store.Save("northstar", "public-egress", 1, json.RawMessage(`{"revision":3}`)); !errors.Is(err, errGatewayConfigRevisionConflict) {
		t.Fatalf("stale revision save error = %v", err)
	}
	loaded, err := store.Get("northstar", "public-egress")
	if err != nil || loaded.CurrentRevision != 2 || string(loaded.Revisions[1].Config) != `{"revision":2}` {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if records, err := store.List("redwood"); err != nil || len(records) != 0 {
		t.Fatalf("other organization records=%+v err=%v", records, err)
	}

	organizationInfo, err := os.Stat(filepath.Join(store.root, "northstar"))
	if err != nil || organizationInfo.Mode().Perm() != 0o700 {
		t.Fatalf("organization directory mode=%v err=%v", organizationInfo.Mode().Perm(), err)
	}
	recordInfo, err := os.Stat(filepath.Join(store.root, "northstar", "public-egress.json"))
	if err != nil || recordInfo.Mode().Perm() != 0o600 {
		t.Fatalf("gateway record mode=%v err=%v", recordInfo.Mode().Perm(), err)
	}
}

func TestGatewayConfigStoreRejectsUnsafeIdentityAndContents(t *testing.T) {
	store, err := newGatewayConfigStore(filepath.Join(t.TempDir(), "gateway-store"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save("../northstar", "public-egress", 0, json.RawMessage(`{}`)); err == nil {
		t.Fatal("unsafe organization ID accepted")
	}
	if _, err := store.Save("northstar", "../outside", 0, json.RawMessage(`{}`)); err == nil {
		t.Fatal("unsafe instance ID accepted")
	}
	if _, err := store.Save("northstar", "public-egress", 0, json.RawMessage(`{`)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := store.Save("northstar", "public-egress", 0, json.RawMessage(make([]byte, maxGatewayConfigBytes+1))); err == nil {
		t.Fatal("oversized config accepted")
	}
}

func TestGatewayConfigStoreRejectsSymlinkedRecords(t *testing.T) {
	directory := t.TempDir()
	store, err := newGatewayConfigStore(filepath.Join(directory, "gateway-store"))
	if err != nil {
		t.Fatal(err)
	}
	orgDir := filepath.Join(store.root, "northstar")
	if err := os.MkdirAll(orgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "outside.json")
	if err := os.WriteFile(target, []byte(`{"organization_id":"northstar"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(orgDir, "public-egress.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("northstar", "public-egress"); err == nil {
		t.Fatal("symlinked gateway record accepted")
	}

	outsideDirectory := filepath.Join(directory, "outside-org")
	if err := os.MkdirAll(outsideDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDirectory, "public-egress.json"), []byte(`{"organization_id":"redwood"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDirectory, filepath.Join(store.root, "redwood")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("redwood", "public-egress"); err == nil {
		t.Fatal("symlinked organization directory accepted")
	}
}
