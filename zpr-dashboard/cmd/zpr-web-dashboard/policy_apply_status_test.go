//go:build !production

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func policyStatusPeer(t *testing.T) (*tls.ConnectionState, string) {
	t.Helper()
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := createTestPolicyLeaf(ca, key, "read-only-status", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(leaf.Raw)
	return &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: [][]*x509.Certificate{{leaf, ca}}}, hex.EncodeToString(hash[:])
}

func TestPolicyStatusScopedReadOnlyLookup(t *testing.T) {
	for _, name := range []string{"pending", "failed", "uncertain", "uncertain-applied", "succeeded", "superseded", "same-digest-later-version", "digest-only", "lower-version", "outage", "wrong-readback-org", "concurrent-change"} {
		t.Run(name, func(t *testing.T) {
			f, resources, workspace, _, posts := policyApplyServiceFixture(t)
			body, proof := f.pair(t, f.current)
			approval, err := operatordelegation.VerifyPolicyPair(f.trust, "POST", policyApplyProducerRoute, body, proof, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			record, err := resources.store.ReservePolicyOperation(context.Background(), approval, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			op := approval.Command().Operation
			receipt := policyApplyReceipt(op)
			applied := operatordelegation.PolicyCurrentState{Organization: f.trust.Organization, TargetID: op.TargetID, CurrentDigest: op.CandidateDigest, CurrentVinst: *receipt.AppliedVinst}
			fresh := applied
			want := name
			switch name {
			case "pending", "digest-only":
				want = "pending"
			case "failed":
				message := "private receiver diagnostic"
				receipt.Status, receipt.AppliedDigest, receipt.AppliedVinst, receipt.Error = "failed", nil, nil, &message
				record, err = resources.store.FinishPolicyOperation(context.Background(), record, changereview.PolicyApplyResult{Status: "failed", Error: message, Receipt: &receipt})
			case "uncertain":
				record, err = resources.store.FinishPolicyOperation(context.Background(), record, changereview.PolicyApplyResult{Status: "uncertain", Error: "private diagnostic"})
			case "uncertain-applied":
				want = "uncertain"
				record, err = resources.store.FinishPolicyOperation(context.Background(), record, changereview.PolicyApplyResult{Status: "uncertain", Error: "response ambiguous", Receipt: &receipt, Readback: &applied})
			default:
				record, err = resources.store.FinishPolicyOperation(context.Background(), record, changereview.PolicyApplyResult{Status: "succeeded", Receipt: &receipt, Readback: &applied})
				switch name {
				case "superseded":
					fresh.CurrentDigest = strings.Repeat("a", 64)
					fresh.CurrentVinst++
					want = "superseded"
				case "same-digest-later-version":
					fresh.CurrentVinst++
					want = "superseded"
				case "lower-version":
					fresh.CurrentVinst--
					want = "uncertain"
				case "outage", "wrong-readback-org":
					want = "uncertain"
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			reads := 0
			resources.client.http = &http.Client{Transport: policyApplyTransport(func(r *http.Request) (*http.Response, error) {
				reads++
				if r.Method != "GET" || r.URL.Path != policyApplyCurrentRoute || r.Header.Get("X-API-Key") != "synthetic" {
					t.Fatal("lookup mutated/retried or bypassed VS auth")
				}
				if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 10*time.Second {
					t.Fatal("missing bounded context")
				}
				if name == "outage" {
					return nil, errors.New("secret unavailable")
				}
				if name == "concurrent-change" {
					if _, err := resources.store.FinishPolicyOperation(context.Background(), record, changereview.PolicyApplyResult{Status: "uncertain", Error: "another observer reconciled", Receipt: &receipt, Readback: &applied}); err != nil {
						t.Fatal(err)
					}
				}
				if name == "wrong-readback-org" {
					fresh.Organization = "other"
				}
				return policyApplyResponse(t, fresh), nil
			})}
			peer, pin := policyStatusPeer(t)
			resources.statusReaders = []policyStatusReader{{pin, f.trust.Organization, op.TargetID}}
			handler := policyServiceHandler(workspace, resources)
			path := policyStatusPrefix + f.trust.Organization + "/" + op.TargetID + "/" + op.OperationID
			request := httptest.NewRequest("GET", path, nil)
			request.TLS = peer
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, request)
			if name == "concurrent-change" {
				if w.Code != 503 {
					t.Fatalf("mixed durable snapshot %d %s", w.Code, w.Body)
				}
				return
			}
			if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" || w.Body.Len() > 16<<10 || reads != 1 || *posts != 0 {
				t.Fatalf("lookup %d %s reads=%d posts=%d", w.Code, w.Body, reads, *posts)
			}
			var response policyStatusResponse
			if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Status != want || response.DurableStatus != record.Status || response.Operation != op || response.Approver.Subject != "reviewer" || response.RecordVersion != record.Version {
				t.Fatalf("unexpected status: %+v want=%s", response, want)
			}
			if strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), `"bundle"`) || strings.Contains(w.Body.String(), `"binding"`) || strings.Contains(w.Body.String(), "secret") {
				t.Fatal("secret/unnecessary state exposed")
			}
			after, err := resources.store.PolicyOperation(context.Background(), f.trust.Organization, op.OperationID)
			if err != nil || !reflect.DeepEqual(after, record) {
				t.Fatal("lookup changed durable record", err)
			}
		})
	}
}

