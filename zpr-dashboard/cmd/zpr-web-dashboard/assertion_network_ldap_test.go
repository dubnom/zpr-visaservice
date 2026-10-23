package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
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
	"github.com/go-ldap/ldif"
)

type packetALDAPFixture struct {
	endpoint    string
	certificate []byte
	mode        atomic.Int32
	binds       atomic.Int64
	searches    atomic.Int64
}

func packetADirectory(t *testing.T) string {
	t.Helper()
	path := filepath.Join("testdata", ".packet-a-"+strings.ReplaceAll(t.Name(), "/", "-"))
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(path); err != nil {
			t.Error(err)
		}
	})
	return path
}

func packetALDAP(t *testing.T, name, baseDN string) *packetALDAPFixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "trusted-sources", name+".ldif"))
	if err != nil {
		t.Fatal(err)
	}
	records, err := ldif.Parse(string(data))
	if err != nil {
		t.Fatal(err)
	}
	certificateServer := httptest.NewTLSServer(nil)
	certificates := certificateServer.TLS.Certificates
	certificate := certificateServer.Certificate().Raw
	certificateServer.Close()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: certificates, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &packetALDAPFixture{endpoint: "ldaps://" + listener.Addr().String(), certificate: certificate}
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
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				for {
					packet, err := ber.ReadPacket(conn)
					if err != nil {
						return
					}
					id := packet.Children[0].Value.(int64)
					request := packet.Children[1]
					switch request.Tag {
					case ldap.ApplicationBindRequest:
						fixture.binds.Add(1)
						code := int64(0)
						if request.Children[1].Value != "cn=zpr-reader,"+baseDN || request.Children[2].Data.String() != "fixture-bind-secret" || fixture.mode.Load() == 1 {
							code = 49
						}
						packetAResult(conn, id, ldap.ApplicationBindResponse, code)
					case ldap.ApplicationSearchRequest:
						fixture.searches.Add(1)
						if request.Children[0].Value != baseDN || request.Children[1].Value != int64(ldap.ScopeWholeSubtree) || request.Children[2].Value != int64(ldap.NeverDerefAliases) || request.Children[3].Value != int64(2000) || request.Children[4].Value != int64(8) {
							t.Error("LDAP search escaped configured scope or limits")
						}
						for _, attribute := range request.Children[7].Children {
							if assertionSensitiveAttribute(strings.ToLower(attribute.Value.(string))) {
								t.Error("requested sensitive attribute")
							}
						}
						switch fixture.mode.Load() {
						case 2:
							packetAResult(conn, id, ldap.ApplicationSearchResultDone, ldap.LDAPResultSizeLimitExceeded)
							continue
						case 4:
							_, _ = ber.ReadPacket(conn)
							return
						case 5:
							op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultReference, nil, "")
							op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "ldaps://other.test", ""))
							packetASend(conn, id, op)
						}
						for _, record := range records.Entries {
							entry := record.Entry
							dn := entry.DN
							if fixture.mode.Load() == 3 {
								dn = "uid=outside,dc=other"
							}
							op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ldap.ApplicationSearchResultEntry, nil, "")
							op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, dn, ""))
							attrs := ber.NewSequence("")
							for _, attribute := range entry.Attributes {
								item := ber.NewSequence("")
								item.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, attribute.Name, ""))
								values := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
								for _, value := range attribute.Values {
									if fixture.mode.Load() == 6 && attribute.Name == "userPassword" {
										value = strings.Repeat("x", maxPolicyLDAPScanOutput+1)
									}
									values.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, value, ""))
								}
								item.AppendChild(values)
								attrs.AppendChild(item)
							}
							op.AppendChild(attrs)
							packetASend(conn, id, op)
						}
						packetAResult(conn, id, ldap.ApplicationSearchResultDone, 0)
					case ldap.ApplicationUnbindRequest:
						return
					default:
						t.Errorf("unexpected LDAP operation (writes forbidden): %d", request.Tag)
						return
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); workers.Wait() })
	return fixture
}

