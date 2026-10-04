package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSQLitePolicyDatabaseRequiresPrivateDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := openSQLitePolicyRepository(filepath.Join(directory, "policy.db")); err == nil {
		t.Fatal("database opened in a directory accessible to other users")
	}
}

func TestSQLitePolicyDatabaseAndWALFilesArePrivate(t *testing.T) {
	databasePath := filepath.Join(privatePolicyTestDir(t), "policy.db")
	repository, err := openSQLitePolicyRepository(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	category, err := repository.CreateCategory(context.Background(), nil, "Policies")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.CreateRecord(context.Background(), category.ID, "alpha", "policy", "text/vnd.zpr.zpl", json.RawMessage(`{"language":"zpl"}`), "allow team.", "tester", "Initial")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{databasePath, databasePath + "-wal", databasePath + "-shm"} {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Errorf("%s permissions = %o; want no group/other access", filepath.Base(path), info.Mode().Perm())
		}
	}
}

func TestSQLitePolicyRecordArchivePreservesImmutableRevisions(t *testing.T) {
	repository, err := openSQLitePolicyRepository(filepath.Join(privatePolicyTestDir(t), "archive.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	category, err := repository.CreateCategory(context.Background(), nil, "Policies")
	if err != nil {
		t.Fatal(err)
	}
	record, err := repository.CreateRecord(context.Background(), category.ID, "Keep history", "policy", "text/vnd.zpr.zpl", json.RawMessage(`{"language":"zpl"}`), "allow team.", "tester", "Initial")
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.SetRecordArchived(context.Background(), record.ID, record.CurrentRevision, true); err != nil {
		t.Fatal(err)
	}
	archived, err := repository.GetRecord(context.Background(), record.ID)
	if err != nil || !archived.Archived {
		t.Fatalf("archived record = %+v, err = %v", archived, err)
	}
	if _, err := repository.AppendRevision(context.Background(), record.ID, record.CurrentRevision, "allow changed.", "tester", "Must not update archived record"); err == nil {
		t.Fatal("archived record accepted a new revision")
	}
	if err := repository.SetRecordArchived(context.Background(), record.ID, record.CurrentRevision, false); err != nil {
		t.Fatal(err)
	}
	restored, err := repository.GetRecord(context.Background(), record.ID)
	if err != nil || restored.Archived {
		t.Fatalf("restored record = %+v, err = %v", restored, err)
	}
	revisions, err := repository.ListRevisions(context.Background(), record.ID)
	if err != nil || len(revisions) != 1 {
		t.Fatalf("revision history = %+v, err = %v", revisions, err)
	}
}

func TestSQLiteRevisionRowsRemainImmutableAcrossSchemaUpgrade(t *testing.T) {
	databasePath := filepath.Join(privatePolicyTestDir(t), "policy.db")
	repository, err := openSQLitePolicyRepository(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	category, err := repository.CreateCategory(context.Background(), nil, "Policies")
	if err != nil {
		t.Fatal(err)
	}
	record, err := repository.CreateRecord(context.Background(), category.ID, "alpha", "policy", "text/vnd.zpr.zpl", json.RawMessage(`{"language":"zpl"}`), "allow team.", "tester", "Initial")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.db.Exec(`DROP TRIGGER policy_revisions_no_update; DROP TRIGGER policy_revisions_no_delete; PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
	if err := repository.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err = openSQLitePolicyRepository(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	for _, statement := range []string{
		`UPDATE policy_revisions SET content='tampered' WHERE record_id=? AND revision=1`,
		`DELETE FROM policy_revisions WHERE record_id=? AND revision=1`,
	} {
		if _, err := repository.db.Exec(statement, record.ID); err == nil {
			t.Fatalf("immutable revision accepted statement %q", statement)
		}
	}
	revision, err := repository.GetRevision(context.Background(), record.ID, 1)
	if err != nil || revision.Content != "allow team." {
		t.Fatalf("revision changed after attempted mutation: %+v, err = %v", revision, err)
	}
}

func privatePolicyTestDir(t *testing.T) string {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	return directory
}