func TestPolicyStatusRejectsPinnedVerifiedCertificateWithoutClientAuth(t *testing.T) {
	f, resources, workspace, _, _ := policyApplyServiceFixture(t)
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	cert, _, err := createTestPolicyLeaf(ca, caKey, "server-only-status", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	chains, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil || len(chains) == 0 {
		t.Fatal("synthetic status certificate did not verify", err)
	}
	hash := sha256.Sum256(leaf.Raw)
	peer := &tls.ConnectionState{PeerCertificates: []*x509.Certificate{leaf}, VerifiedChains: chains}
	resources.statusReaders = []policyStatusReader{{
		CertificateSHA256: hex.EncodeToString(hash[:]),
		Organization:      f.trust.Organization,
		TargetID:          f.trust.TargetID,
	}}
	upstreamCalls := 0
	resources.client.http = &http.Client{Transport: policyApplyTransport(func(*http.Request) (*http.Response, error) {
		upstreamCalls++
		return nil, errors.New("unexpected upstream status lookup")
	})}
	handler := policyServiceHandler(workspace, resources)
	request := httptest.NewRequest(http.MethodGet, policyStatusPrefix+f.trust.Organization+"/"+f.trust.TargetID+"/unknown", nil)
	request.TLS = peer
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || upstreamCalls != 0 {
		t.Fatalf("status lookup returned %d after %d upstream calls; want 403 and no upstream call", response.Code, upstreamCalls)
	}
}

func TestPolicyStatusAuthorizationCanonicalURIAndGate(t *testing.T) {
	f, resources, workspace, _, posts := policyApplyServiceFixture(t)
	peer, pin := policyStatusPeer(t)
	resources.statusReaders = []policyStatusReader{{pin, f.trust.Organization, f.trust.TargetID}}
	handler := policyServiceHandler(workspace, resources)
	resources.client.http = &http.Client{Transport: policyApplyTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthorized/unknown lookup reached receiver")
		return nil, errors.New("not allowed")
	})}
	path := policyStatusPrefix + f.trust.Organization + "/" + f.trust.TargetID + "/unknown"
	for _, test := range []struct {
		name, method, path string
		peer               *tls.ConnectionState
		body               string
		code               int
	}{
		{"unknown", "GET", path, peer, "", 404},
		{"no TLS", "GET", path, nil, "", 403},
		{"wrong org", "GET", strings.Replace(path, f.trust.Organization, "other", 1), peer, "", 403},
		{"wrong target", "GET", strings.Replace(path, f.trust.TargetID, "other", 1), peer, "", 403},
		{"post", "POST", path, peer, "", 405},
		{"query", "GET", path + "?organization=other", peer, "", 400},
		{"escaped", "GET", strings.Replace(path, "unknown", "%75nknown", 1), peer, "", 400},
		{"body", "GET", path, peer, "{}", 400},
		{"missing ID", "GET", policyStatusPrefix + f.trust.Organization + "/" + f.trust.TargetID + "/", peer, "", 400},
		{"double slash", "GET", policyStatusPrefix + f.trust.Organization + "//" + f.trust.TargetID + "/unknown", peer, "", 400},
		{"dot segment", "GET", policyStatusPrefix + f.trust.Organization + "/" + f.trust.TargetID + "/../unknown", peer, "", 400},
		{"base route", "GET", strings.TrimSuffix(policyStatusPrefix, "/"), peer, "", 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			r.TLS = test.peer
			r.Header.Set("X-Actor", "read-only-status")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != test.code || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
		})
	}
	other, _ := policyStatusPeer(t)
	r := httptest.NewRequest("GET", path, nil)
	r.TLS = other
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("unlisted verified mTLS identity authorized")
	}
	savedChains := peer.VerifiedChains
	peer.VerifiedChains = nil
	r = httptest.NewRequest("GET", path, nil)
	r.TLS = peer
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("unverified certificate pin authorized")
	}
	peer.VerifiedChains = savedChains
	notAfter := peer.PeerCertificates[0].NotAfter
	peer.PeerCertificates[0].NotAfter = time.Now().Add(-time.Second)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("expired read identity authorized")
	}
	peer.PeerCertificates[0].NotAfter = notAfter
	resources.statusReaders = nil
	handler = policyServiceHandler(workspace, resources)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 404 || *posts != 0 {
		t.Fatal("unconfigured status route enabled or mutated")
	}
}

