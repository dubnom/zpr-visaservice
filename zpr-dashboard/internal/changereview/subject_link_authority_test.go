package changereview

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSubjectLinkAuthorityRegistrationCaptureAndRestart(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/manifest.json")
	store, path := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	config := SubjectLinkAuthorityConfig{Kind: "bas_device", ServiceID: "bas-service", Issuer: "zpr/bas", Enabled: true}
	a, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", config, p.Author, "Register BAS")
	if err != nil || a.Version != "1" || a.Config != config {
		t.Fatal(a, err)
	}
	captured, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root)
	if err != nil || captured != a {
		t.Fatal(captured, err)
	}
	event, err := store.GetSubjectLinkAuthorityEvent(ctx, p.Organization, a.ID, "1")
	if err != nil || event.Authority != a || event.Actor != p.Author || event.Reason != "Register BAS" || event.At.IsZero() {
		t.Fatal(event, err)
	}
	unchanged, err := store.PutSubjectLinkAuthority(ctx, p.Organization, a.ID, "1", config, p.Author, "No config change")
	if err != nil || unchanged != a {
		t.Fatal("identical CAS edit should preserve version", unchanged, err)
	}
	config.Enabled = false
	disabled, err := store.PutSubjectLinkAuthority(ctx, p.Organization, a.ID, "1", config, p.Author, "Disable")
	if err != nil || disabled.Version != "2" {
		t.Fatal(disabled, err)
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, "alias", "", config, p.Author, "Reuse disabled mapping"); !errors.Is(err, ErrConflict) {
		t.Fatal("disabled mapping was aliased", err)
	}
	disableEvent, err := store.GetSubjectLinkAuthorityEvent(ctx, p.Organization, a.ID, "2")
	if err != nil || disableEvent.Authority != disabled || disableEvent.Reason != "Disable" {
		t.Fatal("missing disable evidence", disableEvent, err)
	}
	if _, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root); !errors.Is(err, ErrConflict) {
		t.Fatal("old version captured after disable", err)
	}
	p.Root.AuthorityVersion = "2"
	if _, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root); !errors.Is(err, ErrConflict) {
		t.Fatal("disabled current version captured", err)
	}
	config.Enabled, config.Issuer = true, "zpr/bas-new"
	replaced, err := store.PutSubjectLinkAuthority(ctx, p.Organization, a.ID, "2", config, p.Author, "Replace issuer and enable")
	if err != nil || replaced.Version != "3" {
		t.Fatal(replaced, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p.Root.AuthorityVersion = "3"
	captured, err = store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root)
	if err != nil || captured != replaced {
		t.Fatal("restart lost authority", captured, err)
	}
	old, err := store.GetSubjectLinkAuthorityEvent(ctx, p.Organization, a.ID, "1")
	if err != nil || !reflect.DeepEqual(old, event) {
		t.Fatal("history changed", old, err)
	}
	for _, query := range []string{
		"UPDATE subject_link_authorities SET id='forged'",
		"UPDATE subject_link_authorities SET organization='foreign'",
		"DELETE FROM subject_link_authorities",
		"UPDATE subject_link_authority_events SET evidence='forged'",
		"DELETE FROM subject_link_authority_events",
	} {
		if _, err := store.db.Exec(query); err == nil {
			t.Fatal("immutable authority evidence modified", query)
		}
	}
}

func TestSubjectLinkAuthorityScopeValidationAndConflicts(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	base := SubjectLinkAuthorityConfig{"bas_device", "bas-service", "opaque-issuer", true}
	put := func(org, id, version string, config SubjectLinkAuthorityConfig) error {
		_, err := store.PutSubjectLinkAuthority(ctx, org, id, version, config, p.Author, "Configure authority")
		return err
	}
	if err := put(p.Organization, p.Root.AuthorityID, "", base); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ org, id, version string }{
		{p.Organization, p.Root.AuthorityID, ""},
		{p.Organization, p.Root.AuthorityID, "2"},
		{p.Organization, "missing", "1"},
		{p.Organization, "alias", ""},
	} {
		if err := put(test.org, test.id, test.version, base); !errors.Is(err, ErrConflict) {
			t.Fatal("expected CAS/duplicate mapping conflict", test, err)
		}
	}
	if err := put("foreign", p.Root.AuthorityID, "", base); err != nil {
		t.Fatal("independent organization registration rejected", err)
	}
	if _, err := store.CaptureSubjectLinkAuthority(ctx, "unregistered", p.Root); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign capture leaked registration", err)
	}
	if _, err := store.GetSubjectLinkAuthorityEvent(ctx, "unregistered", p.Root.AuthorityID, "1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign history leaked", err)
	}
	for name, mutate := range map[string]func(*SubjectLinkAuthorityConfig){
		"bootstrap":        func(c *SubjectLinkAuthorityConfig) { c.Kind = "bootstrap" },
		"local":            func(c *SubjectLinkAuthorityConfig) { c.Kind = "local_visa_service" },
		"human":            func(c *SubjectLinkAuthorityConfig) { c.Kind = "human" },
		"empty service":    func(c *SubjectLinkAuthorityConfig) { c.ServiceID = "" },
		"padded issuer":    func(c *SubjectLinkAuthorityConfig) { c.Issuer += " " },
		"control issuer":   func(c *SubjectLinkAuthorityConfig) { c.Issuer += "\nforged" },
		"oversized issuer": func(c *SubjectLinkAuthorityConfig) { c.Issuer = strings.Repeat("a", 257) },
	} {
		t.Run(name, func(t *testing.T) {
			config := base
			mutate(&config)
			if err := put(p.Organization, "new", "", config); !errors.Is(err, ErrInvalid) {
				t.Fatal("invalid authority accepted", err)
			}
		})
	}
	for _, version := range []string{"0", "01", "-1", "18446744073709551616"} {
		if err := put(p.Organization, p.Root.AuthorityID, version, base); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid expected version accepted", version, err)
		}
	}
	for _, root := range []SubjectLinkRoot{
		{p.Root.AuthorityID, "1", "human", "subject"},
		{p.Root.AuthorityID, "1", "device", ""},
		{p.Root.AuthorityID, "01", "device", "subject"},
		{"../bad", "1", "device", "subject"},
	} {
		if _, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, root); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid root accepted", root, err)
		}
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, "new", "", base, Principal{}, "reason"); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid audit principal accepted", err)
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, "new", "", base, p.Author, ""); !errors.Is(err, ErrInvalid) {
		t.Fatal("missing reason accepted", err)
	}
	for _, org := range []string{"", "../production", strings.Repeat("o", 129)} {
		if err := put(org, "new", "", base); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid organization accepted", org, err)
		}
	}
	if _, err := store.GetSubjectLinkAuthorityEvent(ctx, p.Organization, p.Root.AuthorityID, "01"); !errors.Is(err, ErrInvalid) {
		t.Fatal("noncanonical history version accepted", err)
	}
	// Issuer values are opaque and exact, not URL-normalized or case-folded.
	exact := base
	exact.Issuer = "Opaque-issuer"
	if err := put(p.Organization, "case-distinct", "", exact); err != nil {
		t.Fatal("distinct exact issuer was normalized", err)
	}
}

