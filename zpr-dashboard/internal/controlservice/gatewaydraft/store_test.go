package gatewaydraft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStoreRetentionReopenAndJSONEncoding(t *testing.T) {
	root := filepath.Join(t.TempDir(), "drafts")
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	config := json.RawMessage("{\n  \"source\": \"unchanged\"\n}")
	var compact bytes.Buffer
	if err := json.Compact(&compact, config); err != nil {
		t.Fatal(err)
	}
	for revision := 0; revision < 52; revision++ {
		if _, err := store.Save("production", "gateway", revision, config); err != nil {
			t.Fatalf("save %d: %v", revision, err)
		}
	}
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := reopened.Get("production", "gateway")
	if err != nil || record.CurrentRevision != 52 || len(record.Revisions) != 50 {
		t.Fatalf("reopened = %+v %v", record, err)
	}
	for index, revision := range record.Revisions {
		if revision.Revision != index+3 || !bytes.Equal(revision.Config, compact.Bytes()) || revision.SavedAt.IsZero() {
			t.Fatalf("revision %d = %+v", index, revision)
		}
	}
	for path, mode := range map[string]os.FileMode{
		root: 0o700, filepath.Join(root, "production"): 0o700,
		filepath.Join(root, "production", "gateway.json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("%s permissions = %o want %o", path, info.Mode().Perm(), mode)
		}
	}
	entries, err := os.ReadDir(filepath.Join(root, "production"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files retained: %v %v", entries, err)
	}
	if _, err := reopened.Get("other", "gateway"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cross-org read: %v", err)
	}
}

func TestStoreConcurrentExpectedRevision(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var writers sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		writers.Go(func() {
			_, err := store.Save("production", "gateway", 0, json.RawMessage(`{"source":1}`))
			results <- err
		})
	}
	writers.Wait()
	close(results)
	saved := 0
	for err := range results {
		if err == nil {
			saved++
		} else if !errors.Is(err, ErrRevisionConflict) {
			t.Fatal(err)
		}
	}
	if saved != 1 {
		t.Fatalf("successful concurrent writers = %d", saved)
	}
}

func TestStoreConfigSizeAndCorruptRecordBoundaries(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := json.RawMessage(`{}` + string(bytes.Repeat([]byte(" "), MaxConfigBytes-2)))
	if _, err := store.Save("production", "gateway", 0, data); err != nil {
		t.Fatalf("exact config size rejected: %v", err)
	}
	if _, err := store.Save("production", "gateway", 1, append(data, ' ')); err == nil {
		t.Fatal("one byte over accepted")
	}
	record, err := store.Get("production", "gateway")
	if err != nil || record.CurrentRevision != 1 {
		t.Fatalf("failed save modified revision: %+v %v", record, err)
	}
	path := filepath.Join(store.root, "production", "gateway.json")
	for _, test := range []struct {
		name, content string
	}{
		{"unknown field", `{"unknown":true}`},
		{"trailing data", `{}` + ` {}`},
		{"identity", `{"organization_id":"other","instance_id":"gateway","current_revision":1,"revisions":[{"revision":1,"config":{}}]}`},
		{"revision order", `{"organization_id":"production","instance_id":"gateway","current_revision":2,"revisions":[{"revision":2,"config":{}},{"revision":1,"config":{}}]}`},
		{"current revision", `{"organization_id":"production","instance_id":"gateway","current_revision":2,"revisions":[{"revision":1,"config":{}}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get("production", "gateway"); err == nil {
				t.Fatal("corrupt record accepted")
			}
			if _, err := store.List("production"); err == nil {
				t.Fatal("corrupt record silently omitted")
			}
		})
	}
	for index, instance := range []string{"zulu", "alpha"} {
		if _, err := store.Save("other", instance, 0, json.RawMessage(fmt.Sprintf(`{"index":%d}`, index))); err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.List("other")
	if err != nil || len(records) != 2 || records[0].InstanceID != "alpha" || records[1].InstanceID != "zulu" {
		t.Fatalf("list order = %+v %v", records, err)
	}
}
