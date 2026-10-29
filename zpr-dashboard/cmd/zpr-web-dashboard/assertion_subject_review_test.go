//go:build !production

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
)

func subjectReviewRuntime(t *testing.T, fixture *subjectLDAPFixture) (*assertionRuntime, *changereview.Store, changereview.SubjectLinkProposal, string) {
	t.Helper()
	config := assertionSourcesConfig{DefaultSource: fixture.config.Name, Sources: []assertionSourceConfig{fixture.config}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "sources.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	runtime := &assertionRuntime{organizationID: "production", sourcesPath: path,
		sourcesConfig: config, sourcesRevision: sourceConfigRevision(data)}
	if err := runtime.buildSources(config); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(t.TempDir(), "private", "reviews.sqlite")
	store, err := changereview.Open(database)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	author := changereview.Principal{Issuer: "https://idp.example", Subject: "author"}
	if _, err := store.PutSubjectLinkAuthority(context.Background(), "production", "bas", "",
		changereview.SubjectLinkAuthorityConfig{Kind: "bas_device", ServiceID: "bas-service", Issuer: "zpr/bas", Enabled: true},
		author, "Register test authority"); err != nil {
		t.Fatal(err)
	}
	p := changereview.SubjectLinkProposal{
		Organization: "production", RegistryRevision: runtime.sourcesRevision,
		Root: changereview.SubjectLinkRoot{AuthorityID: "bas", AuthorityVersion: "1", SubjectKind: "device", SubjectID: "device"},
		Target: changereview.SubjectLinkTarget{SourceID: fixture.config.Name, SourceVersion: "1",
			SelectorKind: "ldap_entry_uuid", SubjectID: subjectLookupUUID},
		EnrichmentKeys: []string{"mail"}, Author: author, Reason: "Review exact subject",
		NotBefore: time.Now().UTC().Add(-time.Minute), ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	return runtime, store, p, database
}

func TestSubjectSourceReviewExactLookupRetryAndRestart(t *testing.T) {
	fixture := subjectLDAP(t)
	runtime, store, p, path := subjectReviewRuntime(t, fixture)
	ctx := context.Background()
	submitted, err := runtime.submitSubjectLinkReview(ctx, store, p, "submit")
	if err != nil || fixture.searches.Load() != 1 {
		t.Fatal(submitted, err, fixture.searches.Load())
	}
	reviewer := changereview.Principal{Issuer: p.Author.Issuer, Subject: "reviewer"}
	approve := func(key string) (changereview.SubjectLinkReview, error) {
		return runtime.approveSubjectLinkReview(ctx, store, p.Organization, submitted.ID, submitted.Digest, "1",
			p.RegistryRevision, "Review", reviewer, key)
	}
	fixture.mode.Store(1)
	if _, err := approve("decision"); err == nil {
		t.Fatal("missing LDAP subject approved")
	}
	loaded, err := store.GetSubjectLink(ctx, p.Organization, submitted.ID)
	if err != nil || !reflect.DeepEqual(loaded, submitted) {
		t.Fatal("failed lookup changed evidence", loaded, err)
	}
	fixture.mode.Store(0)
	approved, err := approve("decision")
	if err != nil || approved.State != "review-approved" || len(approved.Events) != 2 {
		t.Fatal("same key could not retry recovered LDAP", approved, err)
	}
	count := fixture.searches.Load()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = changereview.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// A successful historical receipt is retrievable without provider/file
	// availability; every future HTTP attempt still needs fresh authorization.
	fixture.mode.Store(8)
	if err := os.Remove(runtime.sourcesPath); err != nil {
		t.Fatal(err)
	}
	replayed, err := runtime.submitSubjectLinkReview(ctx, store, p, "submit")
	if err != nil || !reflect.DeepEqual(replayed, submitted) {
		t.Fatal(replayed, err)
	}
	replayed, err = approve("decision")
	if err != nil || !reflect.DeepEqual(replayed, approved) || fixture.searches.Load() != count {
		t.Fatal("historical receipt queried provider or changed", replayed, err)
	}
	if _, err := runtime.submitSubjectLinkReview(ctx, store, p, "new"); err == nil {
		t.Fatal("new mutation accepted unavailable source registry")
	}
	p.Reason = "Changed request"
	if _, err := runtime.submitSubjectLinkReview(ctx, store, p, "submit"); !errors.Is(err, changereview.ErrConflict) {
		t.Fatal("historical retry accepted changed content", err)
	}
}

func TestSubjectSourceReviewRejectsInvalidContextBeforeLookup(t *testing.T) {
	fixture := subjectLDAP(t)
	runtime, store, p, _ := subjectReviewRuntime(t, fixture)
	for name, mutate := range map[string]func(*changereview.SubjectLinkProposal){
		"organization": func(p *changereview.SubjectLinkProposal) { p.Organization = "foreign" },
		"revision":     func(p *changereview.SubjectLinkProposal) { p.RegistryRevision = sourceConfigRevision([]byte("other")) },
		"version":      func(p *changereview.SubjectLinkProposal) { p.Target.SourceVersion = "2" },
		"source":       func(p *changereview.SubjectLinkProposal) { p.Target.SourceID = "missing" },
		"attribute":    func(p *changereview.SubjectLinkProposal) { p.EnrichmentKeys = []string{"uid"} },
		"mapping":      func(p *changereview.SubjectLinkProposal) { p.EnrichmentKeys = []string{"user.mail"} },
		"authority":    func(p *changereview.SubjectLinkProposal) { p.Root.AuthorityVersion = "2" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := p
			mutate(&candidate)
			if _, err := runtime.submitSubjectLinkReview(context.Background(), store, candidate, name); err == nil {
				t.Fatal("invalid source context accepted")
			}
			if fixture.searches.Load() != 0 {
				t.Fatal("invalid source context performed LDAP queries")
			}
		})
	}
	runtime.sourcesConfig.Sources[0].SubjectSelector = ""
	if _, err := runtime.submitSubjectLinkReview(context.Background(), store, p, "undeclared"); err == nil {
		t.Fatal("undeclared source accepted")
	}
}

