package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testDirectoryLDIF = `version: 1
# organization root
dn: dc=example,dc=test
objectClass: top
objectClass: dcObject
objectClass: organization
dc: example
o: Example

dn: ou=People,dc=example,dc=test
objectClass: organizationalUnit
ou: People

dn: uid=ada,ou=People,dc=example,dc=test
objectClass: inetOrgPerson
objectClass: zprMachine
uid: ada
cn: Ada
sn: Lovelace
zprMachineSecure: TRUE

dn: ou=Service Accounts,dc=example,dc=test
objectClass: organizationalUnit
ou: Service Accounts

dn: cn=zpr-reader,ou=Service Accounts,dc=example,dc=test
objectClass: simpleSecurityObject
objectClass: organizationalRole
cn: zpr-reader
userPassword: DEMO_BIND_PASSWORD_HASH
`

func writeTestDirectoryLDIF(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "directory.ldif")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEvaluationOperatorDirectory(t *testing.T) {
	config := testEvaluationDataPlaneConfig()
	config.Directory = &evaluationDirectory{BaseDN: "dc=example,dc=test", LDIFFiles: []string{writeTestDirectoryLDIF(t, testDirectoryLDIF)}}
	state := filepath.Join(t.TempDir(), "install")
	if _, err := prepareEvaluation(state, config, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := readEvaluation(state, config); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(state, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	ldif := read("directory.ldif")
	for _, want := range []string{"dn: dc=example,dc=test\n", "dn: uid=ada,ou=People,dc=example,dc=test\n", "dn: cn=zpr-reader,dc=example,dc=test\n",
		"dn: ou=Groups,dc=example,dc=test\n", "dn: uid=evaluation-subject,ou=People,dc=example,dc=test\n"} {
		if !strings.Contains(ldif, want) {
			t.Errorf("directory.ldif lacks %q", want)
		}
	}
	if strings.Contains(ldif, "DEMO_BIND_PASSWORD_HASH") || strings.Contains(ldif, "dc=evaluation,dc=zpr") || strings.Count(ldif, "dn: ou=People,") != 1 {
		t.Fatalf("directory.ldif: %s", ldif)
	}
	slapd := read("slapd.conf")
	for _, want := range []string{"include /etc/zpr/evaluation/" + evaluationDirectorySchemaFile + "\n", `suffix "dc=example,dc=test"`, `rootdn "cn=admin,dc=example,dc=test"`, `dn.exact="cn=zpr-reader,dc=example,dc=test"`} {
		if !strings.Contains(slapd, want) {
			t.Errorf("slapd.conf lacks %q", want)
		}
	}
	if !strings.Contains(read(evaluationDirectorySchemaFile), "NAME 'zprMachine'") || !strings.Contains(read("compose.json"), evaluationDirectorySchemaFile) {
		t.Fatal("schema not written or not mounted")
	}
	if text := read("EVALUATION.txt"); !strings.Contains(text, "Not imported (entries carrying userPassword): cn=zpr-reader,ou=Service Accounts,dc=example,dc=test") {
		t.Fatalf("EVALUATION.txt: %s", text)
	}
	var registration evaluationRegistration
	if err := json.Unmarshal([]byte(read("source-registration.json")), &registration); err != nil {
		t.Fatal(err)
	}
	organizations, _ := json.Marshal(registration.Config["organizations"])
	if !strings.Contains(string(organizations), `"base_dn":"dc=example,dc=test","bind_dn":"cn=zpr-reader,dc=example,dc=test"`) {
		t.Fatalf("registration: %s", organizations)
	}
	if providers := read("subject-link-providers.json"); !strings.Contains(providers, `"base_dn": "dc=example,dc=test"`) {
		t.Fatalf("providers: %s", providers)
	}

	starter := filepath.Join(t.TempDir(), "starter")
	if _, err := prepareEvaluation(starter, testEvaluationConfig(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(starter, "slapd.conf")); !bytes.Contains(data, []byte(`suffix "dc=evaluation,dc=zpr"`)) || bytes.Contains(data, []byte(evaluationDirectorySchemaFile)) {
		t.Fatalf("starter slapd.conf changed: %s", data)
	}
	if _, err := os.Stat(filepath.Join(starter, evaluationDirectorySchemaFile)); err == nil {
		t.Fatal("starter install must not gain the schema file")
	}
}

func TestEvaluationOperatorDirectoryRejects(t *testing.T) {
	for name, content := range map[string]string{
		"outside base":       strings.Replace(testDirectoryLDIF, "uid=ada,ou=People,dc=example,dc=test", "uid=ada,dc=other,dc=test", 1),
		"base not first":     strings.Replace(testDirectoryLDIF, "dn: dc=example,dc=test\n", "dn: ou=Extra,dc=example,dc=test\n", 1),
		"duplicate":          testDirectoryLDIF + "\ndn: uid=ada,ou=People,dc=example,dc=test\nobjectClass: account\nuid: ada\n",
		"reserved reader":    testDirectoryLDIF + "\ndn: cn=zpr-reader,dc=example,dc=test\nobjectClass: organizationalRole\ncn: zpr-reader\n",
		"reserved subject":   testDirectoryLDIF + "\ndn: uid=evaluation-subject,ou=People,dc=example,dc=test\nobjectClass: account\nuid: evaluation-subject\n",
		"change record":      testDirectoryLDIF + "\ndn: uid=bob,ou=People,dc=example,dc=test\nchangetype: add\nuid: bob\n",
		"base64 dn":          testDirectoryLDIF + "\ndn:: dWlkPWJvYg==\nuid: bob\n",
		"below skipped":      testDirectoryLDIF + "\ndn: uid=x,cn=zpr-reader,ou=Service Accounts,dc=example,dc=test\nobjectClass: account\nuid: x\n",
		"empty":              "# nothing\n",
		"stray continuation": " orphan\n" + testDirectoryLDIF,
	} {
		directory := &evaluationDirectory{BaseDN: "dc=example,dc=test", LDIFFiles: []string{writeTestDirectoryLDIF(t, content)}}
		if _, _, err := renderEvaluationDirectoryLDIF(directory, "{SSHA}x"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, base := range []string{"", "dc=example,dc=test\nsuffix", `dc="x"`, "dc=a b", "dc=,dc=test", "dc=example, dc=test"} {
		if err := validateEvaluationBaseDN(base); err == nil {
			t.Errorf("base DN %q accepted", base)
		}
	}
	for name, directory := range map[string]*evaluationDirectory{
		"no files":      {BaseDN: "dc=example,dc=test"},
		"relative file": {BaseDN: "dc=example,dc=test", LDIFFiles: []string{"directory.ldif"}},
		"duplicate":     {BaseDN: "dc=example,dc=test", LDIFFiles: []string{"/a.ldif", "/a.ldif"}},
	} {
		if err := directory.validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-evaluation", "-state", filepath.Join(t.TempDir(), "x"), "-render-only", "-directory-ldif", "/a.ldif"}, &stdout, &stderr); code != 2 {
		t.Fatalf("-directory-ldif without base: code %d", code)
	}
	located := filepath.Join(t.TempDir(), "located")
	ldif := writeTestDirectoryLDIF(t, testDirectoryLDIF)
	if code := run([]string{"-evaluation", "-state", located, "-render-only", "-directory-base-dn", "dc=example,dc=test", "-directory-ldif", ldif}, &stdout, &stderr); code != 0 {
		t.Fatalf("render with directory: code %d: %s", code, stderr.String())
	}
	if code := run([]string{"-evaluation", "-state", located, "-render-only"}, &stdout, &stderr); code != 1 {
		t.Fatalf("restart without the frozen directory flags: code %d", code)
	}
}