func packetASend(conn net.Conn, id int64, op *ber.Packet) {
	packet := ber.NewSequence("")
	packet.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, id, ""))
	packet.AppendChild(op)
	_, _ = conn.Write(packet.Bytes())
}

func packetAResult(conn net.Conn, id int64, tag ber.Tag, code int64) {
	op := ber.Encode(ber.ClassApplication, ber.TypeConstructed, tag, nil, "")
	op.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, code, ""))
	op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", ""))
	op.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "fixture-bind-secret /private/bind-file", ""))
	packetASend(conn, id, op)
}

func packetARuntime(t *testing.T, company, lab *packetALDAPFixture) (*assertionRuntime, string) {
	t.Helper()
	directory := packetADirectory(t)
	var config struct {
		DefaultSource string                  `json:"default_source"`
		Sources       []assertionSourceConfig `json:"sources"`
	}
	data, err := os.ReadFile(filepath.Join("testdata", "trusted-sources", "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(directory, "bind-password")
	if err := os.WriteFile(secretFile, []byte("fixture-bind-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for index, fixture := range []*packetALDAPFixture{company, lab} {
		entry := &config.Sources[index]
		entry.URL = fixture.endpoint
		entry.CAFile = filepath.Join(directory, entry.Name+"-ca.pem")
		if err := os.WriteFile(entry.CAFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: fixture.certificate}), 0600); err != nil {
			t.Fatal(err)
		}
		scope := entry.Organizations["northstar"]
		scope.BindPasswordFile = secretFile
		entry.Organizations["northstar"] = scope
	}
	data, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "sources.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ZPR_ASSERTION_STORE_FILE", "ZPR_ASSERTION_LDAP_CONTAINER", "ZPR_ASSERTION_LDAP_BASE_DN", "ZPR_ASSERTION_LDAP_BIND_DN"} {
		t.Setenv(name, "")
	}
	t.Setenv("ZPR_ASSERTION_SOURCES_FILE", path)
	runtime, err := newAssertionRuntime()
	if err != nil {
		t.Fatal(err)
	}
	runtime.settingsStore = &memoryAssertionSettingsStore{response: assertionSettingsResponse{
		OrganizationID: "northstar", OrganizationName: "Northstar", BaseDN: "dc=context",
		Settings: assertionSettings{Revision: 1, IntervalSeconds: 60},
	}}
	if err := runtime.refreshSettings(context.Background()); err != nil {
		t.Fatal(err)
	}
	return runtime, secretFile
}

func packetARead(runtime *assertionRuntime, query string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/assertions/source"+query, nil)
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	runtime.handleSource(response, request)
	return response
}