func TestPolicyCurrentReadbackIsScopedFreshAndReadOnly(t *testing.T) {
	f, resources, workspace, _, posts := policyApplyServiceFixture(t)
	peer, pin := policyStatusPeer(t)
	resources.statusReaders = []policyStatusReader{{pin, f.trust.Organization, f.trust.TargetID}}
	current := operatordelegation.PolicyCurrentState{
		TargetID: f.trust.TargetID, Organization: f.trust.Organization,
		CurrentDigest: strings.Repeat("a", 64), CurrentVinst: 4,
	}
	reads := 0
	resources.client.http = &http.Client{Transport: policyApplyTransport(func(r *http.Request) (*http.Response, error) {
		reads++
		if r.Method != http.MethodGet || r.URL.Path != policyApplyCurrentRoute ||
			r.Header.Get("X-API-Key") != "synthetic" {
			t.Fatal("current policy lookup mutated or bypassed Visa Service authentication")
		}
		if deadline, ok := r.Context().Deadline(); !ok || time.Until(deadline) > 10*time.Second {
			t.Fatal("current policy lookup is missing a bounded context")
		}
		return policyApplyResponse(t, current), nil
	})}
	handler := policyServiceHandler(workspace, resources)
	path := policyCurrentPrefix + f.trust.Organization + "/" + f.trust.TargetID
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.TLS = peer
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
		reads != 1 || *posts != 0 {
		t.Fatalf("current policy lookup returned %d %s; reads=%d posts=%d", response.Code, response.Body, reads, *posts)
	}
	var got operatordelegation.PolicyCurrentState
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil || !reflect.DeepEqual(got, current) {
		t.Fatalf("current policy response = %+v, decode error = %v", got, err)
	}

	for _, test := range []struct {
		name, method, path string
		peer               *tls.ConnectionState
		body               string
		want               int
	}{
		{"no TLS", http.MethodGet, path, nil, "", http.StatusForbidden},
		{"wrong organization", http.MethodGet, strings.Replace(path, f.trust.Organization, "other", 1), peer, "", http.StatusForbidden},
		{"wrong target", http.MethodGet, strings.Replace(path, f.trust.TargetID, "other", 1), peer, "", http.StatusForbidden},
		{"post", http.MethodPost, path, peer, "", http.StatusMethodNotAllowed},
		{"query", http.MethodGet, path + "?target=other", peer, "", http.StatusBadRequest},
		{"escaped", http.MethodGet, strings.Replace(path, f.trust.Organization, "%6c"+f.trust.Organization[1:], 1), peer, "", http.StatusBadRequest},
		{"body", http.MethodGet, path, peer, "{}", http.StatusBadRequest},
		{"missing target", http.MethodGet, policyCurrentPrefix + f.trust.Organization + "/", peer, "", http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.TLS = test.peer
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("current policy request = %d %s", response.Code, response.Body)
			}
		})
	}
	if reads != 1 || *posts != 0 {
		t.Fatalf("invalid current policy requests reached Visa Service: reads=%d posts=%d", reads, *posts)
	}
}

