package subjectsnapshot

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

type vector struct {
	Name         string            `json:"name"`
	Envelope     string            `json:"envelope"`
	Pins         map[string]string `json:"pins"`
	Organization string            `json:"organization"`
	Recipient    string            `json:"recipient"`
	Epoch        string            `json:"epoch"`
	Now          uint64            `json:"now"`
	Valid        bool              `json:"valid"`
	Prior        string            `json:"prior"`
	Decision     string            `json:"decision"`
	Digest       string            `json:"digest"`
}

func fixture() Payload {
	return Payload{
		Version: 1, Organization: "org", Recipient: "vs-1", Epoch: "epoch-1", Sequence: "1", IssuedAt: "1000", ExpiresAt: "2000",
		Authorities: []Authority{{"bas", "1", "bas_device", "auth-service", "https://issuer.example", true}},
		Sources:     []Source{{"directory", "1", "ldap-main", "ldap_entry_uuid", []string{"user.department"}, true}},
		Bindings: []Binding{{
			"link-1", "1", Root{"bas", "1", "device", "device:123"},
			Target{"directory", "1", "ldap_entry_uuid", "b6a21ab4-5554-4cde-ae65-41a406a3c07f"},
			[]string{"user.department"}, "1000", "1900",
		}},
		Tombstones: []Tombstone{},
	}
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return public, private
}