func TestNetworkLDAPMultipleSourcesWithoutSimulator(t *testing.T) {
	for _, simulator := range []string{"unset", "invalid"} {
		t.Run(simulator, func(t *testing.T) {
			for _, name := range []string{"SIMULATOR_URL", "SIMULATION_MANIFEST", "SIMULATION_CONTAINER", "SIMULATOR_ORGANIZATIONS_DIR"} {
				t.Setenv(name, "unreachable-invalid-input")
				if simulator == "unset" {
					if err := os.Unsetenv(name); err != nil {
						t.Fatal(err)
					}
				}
			}
			company := packetALDAP(t, "company_directory", "dc=company")
			lab := packetALDAP(t, "lab_directory", "dc=lab")
			runtime, secretFile := packetARuntime(t, company, lab)
			expression := `people from "company_directory" where source("lab_directory").attribute("salary") > source("company_directory").attribute("salary");`
			run, err := runtime.evaluate(context.Background(), expression, 1)
			if err != nil || run.Status != "pass" || company.searches.Load() != 1 || lab.searches.Load() != 1 {
				t.Fatalf("two source run=%+v err=%v", run, err)
			}
			for _, name := range []string{"company_directory", "lab_directory"} {
				response := packetARead(runtime, "?source="+name)
				if response.Code != http.StatusOK {
					t.Fatalf("source read: %d %s", response.Code, response.Body)
				}
				var body map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if body["source_id"] != name || body["organization_id"] != "northstar" || body["kind"] != "ldap" {
					t.Fatalf("missing identity: %v", body)
				}
				baseDN, salary := "dc=company", "50000"
				if name == "lab_directory" {
					baseDN, salary = "dc=lab", "60000"
				}
				if name == "company_directory" {
					packetAContract(t, body, "source-read-company.json")
				}
				if body["base_dn"] != baseDN || !strings.Contains(response.Body.String(), salary) {
					t.Fatalf("cross-source snapshot: %s", response.Body)
				}
				for _, secret := range []string{"fixture-bind-secret", "fixture-must-not-leak", secretFile, company.endpoint, lab.endpoint, "userpassword"} {
					if strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(secret)) {
						t.Fatalf("source response leaked %q", secret)
					}
				}
			}
			packetAContract(t, runtime.status(), "source-status.json")
			restarted, err := newAssertionRuntime()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := restarted.readDirectory(context.Background(), "northstar", "dc=ignored", "cn=ignored"); err != nil {
				t.Fatalf("restart lost sources: %v", err)
			}
			metadata, _ := json.Marshal(runtime.status())
			for _, secret := range []string{"fixture-bind-secret", secretFile, company.endpoint, lab.endpoint, "bind_password_file", "ca_file", "cn=zpr-reader"} {
				if strings.Contains(string(metadata), secret) {
					t.Fatalf("metadata leaked %q", secret)
				}
			}
			lab.mode.Store(1)
			companyReads := company.binds.Load()
			response := packetARead(runtime, "?source=lab_directory")
			if response.Code != http.StatusBadGateway || company.binds.Load() != companyReads || strings.Contains(response.Body.String(), "fixture-bind-secret") {
				t.Fatalf("failure substituted or leaked: %d %s", response.Code, response.Body)
			}
			if response := packetARead(runtime, "?source=company_directory"); response.Code != http.StatusOK {
				t.Fatalf("unrelated source unavailable: %s", response.Body)
			}
			run, err = runtime.evaluate(context.Background(), expression, 1)
			if err != nil || run.Status != "error" || len(run.Results) != 0 {
				t.Fatalf("failed dependency evaluated: %+v %v", run, err)
			}
			for _, query := range []string{"?source=missing", "?source=lab_directory&source=company_directory", "?unknown=1", "?source=", "?source=%zz", "?source=company_directory;other=lab_directory"} {
				if response := packetARead(runtime, query); response.Code < 400 {
					t.Fatalf("accepted %s", query)
				}
			}
			runtime.settingsStore.(*memoryAssertionSettingsStore).response.OrganizationID = "other"
			beforeCompany, beforeLab := company.binds.Load(), lab.binds.Load()
			if response := packetARead(runtime, "?source=company_directory"); response.Code != http.StatusNotFound {
				t.Fatalf("cross-organization source: %d", response.Code)
			}
			if company.binds.Load() != beforeCompany || lab.binds.Load() != beforeLab || len(runtime.sourceMetadata()) != 0 {
				t.Fatal("cross-organization provider was contacted or exposed")
			}
		})
	}
}