func TestPolicyStatusProtectedReaderConfig(t *testing.T) {
	for _, name := range []string{"valid", "wrong-org", "wrong-target", "malformed-pin", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			path, config, now := policyApplyConfigFixture(t)
			trustBytes, err := readOperatorFile(config.TrustFile, 65536, true)
			if err != nil {
				t.Fatal(err)
			}

			var trust operatordelegation.PolicyTrust
			if err = json.Unmarshal(trustBytes, &trust); err != nil {
				t.Fatal(err)
			}
			reader := policyStatusReader{strings.Repeat("a", 64), trust.Organization, trust.TargetID}
			switch name {
			case "wrong-org":
				reader.Organization = "other"
			case "wrong-target":
				reader.TargetID = "other"
			case "malformed-pin":
				reader.CertificateSHA256 = "bad"
			}
			config.StatusReaders = []policyStatusReader{reader}
			if name == "duplicate" {
				config.StatusReaders = append(config.StatusReaders, reader)
			}
			writePolicyApplyJSON(t, path, config)
			resources, err := loadPolicyApplyResources(path, now)
			if name == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if len(resources.statusReaders) != 1 {
					t.Fatal("reader not loaded")
				}
				if err = resources.Close(); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				resources.Close()
				t.Fatal("invalid reader config accepted")
			}
		})
	}
}

func TestPolicyStatusCorruptEvidenceFailsClosed(t *testing.T) {
	for _, name := range []string{"missing-receipt", "another-operation", "wrong-applied-version", "wrong-fingerprint", "invalid-durable-status", "malformed-result"} {
		t.Run(name, func(t *testing.T) {
			f, resources, workspace, _, _ := policyApplyServiceFixture(t)
			path := filepath.Join(t.TempDir(), "private", "lookup.sqlite")
			store, err := changereview.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.Close(); err != nil {
					t.Error(err)
				}
			})
			resources.store = store
			body, proof := f.pair(t, f.current)
			approval, err := operatordelegation.VerifyPolicyPair(f.trust, "POST", policyApplyProducerRoute, body, proof, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			record, err := store.ReservePolicyOperation(context.Background(), approval, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			op := approval.Command().Operation
			receipt := policyApplyReceipt(op)
			applied := operatordelegation.PolicyCurrentState{Organization: f.trust.Organization, TargetID: op.TargetID, CurrentDigest: op.CandidateDigest, CurrentVinst: *receipt.AppliedVinst}
			evidence := changereview.PolicyApplyResult{Status: "succeeded", Receipt: &receipt, Readback: &applied}
			switch name {
			case "missing-receipt":
				evidence.Receipt = nil
			case "another-operation":
				receipt.OperationID = "another"
			case "wrong-applied-version":
				v := *receipt.AppliedVinst + 1
				receipt.AppliedVinst = &v
			case "wrong-fingerprint":
				receipt.RequestFingerprint = strings.Repeat("0", 64)
			}
			encoded, err := json.Marshal(evidence)
			if err != nil {
				t.Fatal(err)
			}
			status := "succeeded"
			if name == "invalid-durable-status" {
				status = "invented"
			}
			if name == "malformed-result" {
				encoded = []byte(`{`)
			}
			raw, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = raw.Exec("UPDATE policy_operations SET status=?,result=? WHERE id=?", status, string(encoded), record.ID)
			if closeErr := raw.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if err != nil {
				t.Fatal(err)
			}
			peer, pin := policyStatusPeer(t)
			resources.statusReaders = []policyStatusReader{{pin, f.trust.Organization, f.trust.TargetID}}
			r := httptest.NewRequest("GET", policyStatusPrefix+f.trust.Organization+"/"+op.TargetID+"/"+op.OperationID, nil)
			r.TLS = peer
			w := httptest.NewRecorder()
			policyServiceHandler(workspace, resources).ServeHTTP(w, r)
			if name == "invalid-durable-status" || name == "malformed-result" {
				if w.Code != 503 {
					t.Fatalf("corrupt storage %d %s", w.Code, w.Body)
				}
				return
			}
			var response policyStatusResponse
			if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || response.Status != "uncertain" || response.ReadbackError != "invalid_evidence" || response.Receipt != nil {
				t.Fatalf("corrupt success exposed: %d %s", w.Code, w.Body)
			}
		})
	}
}