func rawEnvelope(t *testing.T, raw []byte, id string, key ed25519.PrivateKey, domain string) string {
	t.Helper()
	data, err := json.Marshal(envelope{1, id, base64.RawURLEncoding.EncodeToString(raw),
		base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, append([]byte(domain), raw...)))})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func marshal(t *testing.T, p Payload) []byte {
	t.Helper()
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func vectors(t *testing.T) []vector {
	t.Helper()
	public, private := newKey(t)
	secondPublic, secondPrivate := newKey(t)
	raw := marshal(t, fixture())
	base := rawEnvelope(t, raw, "first", private, Domain)
	basePins := map[string]string{"first": hex.EncodeToString(public)}
	var result []vector
	add := func(name, envelope string, valid bool) *vector {
		result = append(result, vector{Name: name, Envelope: envelope, Pins: basePins, Organization: "org", Recipient: "vs-1", Epoch: "epoch-1", Now: 1000, Valid: valid})
		return &result[len(result)-1]
	}
	add("valid", base, true)
	add("before-issue", base, false).Now = 999
	add("last-valid-second", base, true).Now = 1999
	add("expired-boundary", base, false).Now = 2000
	add("zero-clock", base, false).Now = 0
	add("wrong-organization", base, false).Organization = "other"
	add("wrong-recipient", base, false).Recipient = "vs-2"
	add("unknown-epoch", base, false).Epoch = "epoch-2"
	add("missing-trust", base, false).Pins = map[string]string{}
	add("wrong-key", base, false).Pins = map[string]string{"first": hex.EncodeToString(secondPublic)}
	add("invalid-pin", base, false).Pins = map[string]string{"first": "00"}
	add("unknown-key-id", strings.Replace(base, `"first"`, `"missing"`, 1), false)
	add("wrong-domain", rawEnvelope(t, raw, "first", private, Domain+"other"), false)
	tampered := strings.Replace(string(raw), `"vs-1"`, `"vs-2"`, 1)
	var decoded envelope
	if err := json.Unmarshal([]byte(base), &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Payload = base64.RawURLEncoding.EncodeToString([]byte(tampered))
	tamperedEnvelope, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	add("tampered-payload", string(tamperedEnvelope), false)
	rotated := rawEnvelope(t, raw, "second", secondPrivate, Domain)
	both := map[string]string{"first": hex.EncodeToString(public), "second": hex.EncodeToString(secondPublic)}
	add("overlap-old", base, true).Pins = both
	add("overlap-new", rotated, true).Pins = both
	add("removed-old", base, false).Pins = map[string]string{"second": hex.EncodeToString(secondPublic)}
	add("retained-new", rotated, true).Pins = map[string]string{"second": hex.EncodeToString(secondPublic)}
	for name, value := range map[string]string{
		"sequence-above-js-precision": "9007199254740993",
		"sequence-u64-maximum":        "18446744073709551615",
	} {
		p := fixture()
		p.Sequence = value
		add(name, rawEnvelope(t, marshal(t, p), "first", private, Domain), true)
	}
	for _, count := range []int{128, 129} {
		p := fixture()
		p.Recipient = strings.Repeat("v", count)
		add("identifier-bound-"+stringID(count), rawEnvelope(t, marshal(t, p), "first", private, Domain), count == 128).Recipient = p.Recipient
		p = fixture()
		p.Sources[0].EnrichmentKeys = []string{}
		for i := 0; i < count; i++ {
			p.Sources[0].EnrichmentKeys = append(p.Sources[0].EnrichmentKeys, "key"+stringID(i))
		}
		p.Bindings[0].EnrichmentKeys = []string{"key0"}
		add("key-count-bound-"+stringID(count), rawEnvelope(t, marshal(t, p), "first", private, Domain), count == 128)
	}
	for name, replacement := range map[string]string{
		"null-array":         strings.Replace(string(raw), `"tombstones":[]`, `"tombstones":null`, 1),
		"unknown-field":      strings.Replace(string(raw), `"version":1`, `"version":1,"trust_key":"injected"`, 1),
		"duplicate-field":    strings.Replace(string(raw), `"version":1`, `"version":1,"version":1`, 1),
		"escaped-duplicate":  strings.Replace(string(raw), `"version":1`, `"version":1,"\u0076ersion":1`, 1),
		"missing-false":      strings.Replace(string(raw), `,"enabled":true`, ``, 1),
		"float-version":      strings.Replace(string(raw), `"version":1`, `"version":1.0`, 1),
		"wrong-type":         strings.Replace(string(raw), `"sequence":"1"`, `"sequence":1`, 1),
		"trailing-value":     string(raw) + `{}`,
		"invalid-utf8":       string(raw) + string([]byte{255}),
		"bad-surrogate":      strings.Replace(string(raw), `"device:123"`, `"\ud800"`, 1),
		"nested-unknown":     strings.Replace(string(raw), `"subject_kind":"device"`, `"subject_kind":"device","extra":0`, 1),
		"nested-duplicate":   strings.Replace(string(raw), `"authority_version":"1"`, `"authority_version":"1","authority_version":"1"`, 1),
		"sequence-leading-0": strings.Replace(string(raw), `"sequence":"1"`, `"sequence":"01"`, 1),
		"sequence-overflow":  strings.Replace(string(raw), `"sequence":"1"`, `"sequence":"18446744073709551616"`, 1),
		"timestamp-overflow": strings.Replace(string(raw), `"issued_at":"1000"`, `"issued_at":"9223372036854775808"`, 1),
		"timestamp-fraction": strings.Replace(string(raw), `"issued_at":"1000"`, `"issued_at":"1000.0"`, 1),
	} {
		add(name, rawEnvelope(t, []byte(replacement), "first", private, Domain), false)
	}
	for name, edit := range map[string]func(*Payload){
		"foreign-authority":          func(p *Payload) { p.Bindings[0].Root.AuthorityID = "other" },
		"foreign-source":             func(p *Payload) { p.Bindings[0].Target.SourceID = "other" },
		"wrong-source-version":       func(p *Payload) { p.Bindings[0].Target.SourceVersion = "2" },
		"wrong-authority-version":    func(p *Payload) { p.Bindings[0].Root.AuthorityVersion = "2" },
		"disabled-authority":         func(p *Payload) { p.Authorities[0].Enabled = false },
		"disabled-source":            func(p *Payload) { p.Sources[0].Enabled = false },
		"foreign-key":                func(p *Payload) { p.Bindings[0].EnrichmentKeys = []string{"user.other"} },
		"duplicate-key":              func(p *Payload) { p.Bindings[0].EnrichmentKeys = []string{"user.department", "user.department"} },
		"unsupported-root":           func(p *Payload) { p.Bindings[0].Root.SubjectKind = "user" },
		"noncanonical-uuid":          func(p *Payload) { p.Bindings[0].Target.SubjectID = strings.ToUpper(p.Bindings[0].Target.SubjectID) },
		"zero-uuid":                  func(p *Payload) { p.Bindings[0].Target.SubjectID = "00000000-0000-0000-0000-000000000000" },
		"empty-validity":             func(p *Payload) { p.ExpiresAt = p.IssuedAt },
		"expired-binding":            func(p *Payload) { p.Bindings[0].ExpiresAt = "1000" },
		"reversed-binding":           func(p *Payload) { p.Bindings[0].NotBefore = "1950" },
		"duplicate-authority":        func(p *Payload) { p.Authorities = append(p.Authorities, p.Authorities[0]) },
		"duplicate-authority-tuple":  func(p *Payload) { a := p.Authorities[0]; a.ID = "other"; p.Authorities = append(p.Authorities, a) },
		"duplicate-source":           func(p *Payload) { p.Sources = append(p.Sources, p.Sources[0]) },
		"duplicate-binding":          func(p *Payload) { p.Bindings = append(p.Bindings, p.Bindings[0]) },
		"binding-tombstone-conflict": func(p *Payload) { p.Tombstones = []Tombstone{{"link-1", "2"}} },
		"duplicate-tombstone":        func(p *Payload) { p.Tombstones = []Tombstone{{"deleted", "1"}, {"deleted", "2"}} },
		"empty-key-array":            func(p *Payload) { p.Sources[0].EnrichmentKeys = []string{} },
	} {
		p := fixture()
		edit(&p)
		add(name, rawEnvelope(t, marshal(t, p), "first", private, Domain), false)
	}
	for name, replacement := range map[string]string{
		"envelope-duplicate": strings.Replace(base, `"version":1`, `"version":1,"version":1`, 1),
		"envelope-unknown":   strings.Replace(base, `"version":1`, `"version":1,"public_key":"injected"`, 1),
		"envelope-null":      strings.Replace(base, `"first"`, `null`, 1),
		"envelope-version":   strings.Replace(base, `"version":1`, `"version":2`, 1),
		"envelope-trailing":  base + `{}`,
	} {
		add(name, replacement, false)
	}
	if err := json.Unmarshal([]byte(base), &decoded); err != nil {
		t.Fatal(err)
	}
	decoded.Payload += "="
	padded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	add("padded-base64", string(padded), false)
	decoded.Payload = base64.RawURLEncoding.EncodeToString(raw)
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, decoded.Signature[len(decoded.Signature)-1])
	noncanonical := decoded.Signature[:len(decoded.Signature)-1] + string(alphabet[last+1])
	for name, signature := range map[string]string{
		"invalid-signature":     base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{255}, ed25519.SignatureSize)),
		"standard-base64":       base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{255}, ed25519.SignatureSize)),
		"short-signature":       base64.RawURLEncoding.EncodeToString([]byte{1}),
		"noncanonical-pad-bits": noncanonical,
	} {
		decoded.Signature = signature
		signatureEnvelope, err := json.Marshal(decoded)
		if err != nil {
			t.Fatal(err)
		}
		add(name, string(signatureEnvelope), false)
	}
	for _, count := range []int{MaxRecords, MaxRecords + 1} {
		p := fixture()
		for i := 0; i < count; i++ {
			p.Tombstones = append(p.Tombstones, Tombstone{ID: "deleted-" + stringID(i), Version: "1"})
		}
		add("record-bound-"+stringID(count), rawEnvelope(t, marshal(t, p), "first", private, Domain), count == MaxRecords)
	}
	for _, size := range []int{MaxPayload, MaxPayload + 1} {
		padded := append(bytes.Clone(raw), bytes.Repeat([]byte(" "), size-len(raw))...)
		add("payload-bound-"+stringID(size), rawEnvelope(t, padded, "first", private, Domain), size == MaxPayload)
	}
	for _, size := range []int{MaxEnvelope, MaxEnvelope + 1} {
		add("envelope-bound-"+stringID(size), base+strings.Repeat(" ", size-len(base)), size == MaxEnvelope)
	}
	for name, edit := range map[string]func(*Payload){
		"advance":                          func(p *Payload) {},
		"equivocation":                     func(p *Payload) { p.Sequence = "1"; p.ExpiresAt = "2100" },
		"record-content-equivocation":      func(p *Payload) { p.Bindings[0].Root.SubjectID = "changed" },
		"source-content-equivocation":      func(p *Payload) { p.Sources[0].ProviderID = "changed" },
		"authority-content-equivocation":   func(p *Payload) { p.Authorities[0].Issuer = "changed" },
		"remove-binding-without-tombstone": func(p *Payload) { p.Bindings = []Binding{} },
		"revoke":                           func(p *Payload) { p.Bindings = []Binding{}; p.Tombstones = []Tombstone{{"link-1", "2"}} },
		"missing-registration": func(p *Payload) {
			p.Bindings = []Binding{}
			p.Tombstones = []Tombstone{{"link-1", "2"}}
			p.Sources = []Source{}
		},
		"backdate":        func(p *Payload) { p.IssuedAt = "999" },
		"updated-binding": func(p *Payload) { p.Bindings[0].Version = "2"; p.Bindings[0].Root.SubjectID = "changed" },
	} {
		p := fixture()
		p.Sequence = "2"
		edit(&p)
		v := add("compare-"+name, rawEnvelope(t, marshal(t, p), "first", private, Domain), true)
		v.Prior, v.Decision = base, "reject"
		if name == "advance" || name == "revoke" || name == "updated-binding" {
			v.Decision = string(Advance)
		}
	}
	v := add("historical-replay", base, true)
	v.Prior, v.Decision, v.Now = base, string(Replay), 1999
	second := fixture()
	second.Sequence = "2"
	v = add("rollback", base, true)
	v.Prior, v.Decision = rawEnvelope(t, marshal(t, second), "first", private, Domain), "reject"
	newEpoch := fixture()
	newEpoch.Epoch = "epoch-2"
	v = add("compare-epoch-reset", rawEnvelope(t, marshal(t, newEpoch), "first", private, Domain), true)
	v.Epoch, v.Prior, v.Decision = "epoch-2", base, "reject-scope"
	for name, edits := range map[string]func(*Payload, *Payload){
		"source-version-rollback": func(a, b *Payload) {
			a.Sources[0].Version, a.Bindings[0].Target.SourceVersion = "2", "2"
		},
		"authority-version-rollback": func(a, b *Payload) {
			a.Authorities[0].Version, a.Bindings[0].Root.AuthorityVersion = "2", "2"
		},
		"binding-version-rollback": func(a, b *Payload) { a.Bindings[0].Version = "2" },
	} {
		a, b := fixture(), fixture()
		b.Sequence = "2"
		edits(&a, &b)
		v := add(name, rawEnvelope(t, marshal(t, b), "first", private, Domain), true)
		v.Prior, v.Decision = rawEnvelope(t, marshal(t, a), "first", private, Domain), "reject"
	}
	revoked := fixture()
	revoked.Sequence, revoked.Bindings, revoked.Tombstones = "2", []Binding{}, []Tombstone{{"link-1", "2"}}
	for name, edit := range map[string]func(*Payload){
		"forget-tombstone":        func(p *Payload) { p.Tombstones = []Tombstone{} },
		"resurrect-tombstone":     func(p *Payload) { p.Tombstones = []Tombstone{}; p.Bindings = fixture().Bindings },
		"lower-tombstone-version": func(p *Payload) { p.Tombstones[0].Version = "1" },
	} {
		p := fixture()
		p.Sequence, p.Bindings, p.Tombstones = "3", []Binding{}, []Tombstone{{"link-1", "2"}}
		edit(&p)
		v := add(name, rawEnvelope(t, marshal(t, p), "first", private, Domain), true)
		v.Prior, v.Decision = rawEnvelope(t, marshal(t, revoked), "first", private, Domain), "reject"
	}
	return result
}

