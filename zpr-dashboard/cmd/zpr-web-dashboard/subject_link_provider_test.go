package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
	"github.com/go-ldap/ldap/v3"
	"neboagency.com/zpr-dashborad/internal/changereview"
)

func subjectReviewDirectory(t *testing.T, mode *atomic.Int32, searches *atomic.Int64) (string, []byte) {
	t.Helper()
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := createTestPolicyLeaf(ca, key, "review-directory", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				for {
					packet, err := ber.ReadPacket(conn)
					if err != nil {
						return
					}
					id, request := packet.Children[0].Value.(int64), packet.Children[1]
					switch request.Tag {
					case ldap.ApplicationBindRequest:
						code := int64(0)
						expected := "fixture-secret"
						if mode.Load() == 10 {
							expected = "rotated-secret"
						}
						if request.Children[1].Value != "cn=reader,dc=example" || request.Children[2].Data.String() != expected || mode.Load() == 1 {
							code = 49
						}
						packetAResult(conn, id, ldap.ApplicationBindResponse, code)
					case ldap.ApplicationSearchRequest:
						searches.Add(1)
						expectedFilter, _ := ldap.CompileFilter("(entryUUID=b6a21ab4-5554-4cde-ae65-41a406a3c07f)")
						if request.Children[0].Value != "dc=example" || request.Children[1].Value != int64(ldap.ScopeWholeSubtree) ||
							request.Children[2].Value != int64(ldap.NeverDerefAliases) || request.Children[3].Value != int64(2) ||
							request.Children[4].Value != int64(8) ||
							!reflect.DeepEqual(request.Children[6].Bytes(), expectedFilter.Bytes()) ||
							len(request.Children[7].Children) != 1 || request.Children[7].Children[0].Value != "entryUUID" {
							t.Error("review query leaked selectors/attributes or escaped exact directory limits")
						}
						if mode.Load() == 2 {
							packetAResult(conn, id, ldap.ApplicationSearchResultDone, ldap.LDAPResultSizeLimitExceeded)
							continue
						}
						count := 1
						if mode.Load() == 3 {
							count = 0
						} else if mode.Load() == 4 {
							count = 2
						}
						for range count {
							dn := "uid=renamable-label,dc=example"
							if mode.Load() == 5 {
								dn = "uid=outside,dc=foreign"
							}
							op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultEntry, nil, "")
							op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, dn, ""))
							attrs := ber.NewSequence("")
							if mode.Load() != 6 {
								item := ber.NewSequence("")
								item.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "entryUUID", ""))
								values := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
								uuid := "b6a21ab4-5554-4cde-ae65-41a406a3c07f"
								if mode.Load() == 7 {
									uuid = "b6a21ab4-5554-4cde-ae65-41a406a3c070"
								}
								values.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, uuid, ""))
								if mode.Load() == 8 {
									values.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, uuid, ""))
								}
								item.AppendChild(values)
								attrs.AppendChild(item)
								if mode.Load() == 9 {
									attrs.AppendChild(item)
								}
							}
							op.AppendChild(attrs)
							packetASend(conn, id, op)
						}
						packetAResult(conn, id, ldap.ApplicationSearchResultDone, 0)
					case ldap.ApplicationUnbindRequest:
						return
					default:
						t.Errorf("review provider attempted unsupported/write operation %d", request.Tag)
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); workers.Wait() })
	return "ldaps://" + listener.Addr().String(), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
}