func TestSubjectLinkAuthorityCaptureDoesNotFreezeLaterRegistration(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	config := SubjectLinkAuthorityConfig{"bas_device", "service", "zpr/bas", true}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", config, p.Author, "Register"); err != nil {
		t.Fatal(err)
	}
	captured, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root)
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.SubmitSubjectLinkOnce(ctx, p, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	config.ServiceID = "replacement-service"
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "1", config, p.Author, "Replace service"); err != nil {
		t.Fatal(err)
	}
	if captured.Version != "1" || captured.Config.ServiceID != "service" {
		t.Fatal("caller capture mutated", captured)
	}
	if _, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root); !errors.Is(err, ErrConflict) {
		t.Fatal("obsolete candidate root still captured", err)
	}
	loaded, err := store.GetSubjectLink(ctx, p.Organization, record.ID)
	if err != nil || loaded.Proposal.Root.AuthorityVersion != "1" || loaded.State != "submitted" {
		t.Fatal("registration edit silently rewrote review evidence", loaded, err)
	}
	p.Root.AuthorityVersion = "2"
	current, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root)
	if err != nil || current.Config != config {
		t.Fatal("replacement registration not captured", current, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.CaptureSubjectLinkAuthority(cancelled, p.Organization, p.Root); err == nil {
		t.Fatal("cancelled capture succeeded")
	}
	if _, err := store.PutSubjectLinkAuthority(cancelled, p.Organization, "new", "", config, p.Author, "Create"); err == nil {
		t.Fatal("cancelled registration succeeded")
	}
}

func TestSubjectLinkAuthorityConcurrentCASAndAuditRollback(t *testing.T) {
	store, _ := subjectLinkStore(t)
	ctx := context.Background()
	p := subjectLinkFixture()
	base := SubjectLinkAuthorityConfig{"bas_device", "service", "issuer", true}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "", base, p.Author, "Register"); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, issuer := range []string{"issuer-a", "issuer-b"} {
		config := base
		config.Issuer = issuer
		go func() {
			_, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "1", config, p.Author, "Replace")
			results <- err
		}()
	}
	first, second := <-results, <-results
	if !((first == nil && errors.Is(second, ErrConflict)) || (second == nil && errors.Is(first, ErrConflict))) {
		t.Fatal("expected one CAS winner", first, second)
	}
	p.Root.AuthorityVersion = "2"
	before, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER fail_authority_audit BEFORE INSERT ON subject_link_authority_events
BEGIN SELECT RAISE(ABORT,'injected authority audit failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "2", base, p.Author, "Edit"); err == nil {
		t.Fatal("update succeeded without audit")
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, "new", "", base, p.Author, "Create"); err == nil {
		t.Fatal("creation succeeded without audit")
	}
	after, err := store.CaptureSubjectLinkAuthority(ctx, p.Organization, p.Root)
	if err != nil || after != before {
		t.Fatal("audit failure escaped rollback", after, err)
	}
	if _, err := store.GetSubjectLinkAuthorityEvent(ctx, p.Organization, p.Root.AuthorityID, "3"); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed edit retained history", err)
	}
	for table, want := range map[string]int{
		"subject_link_authorities": 1, "subject_link_authority_events": 2,
		"subject_link_reviews": 0, "changes": 0, "policy_operations": 0,
	} {
		var count int
		if err := store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != want {
			t.Fatal(table, count, err)
		}
	}
	if _, err := store.db.Exec("DROP TRIGGER fail_authority_audit"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE subject_link_authorities SET version='18446744073709551615'"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutSubjectLinkAuthority(ctx, p.Organization, p.Root.AuthorityID, "18446744073709551615", base, p.Author, "Overflow"); !errors.Is(err, ErrConflict) {
		t.Fatal("authority version overflow accepted", err)
	}
}