func stringID(n int) string {
	return strconv.Itoa(n)
}

func checkVector(t *testing.T, v *vector) {
	t.Helper()
	pins := map[string]ed25519.PublicKey{}
	for id, value := range v.Pins {
		key, err := hex.DecodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		pins[id] = key
	}
	scope := Scope{v.Organization, v.Recipient, v.Epoch}
	verified, err := Verify([]byte(v.Envelope), pins, scope, v.Now)
	if (err == nil) != v.Valid {
		t.Fatalf("valid=%v, error=%v", v.Valid, err)
	}
	if err != nil {
		return
	}
	if v.Digest != "" && v.Digest != verified.Digest() {
		t.Fatalf("digest=%s, want=%s", verified.Digest(), v.Digest)
	}
	v.Digest = verified.Digest()
	if v.Prior != "" {
		if v.Decision == "reject-scope" {
			if _, err := Verify([]byte(v.Prior), pins, scope, v.Now); err == nil {
				t.Fatal("accepted previous epoch under new epoch pin")
			}
			priorScope := scope
			priorScope.Epoch = "epoch-1"
			prior, err := Verify([]byte(v.Prior), pins, priorScope, v.Now)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Compare(prior, verified); err == nil {
				t.Fatal("accepted epoch transition")
			}
			return
		}
		prior, err := Verify([]byte(v.Prior), pins, scope, v.Now)
		if err != nil {
			t.Fatal(err)
		}
		decision, err := Compare(prior, verified)
		if v.Decision == "reject" {
			if err == nil {
				t.Fatalf("accepted forbidden state transition: %s", decision)
			}
		} else if err != nil || string(decision) != v.Decision {
			t.Fatalf("decision=%s, error=%v, want=%s", decision, err, v.Decision)
		}
	}
}

