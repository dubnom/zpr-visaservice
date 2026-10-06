package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceRepositoryScopesArtifactsAndVersionsByOrganization(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "workspace.db")
	policyStore, err := openSQLitePolicyRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer policyStore.Close()
	store, err := openWorkspaceRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	northstar := json.RawMessage(`{"name":"Northstar scenario"}`)
	redwood := json.RawMessage(`{"name":"Redwood scenario"}`)
	if _, err := store.Create(ctx, "northstar", "scenario", "shared-flow", northstar, "test", "Initial"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(ctx, "redwood", "scenario", "shared-flow", redwood, "test", "Initial"); err != nil {
		t.Fatal(err)
	}
	updated := json.RawMessage(`{"name":"Northstar revised"}`)
	if _, err := store.AppendRevision(ctx, "northstar", "scenario", "shared-flow", 1, updated, "test", "Update"); err != nil {
		t.Fatal(err)
	}
	first, err := store.Get(ctx, "northstar", "scenario", "shared-flow")
	if err != nil || first.Revision != 2 || string(first.Content) != string(updated) {
		t.Fatalf("northstar artifact = %+v, %v", first, err)
	}
	second, err := store.Get(ctx, "redwood", "scenario", "shared-flow")
	if err != nil || second.Revision != 1 || string(second.Content) != string(redwood) {
		t.Fatalf("redwood artifact = %+v, %v", second, err)
	}
	if _, err := store.Get(ctx, "northstar", "directory", "shared-flow"); !errors.Is(err, errWorkspaceArtifactNotFound) {
		t.Fatalf("cross-kind lookup error = %v", err)
	}
	if _, err := store.AppendRevision(ctx, "northstar", "scenario", "shared-flow", 1, updated, "test", "stale"); !errors.Is(err, errWorkspaceRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
	published, err := store.Publish(ctx, "northstar", "scenario", "shared-flow", 2)
	if err != nil || published.PublishedRevision != 2 || published.PublishedAt == nil {
		t.Fatalf("published artifact = %+v, %v", published, err)
	}
	if _, err := store.Publish(ctx, "northstar", "scenario", "shared-flow", 1); !errors.Is(err, errWorkspaceRevisionConflict) {
		t.Fatalf("stale publish error = %v", err)
	}
	revisions, err := store.ListRevisions(ctx, "northstar", "scenario", "shared-flow")
	if err != nil || len(revisions) != 2 || revisions[0].Revision != 2 || revisions[1].Revision != 1 {
		t.Fatalf("revision list = %+v, %v", revisions, err)
	}
}

func TestWorkspaceRepositoryKeepsRevisionsImmutable(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := openWorkspaceRepository(filepath.Join(directory, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.Create(ctx, "northstar", "directory", "directory", json.RawMessage(`{"ldif":"seed"}`), "test", "Initial"); err != nil {
		t.Fatal(err)
	}
	_, err = store.db.Exec(`UPDATE workspace_artifact_revisions SET summary='changed' WHERE organization_id='northstar'`)
	if err == nil {
		t.Fatal("workspace revision update unexpectedly succeeded")
	}
	_, err = store.db.Exec(`DELETE FROM workspace_artifact_revisions WHERE organization_id='northstar'`)
	if err == nil {
		t.Fatal("workspace revision deletion unexpectedly succeeded")
	}
}

func TestWorkspaceRepositoryArchivesArtifactsWithoutDeletingRevisions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := openWorkspaceRepository(filepath.Join(directory, "workspace.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	content := json.RawMessage(`{"name":"Archive me"}`)
	if _, err := store.Create(ctx, "northstar", "scenario", "archive-me", content, "test", "Initial"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendRevision(ctx, "northstar", "scenario", "archive-me", 1, content, "test", "Update"); err != nil {
		t.Fatal(err)
	}
	if err := store.Archive(ctx, "northstar", "scenario", "archive-me", 1); !errors.Is(err, errWorkspaceRevisionConflict) {
		t.Fatalf("stale archive error = %v", err)
	}
	if err := store.Archive(ctx, "northstar", "scenario", "archive-me", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "northstar", "scenario", "archive-me"); !errors.Is(err, errWorkspaceArtifactNotFound) {
		t.Fatalf("archived artifact lookup error = %v", err)
	}
	artifacts, err := store.List(ctx, "northstar", "scenario")
	if err != nil || len(artifacts) != 0 {
		t.Fatalf("listed artifacts = %+v, %v", artifacts, err)
	}
	var revisions int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM workspace_artifact_revisions WHERE organization_id='northstar' AND artifact_id='archive-me'`).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatalf("retained revisions = %d, %v; want 2", revisions, err)
	}
	if _, err := store.Create(ctx, "northstar", "scenario", "archive-me", content, "test", "Reuse ID"); !errors.Is(err, errWorkspaceArtifactExists) {
		t.Fatalf("reused archived ID error = %v", err)
	}
}