func subjectReviewProviderFixture(t *testing.T) (*subjectLinkProviders, string, *atomic.Int32, *atomic.Int64, changereview.SubjectLinkProposal, changereview.SubjectLinkRegistryCapture) {
	t.Helper()
	t.Setenv("SIMULATION_MANIFEST", "/unavailable/subject-review")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:0")
	mode, searches := &atomic.Int32{}, &atomic.Int64{}
	endpoint, ca := subjectReviewDirectory(t, mode, searches)
	dir := t.TempDir()
	caPath, password := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "bind-password")
	if err := os.WriteFile(caPath, ca, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(password, []byte("fixture-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config := subjectLinkProviderConfig{"production", "company", "1", "directory", endpoint, caPath, "dc=example", "cn=reader,dc=example", password}
	data, err := json.Marshal(map[string]any{"version": 1, "providers": []subjectLinkProviderConfig{config}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "providers.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	providers, err := loadSubjectLinkProviders(path)
	if err != nil {
		t.Fatal(err)
	}
	capture := changereview.SubjectLinkRegistryCapture{
		Revision: strings.Repeat("a", 64),
		Authority: changereview.SubjectLinkAuthority{Organization: "production", ID: "bas", Version: "1",
			Config: changereview.SubjectLinkAuthorityConfig{Kind: "bas_device", Enabled: true}},
		Source: changereview.SubjectLinkSource{Organization: "production", ID: "company", Version: "1",
			Config: changereview.SubjectLinkSourceConfig{ProviderID: "directory", SelectorKind: "ldap_entry_uuid", Enabled: true}},
	}
	proposal := changereview.SubjectLinkProposal{
		Organization: "production", RegistryRevision: capture.Revision,
		Root:   changereview.SubjectLinkRoot{AuthorityID: "bas", AuthorityVersion: "1", SubjectKind: "device", SubjectID: "verified-device"},
		Target: changereview.SubjectLinkTarget{SourceID: "company", SourceVersion: "1", SelectorKind: "ldap_entry_uuid", SubjectID: "b6a21ab4-5554-4cde-ae65-41a406a3c07f"},
	}
	return providers, path, mode, searches, proposal, capture
}

func TestSubjectReviewProviderExactTLSLookupAndFailureRecovery(t *testing.T) {
	providers, _, mode, searches, proposal, capture := subjectReviewProviderFixture(t)
	check, err := providers.validator(capture)
	if err != nil {
		t.Fatal(err)
	}
	for value := int32(0); value <= 10; value++ {
		mode.Store(value)
		err := check(context.Background(), proposal)
		if value == 0 && err != nil || value != 0 && err == nil {
			t.Fatal("directory failure became valid subject evidence", value, err)
		}
	}
	mode.Store(10)
	if err := os.WriteFile(providers.entries[0].config.BindPasswordFile, []byte("rotated-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := check(context.Background(), proposal); err != nil {
		t.Fatal("protected credential rotation did not recover", err)
	}
	before := searches.Load()
	for _, change := range []func(*changereview.SubjectLinkProposal){
		func(p *changereview.SubjectLinkProposal) { p.Organization = "foreign" },
		func(p *changereview.SubjectLinkProposal) { p.Target.SourceID = "other" },
		func(p *changereview.SubjectLinkProposal) { p.Target.SourceVersion = "2" },
		func(p *changereview.SubjectLinkProposal) { p.Root.SubjectKind = "user" },
		func(p *changereview.SubjectLinkProposal) { p.Target.SubjectID = "person@example.test" },
	} {
		invalid := proposal
		change(&invalid)
		if err := check(context.Background(), invalid); !errors.Is(err, changereview.ErrConflict) {
			t.Fatal("invalid selector context accepted", err)
		}
	}
	if searches.Load() != before {
		t.Fatal("invalid identity leaked a directory query")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := check(cancelled, proposal); err == nil {
		t.Fatal("cancelled subject validation succeeded")
	}
	wrongTrust := providers.entries[0].tls.Clone()
	wrongTrust.RootCAs = x509.NewCertPool()
	providers.entries[0].tls = wrongTrust
	untrusted, err := providers.validator(capture)
	if err != nil {
		t.Fatal(err)
	}
	if err := untrusted(context.Background(), proposal); err == nil {
		t.Fatal("unrelated directory trust accepted")
	}
	// The callback captures its independently verified transport at construction.
	if err := check(context.Background(), proposal); err != nil {
		t.Fatal("failed trust probe changed original transport", err)
	}
	password := providers.entries[0].config.BindPasswordFile
	replacement := password + "-replacement"
	if err := os.WriteFile(replacement, []byte("rotated-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(password); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, password); err != nil {
		t.Fatal(err)
	}
	if err := check(context.Background(), proposal); err == nil {
		t.Fatal("symlinked credential accepted after provider startup")
	}
}

func TestSubjectReviewProviderProtectedConfigAndExactResolution(t *testing.T) {
	providers, _, _, _, _, capture := subjectReviewProviderFixture(t)
	for _, change := range []func(*changereview.SubjectLinkRegistryCapture){
		func(c *changereview.SubjectLinkRegistryCapture) { c.Source.Config.ProviderID = "other" },
		func(c *changereview.SubjectLinkRegistryCapture) { c.Source.Version = "2" },
		func(c *changereview.SubjectLinkRegistryCapture) { c.Source.Config.Enabled = false },
		func(c *changereview.SubjectLinkRegistryCapture) { c.Authority.Organization = "foreign" },
	} {
		invalid := capture
		change(&invalid)
		if _, err := providers.validator(invalid); err == nil {
			t.Fatal("provider capture mismap accepted")
		}
	}
}

func TestSubjectReviewProviderDurableRegistryComposition(t *testing.T) {
	providers, _, mode, searches, proposal, _ := subjectReviewProviderFixture(t)
	directory := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "reviews.sqlite")
	store, err := changereview.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	author := changereview.Principal{Issuer: "https://fixture-idp.example", Subject: "proposer"}
	reviewer := changereview.Principal{Issuer: author.Issuer, Subject: "reviewer"}
	if _, err := store.PutSubjectLinkAuthority(ctx, "production", "bas", "",
		changereview.SubjectLinkAuthorityConfig{Kind: "bas_device", ServiceID: "registered-bas", Issuer: "zpr/bas", Enabled: true},
		author, "Register authority"); err != nil {
		t.Fatal(err)
	}
	config := changereview.SubjectLinkSourceConfig{ProviderID: "directory", SelectorKind: "ldap_entry_uuid",
		EnrichmentKeys: []string{"user.department"}, Enabled: true}
	if _, err := store.PutSubjectLinkSource(ctx, "production", "company", "", config, author, "Register source"); err != nil {
		t.Fatal(err)
	}
	capture, err := store.CaptureSubjectLinkRegistry(ctx, "production", proposal.Root, proposal.Target)
	if err != nil {
		t.Fatal(err)
	}
	check, err := providers.validator(capture)
	if err != nil {
		t.Fatal(err)
	}
	proposal.RegistryRevision, proposal.Author, proposal.Reason = capture.Revision, author, "Review exact UUID"
	proposal.NotBefore, proposal.ExpiresAt = time.Now().UTC().Add(-time.Minute), time.Now().UTC().Add(time.Hour)
	proposal.EnrichmentKeys = []string{"user.department"}
	mode.Store(3)
	if _, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, proposal, "submit", check); err == nil {
		t.Fatal("missing exact subject created review")
	}
	mode.Store(0)
	record, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, proposal, "submit", check)
	if err != nil {
		t.Fatal("failed lookup consumed retry key", err)
	}
	approved, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, "production", record.ID, record.Digest, "1",
		capture.Revision, "Independent directory review", reviewer, "approve", check)
	if err != nil || approved.State != "review-approved" {
		t.Fatal("real directory validation failed independent review", approved, err)
	}
	other, err := store.SubmitSubjectLinkWithRegistryOnce(ctx, proposal, "pending", check)
	if err != nil {
		t.Fatal(err)
	}
	config.Enabled = false
	if _, err := store.PutSubjectLinkSource(ctx, "production", "company", "1", config, author, "Disable source"); err != nil {
		t.Fatal(err)
	}
	before := searches.Load()
	if _, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, "production", other.ID, other.Digest, "1",
		capture.Revision, "Stale review", reviewer, "stale-approve", check); !errors.Is(err, changereview.ErrConflict) || searches.Load() != before {
		t.Fatal("disabled source approval reached directory", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = changereview.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mode.Store(1)
	replayed, err := store.ApproveSubjectLinkWithRegistryOnce(ctx, "production", record.ID, record.Digest, "1",
		capture.Revision, "Independent directory review", reviewer, "approve", check)
	if err != nil || !reflect.DeepEqual(replayed, approved) || searches.Load() != before {
		t.Fatal("historical approval replay queried disabled/outage directory", replayed, err)
	}
}
func TestSubjectReviewProviderStrictConfigFiles(t *testing.T) {
	_, path, _, _, _, _ := subjectReviewProviderFixture(t)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte(`{"version":1,"version":1,"providers":[]}`),
		append(append([]byte{}, original...), []byte(`{}`)...),
		[]byte(strings.Replace(string(original), `"provider_id"`, `"Provider_ID"`, 1)),
		[]byte(strings.Replace(string(original), `"source_version":"1"`, `"source_version":"01"`, 1)),
		[]byte(strings.Replace(string(original), `"ldaps://`, `"ldap://`, 1)),
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadSubjectLinkProviders(path); err == nil {
			t.Fatal("invalid protected provider config accepted")
		}
	}
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSubjectLinkProviders(path); err == nil {
		t.Fatal("nonprivate config accepted")
	}
}