func TestSnapshotContract(t *testing.T) {
	all := vectors(t)
	for i := range all {
		t.Run(all[i].Name, func(t *testing.T) { checkVector(t, &all[i]) })
	}
}

func TestSignerAndUnauthenticatedState(t *testing.T) {
	public, private := newKey(t)
	scope := Scope{"org", "vs-1", "epoch-1"}
	pins := map[string]ed25519.PublicKey{"signer": public}
	p := fixture()
	data, err := Sign(p, "signer", private)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := Verify(data, pins, scope, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Compare(Verified{}, verified); err == nil {
		t.Fatal("zero-value state implicitly initialized anti-rollback")
	}
	if _, err := Compare(verified, Verified{}); err == nil {
		t.Fatal("zero-value next state accepted")
	}
	if _, err := Sign(p, "invalid key id", private); err == nil {
		t.Fatal("invalid signer identifier accepted")
	}
	if _, err := Sign(p, "signer", private[:12]); err == nil {
		t.Fatal("short signing key accepted")
	}
	p.Tombstones = nil
	if _, err := Sign(p, "signer", private); err == nil {
		t.Fatal("null required array signed")
	}
	p = fixture()
	for i := 0; i <= MaxRecords; i++ {
		p.Tombstones = append(p.Tombstones, Tombstone{"deleted-" + stringID(i), "1"})
	}
	if _, err := Sign(p, "signer", private); err == nil {
		t.Fatal("oversized record collection signed")
	}
}

// Rust invokes this opt-in exporter into its own temporary directory. No
// production pins or private keys are read or written.
func TestInteropExport(t *testing.T) {
	path := os.Getenv("ZPR_SNAPSHOT_INTEROP_EXPORT")
	if path == "" {
		t.Skip("offline Rust interoperability exporter only")
	}
	all := vectors(t)
	for i := range all {
		checkVector(t, &all[i])
	}
	data, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestInteropImport(t *testing.T) {
	path := os.Getenv("ZPR_SNAPSHOT_INTEROP_IMPORT")
	if path == "" {
		t.Skip("offline Rust interoperability importer only")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v vector
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	checkVector(t, &v)
}