func TestSubjectSourceReviewDetectsExternalEditDuringLookup(t *testing.T) {
	fixture := subjectLDAP(t)
	runtime, store, p, _ := subjectReviewRuntime(t, fixture)
	fixture.mode.Store(9)
	result := make(chan error, 1)
	go func() {
		_, err := runtime.submitSubjectLinkReview(context.Background(), store, p, "submit")
		result <- err
	}()
	select {
	case <-fixture.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("lookup did not start")
	}
	if err := os.WriteFile(runtime.sourcesPath, []byte(`{"external_change":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	close(fixture.release)
	if err := <-result; !errors.Is(err, changereview.ErrConflict) {
		t.Fatal("external registry edit went undetected", err)
	}
	// Failed validation leaves no semantic receipt: restoring the exact
	// registry allows the same key to make its first successful mutation.
	data, err := json.Marshal(runtime.sourcesConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.sourcesPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	fixture.mode.Store(0)
	if _, err := runtime.submitSubjectLinkReview(context.Background(), store, p, "submit"); err != nil {
		t.Fatal("failed source check consumed retry key", err)
	}
}

func TestSubjectSourceReviewFencesManagedEditsThroughCommit(t *testing.T) {
	fixture := subjectLDAP(t)
	runtime, store, p, _ := subjectReviewRuntime(t, fixture)
	fixture.mode.Store(9)
	result := make(chan error, 1)
	go func() {
		_, err := runtime.submitSubjectLinkReview(context.Background(), store, p, "submit")
		result <- err
	}()
	select {
	case <-fixture.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("lookup did not start")
	}
	if runtime.mu.TryLock() {
		runtime.mu.Unlock()
		t.Fatal("lookup did not hold managed-edit fence")
	}
	edited := make(chan error, 1)
	go func() {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		config := runtime.sourcesConfig
		config.Sources = append([]assertionSourceConfig(nil), config.Sources...)
		config.Sources[0].Version++
		config.Sources[0].SubjectSelector = ""
		if err := runtime.persistSources(config); err != nil {
			edited <- err
			return
		}
		runtime.sourcesConfig = config
		edited <- nil
	}()
	close(fixture.release)
	if err := <-result; err != nil {
		t.Fatal("review preceding managed edit failed", err)
	}
	if err := <-edited; err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.submitSubjectLinkReview(context.Background(), store, p, "after-edit"); !errors.Is(err, changereview.ErrConflict) {
		t.Fatal("obsolete source context accepted after managed edit", err)
	}
}