func TestNetworkLDAPConfigurationAndCredentials(t *testing.T) {
	directory := packetADirectory(t)
	passwordFile := filepath.Join(directory, "bind-password")
	if err := os.WriteFile(passwordFile, []byte("fixture-bind-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	valid := func() assertionSourceConfig {
		return assertionSourceConfig{Name: "company_directory", Kind: "ldap", URL: "ldaps://127.0.0.1:636", Attributes: []string{"uid"}, Organizations: map[string]assertionSourceScope{"northstar": {BaseDN: "dc=company", BindDN: "cn=zpr-reader,dc=company", BindPasswordFile: passwordFile}}}
	}
	for _, test := range []struct {
		name   string
		change func(*assertionSourceConfig)
	}{
		{"cleartext", func(e *assertionSourceConfig) { e.URL = "ldap://example.test" }},
		{"invalid port", func(e *assertionSourceConfig) { e.URL = "ldaps://example.test:65536" }},
		{"empty port", func(e *assertionSourceConfig) { e.URL = "ldaps://example.test:" }},
		{"URL credentials", func(e *assertionSourceConfig) { e.URL = "ldaps://secret:secret@example.test" }},
		{"path", func(e *assertionSourceConfig) { e.URL += "/dc=company" }},
		{"query", func(e *assertionSourceConfig) { e.URL += "?secret=secret" }},
		{"empty query", func(e *assertionSourceConfig) { e.URL += "?" }},
		{"fragment", func(e *assertionSourceConfig) { e.URL += "#secret" }},
		{"container", func(e *assertionSourceConfig) { e.Container = "forbidden" }},
		{"token", func(e *assertionSourceConfig) { e.TokenFile = passwordFile }},
		{"no scopes", func(e *assertionSourceConfig) { e.Organizations = nil }},
		{"empty organization", func(e *assertionSourceConfig) { e.Organizations[""] = e.Organizations["northstar"] }},
		{"no attributes", func(e *assertionSourceConfig) { e.Attributes = nil }},
		{"sensitive attributes", func(e *assertionSourceConfig) { e.Attributes = []string{"userPassword"} }},
		{"bad base", func(e *assertionSourceConfig) {
			scope := e.Organizations["northstar"]
			scope.BaseDN = "not-a-DN"
			e.Organizations["northstar"] = scope
		}},
		{"empty base", func(e *assertionSourceConfig) {
			scope := e.Organizations["northstar"]
			scope.BaseDN = ""
			e.Organizations["northstar"] = scope
		}},
		{"empty bind", func(e *assertionSourceConfig) {
			scope := e.Organizations["northstar"]
			scope.BindDN = ""
			e.Organizations["northstar"] = scope
		}},
		{"missing credential", func(e *assertionSourceConfig) {
			scope := e.Organizations["northstar"]
			scope.BindPasswordFile = filepath.Join(directory, "missing-secret")
			e.Organizations["northstar"] = scope
		}},
		{"scope URL", func(e *assertionSourceConfig) {
			scope := e.Organizations["northstar"]
			scope.URL = e.URL
			e.Organizations["northstar"] = scope
		}},
		{"bad CA", func(e *assertionSourceConfig) { e.CAFile = passwordFile }},
	} {
		t.Run(test.name, func(t *testing.T) {
			entry := valid()
			test.change(&entry)
			data, _ := json.Marshal(map[string]any{"default_source": entry.Name, "sources": []assertionSourceConfig{entry}})
			path := filepath.Join(directory, "sources.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			err := (&assertionRuntime{}).configureSources(path)
			if err == nil {
				t.Fatal("accepted invalid network LDAP configuration")
			}
			if strings.Contains(err.Error(), directory) || strings.Contains(err.Error(), "fixture-bind-secret") {
				t.Fatalf("configuration error leaked private data: %v", err)
			}
		})
	}
	for _, data := range []string{"", "secret\nsecond-line", "secret\x00", strings.Repeat("s", 4097)} {
		if err := os.WriteFile(passwordFile, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := assertionLDAPCredential(passwordFile); err == nil {
			t.Fatal("accepted malformed credential")
		}
	}
	if err := os.WriteFile(passwordFile, []byte("fixture-bind-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(passwordFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := assertionLDAPCredential(passwordFile); err == nil {
		t.Fatal("accepted unprotected credential")
	}
	if _, err := assertionLDAPCredential(directory); err == nil {
		t.Fatal("accepted directory credential")
	}
}

func TestNetworkLDAPTransportFailures(t *testing.T) {
	company := packetALDAP(t, "company_directory", "dc=company")
	lab := packetALDAP(t, "lab_directory", "dc=lab")
	runtime, passwordFile := packetARuntime(t, company, lab)
	source := runtime.sources["company_directory"]
	scope := source.organizations["northstar"]
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: company.certificate})) {
		t.Fatal("invalid test CA")
	}
	for _, config := range []*tls.Config{
		{MinVersion: tls.VersionTLS12, RootCAs: x509.NewCertPool(), ServerName: "127.0.0.1"},
		{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "wrong.example.test"},
	} {
		before := company.binds.Load()
		if _, err := readNetworkAssertionLDAP(context.Background(), strings.TrimPrefix(company.endpoint, "ldaps://"), config, scope, []string{"uid"}); err == nil {
			t.Fatal("accepted invalid TLS")
		}
		if company.binds.Load() != before {
			t.Fatal("credential transmitted before certificate verification")
		}
	}
	for _, mode := range []int32{1, 2, 3, 5, 6} {
		company.mode.Store(mode)
		if _, err := source.read(context.Background(), "northstar", "dc=ignored", "cn=ignored"); err == nil || strings.Contains(err.Error(), "fixture-bind-secret") || strings.Contains(err.Error(), "/private") {
			t.Fatalf("unsafe failure for mode %d: %v", mode, err)
		}
	}
	company.mode.Store(0)
	if err := os.WriteFile(passwordFile, []byte("wrong-password"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.read(context.Background(), "northstar", "", ""); err == nil {
		t.Fatal("accepted wrong bind password")
	}
	if err := os.WriteFile(passwordFile, []byte("fixture-bind-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := source.read(context.Background(), "northstar", "", ""); err != nil {
		t.Fatalf("credential rotation not reread: %v", err)
	}
	company.mode.Store(4)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := source.read(ctx, "northstar", "", ""); err == nil || time.Since(start) > time.Second {
		t.Fatalf("cancel did not bound search: %v", err)
	}
	before := company.binds.Load()
	if err := os.Remove(passwordFile); err != nil {
		t.Fatal(err)
	}
	if _, err := source.read(context.Background(), "northstar", "", ""); err == nil || company.binds.Load() != before {
		t.Fatal("missing rotated credential contacted source")
	}
}

func packetAContract(t *testing.T, actual any, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "trusted-sources", name))
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(serialized, &body); err != nil {
		t.Fatal(err)
	}
	for key, value := range expected {
		if !reflect.DeepEqual(body[key], value) {
			t.Errorf("contract fixture %s field %s: got %v want %v", name, key, body[key], value)
		}
	}
}

func TestNetworkLDAPOrganizationApplicableSources(t *testing.T) {
	company := packetALDAP(t, "company_directory", "dc=company")
	lab := packetALDAP(t, "lab_directory", "dc=lab")
	runtime, _ := packetARuntime(t, company, lab)
	runtime.sources["foreign_directory"] = assertionNamedSource{
		kind: "ldap", organizations: map[string]assertionSourceScope{"other": {BaseDN: "dc=other"}},
		read: func(context.Context, string, string, string) (assertionDirectory, error) {
			t.Fatal("contacted foreign organization source")
			return assertionDirectory{}, nil
		},
	}
	directory, err := runtime.readDirectory(context.Background(), "northstar", "dc=ignored", "cn=ignored")
	if err != nil || len(directory.Sources) != 2 || len(runtime.sourceMetadata()) != 2 {
		t.Fatalf("applicable sources=%v err=%v", directory.Sources, err)
	}
	before := company.binds.Load() + lab.binds.Load()
	runtime.defaultSource = "foreign_directory"
	if _, err := runtime.readDirectory(context.Background(), "northstar", "", ""); err == nil || company.binds.Load()+lab.binds.Load() != before {
		t.Fatal("substituted inaccessible default")
	}
}

func TestNetworkLDAPSelectorPreservesLegacyName(t *testing.T) {
	runtime := &assertionRuntime{defaultSource: "ldap", reader: func(context.Context, string, string) (assertionDirectory, error) {
		return assertionDirectory{People: []string{"alice"}, Groups: map[string][]string{"Operators": {"alice"}}}, nil
	}, settingsStore: &memoryAssertionSettingsStore{response: assertionSettingsResponse{
		OrganizationID: "northstar", BaseDN: "dc=company", Settings: assertionSettings{Revision: 1, IntervalSeconds: 60},
	}}}
	response := packetARead(runtime, "?source=ldap")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"source_id":"ldap"`) {
		t.Fatalf("legacy selector=%d %s", response.Code, response.Body)
	}
	if runtime.sourceMetadata()[0]["source_id"] != "ldap" {
		t.Fatal("legacy name changed")
	}
}
