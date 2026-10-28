package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

// This opt-in test owns its Rust test-binary child, listener, keys and state.
// It never starts the production VS, Redis, Simulator, or a receiver substitute.
func TestPolicyApplyRustInterop(t *testing.T) {
	testPolicyApplyRustInterop(t, false)
}

func testPolicyApplyRustInterop(t *testing.T, recovery bool) {
	optIn := "ZPR_RUST_POLICY_INTEROP"
	if recovery {
		optIn = "ZPR_RUST_POLICY_RECOVERY_INTEROP"
	}
	if os.Getenv(optIn) != "1" {
		t.Skip("set " + optIn + "=1 to execute the isolated actual Rust router")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, ".policy-interop-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	write := func(name string, b []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var restartDatabase func() (int, int)
	if recovery {
		address, restart := ownedPolicyRecoveryValkey(t, dir)
		write("redis-address", []byte(address))
		restartDatabase = restart
	}
	marshal := func(v any) []byte {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	// Compilation is bounded, offline, and produces an exact test executable.
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer buildCancel()
	build := exec.CommandContext(buildCtx, "cargo", "test", "-p", "vs", "--bin", "vs", "--no-run", "--message-format=json")
	build.Dir = root
	build.Env = append(os.Environ(), "CARGO_NET_OFFLINE=true")
	var diagnostics bytes.Buffer
	build.Stderr = &diagnostics
	output, err := build.Output()
	if err != nil {
		t.Fatalf("offline Rust fixture compilation: %v\n%s", err, diagnostics.String())
	}
	var executable string
	for _, line := range bytes.Split(output, []byte("\n")) {
		var artifact struct {
			Reason     string                `json:"reason"`
			Executable string                `json:"executable"`
			Target     struct{ Name string } `json:"target"`
		}
		if json.Unmarshal(line, &artifact) == nil && artifact.Reason == "compiler-artifact" &&
			artifact.Target.Name == "vs" && artifact.Executable != "" {
			executable = artifact.Executable
		}
	}
	if executable == "" {
		t.Fatal("cargo did not identify the vs test executable")
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	fd, err := listener.File()
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	endpoint := "https://" + listener.Addr().String()
	now := time.Now()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	trust := operatordelegation.PolicyTrust{
		Audience: "https://policy.interop.invalid", VisaServiceAudience: endpoint,
		Organization: "isolated-interop", TargetID: "owned-rust-router",
		Keys: []operatordelegation.PolicyKey{{KeyID: "generated", Issuer: "https://control.interop.invalid",
			PublicKey: base64.StdEncoding.EncodeToString(public), ExpiresUnix: now.Add(time.Hour).Unix()}},
		Grants: []operatordelegation.PolicyGrant{{Issuer: "https://identity.interop.invalid", Subject: "named-reviewer",
			Organization: "isolated-interop", Actions: []string{"install"}}},
	}
	write("trust.json", marshal(map[string]any{"audience": endpoint, "policy_service_audience": trust.Audience,
		"organization": trust.Organization, "target_id": trust.TargetID, "keys": trust.Keys, "grants": trust.Grants}))
	roots := interopServerTLS(t, now, write)
	childCtx, childCancel := context.WithTimeout(context.Background(), 100*time.Second)
	t.Cleanup(childCancel)
	var child *exec.Cmd
	var log *os.File
	var done chan error
	startReceiver := func() {
		t.Helper()
		_ = os.Remove(filepath.Join(dir, "ready.json"))
		_ = os.Remove(filepath.Join(dir, "stop"))
		child = exec.CommandContext(childCtx, executable, "--exact", "admin_service::tests::policy_apply_rust_interop_fixture", "--ignored", "--nocapture")
		child.Dir = dir
		child.Env = append(os.Environ(), "ZPR_POLICY_INTEROP_DIR="+dir)
		child.ExtraFiles = []*os.File{fd}
		var err error
		log, err = os.OpenFile(filepath.Join(dir, "rust.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			t.Fatal(err)
		}
		child.Stdout, child.Stderr = log, log
		if err = child.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		done = make(chan error, 1)
		// Capture each generation: exactly one waiter reaps each exact process.
		running, finished := child, done
		go func() { finished <- running.Wait() }()
	}
	startReceiver()
	t.Cleanup(func() {
		if child == nil {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
		var waitErr error
		select {
		case waitErr = <-done:
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			waitErr = <-done
		}
		_ = log.Close()
		if waitErr != nil {
			b, _ := os.ReadFile(filepath.Join(dir, "rust.log"))
			t.Errorf("Rust child failed: %v\n%s", waitErr, b)
		} else {
			b, err := os.ReadFile(filepath.Join(dir, "events.json"))
			if err != nil || strings.TrimSpace(string(b)) == "0" {
				t.Errorf("Rust did not confirm drained application events: %q %v", b, err)
			}
			t.Logf("Rust child passed; drained policy events=%s", bytes.TrimSpace(b))
		}
	})
	var ready struct {
		WriteKey      string          `json:"write_key"`
		ReadKey       string          `json:"read_key"`
		Bundle        json.RawMessage `json:"bundle"`
		Container     []byte          `json:"container"`
		InitialDigest string          `json:"initial_digest"`
		InitialVinst  uint64          `json:"initial_vinst"`
		ReceiverPID   int             `json:"receiver_pid"`
		Persistent    bool            `json:"persistent_redis"`
	}
	waitReady := func() {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for {
			b, err := os.ReadFile(filepath.Join(dir, "ready.json"))
			if err == nil {
				if err = json.Unmarshal(b, &ready); err != nil {
					t.Fatal(err)
				}
				if ready.Persistent != recovery || ready.ReceiverPID != child.Process.Pid {
					t.Fatal("fixture backend or actual child PID differs")
				}
				return
			}
			if time.Now().After(until) {
				t.Fatal("Rust fixture did not become ready")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitReady()
	transport := &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
	}
	t.Cleanup(transport.CloseIdleConnections)
	client, err := newPolicyApplyClient(trust, endpoint, ready.WriteKey, transport, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	initial, err := client.current(ctx, time.Now())
	if err != nil {
		t.Fatalf("real HTTPS readiness/current: %v", err)
	}
	if initial.CurrentDigest != ready.InitialDigest || initial.CurrentVinst != ready.InitialVinst {
		t.Fatal("initial authoritative state differs from Rust assembly")
	}
	store, err := changereview.Open(filepath.Join(dir, "operations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if store != nil {
			_ = store.Close()
		}
	})
	signer, err := operatordelegation.NewPolicySigner(trust, "generated", trust.Keys[0].Issuer, private, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	actor := operatorauth.Identity{Issuer: trust.Grants[0].Issuer, Subject: trust.Grants[0].Subject}
	op := operatordelegation.PolicyOperation{OperationID: "real-apply", TargetID: trust.TargetID, SourceRevision: 3,
		CandidateDigest: policyContainerDigest(ready.Container), ExpectedCurrentDigest: initial.CurrentDigest, ExpectedCurrentVinst: initial.CurrentVinst}
	sign := func(op operatordelegation.PolicyOperation) ([]byte, operatordelegation.PolicyProof) {
		t.Helper()
		body, proof, err := signer.SignPair(op, ready.Bundle, actor, "install", "POST", policyApplyProducerRoute, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return body, proof
	}
	body, _ := sign(op)
	expectHTTP := func(name, key string, body []byte, status int) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, "POST", endpoint+operatordelegation.PolicyOperationRoute, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		if key != "" {
			r.Header.Set("X-API-Key", key)
		}
		res, err := client.http.Do(r)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		res.Body.Close()
		if res.StatusCode != status {
			t.Fatalf("%s: HTTP %d, want %d", name, res.StatusCode, status)
		}
		t.Logf("%s: HTTP %d", name, status)
	}
	expectHTTP("missing API key", "", body, 401)
	expectHTTP("insufficient API key", ready.ReadKey, body, 403)
	for _, tc := range []struct{ name, org, action string }{
		{"wrong organization", "other-org", "install"}, {"wrong action", trust.Organization, "rollback"},
	} {
		var command operatordelegation.PolicyCommand
		_ = json.Unmarshal(body, &command)
		payload, _ := base64.StdEncoding.DecodeString(command.Proof.Payload)
		var claims operatordelegation.PolicyClaims
		_ = json.Unmarshal(payload, &claims)
		claims.Organization, claims.Action = tc.org, tc.action
		payload = marshal(claims)
		command.Proof = operatordelegation.PolicyProof{Payload: base64.StdEncoding.EncodeToString(payload),
			Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))}
		expectHTTP(tc.name, ready.WriteKey, marshal(command), 403)
	}
	var invalid operatordelegation.PolicyCommand
	_ = json.Unmarshal(body, &invalid)
	invalid.Proof.Signature = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	expectHTTP("bad signed proof", ready.WriteKey, marshal(invalid), 403)
	unchanged, err := client.current(ctx, time.Now())
	if err != nil || unchanged != initial {
		t.Fatalf("refused requests changed receiver state: %+v %v", unchanged, err)
	}
	// Bind the actual Rust-produced container to a real approved review.
	digest := op.CandidateDigest
	proposal := changereview.Proposal{Organization: trust.Organization, Kind: "policy", Target: "real-policy",
		Revision: 3, BaseRevision: 2, Before: "previous", After: "reviewed fixture policy",
		Validation: "Validated", Impact: "Fixture only", Reason: "Actual Rust receiver integration",
		Author: changereview.Principal{Issuer: actor.Issuer, Subject: "separate-author"}}
	proposal.Candidate = &changereview.PolicyCandidate{StageID: digest, SourceSHA256: policyContainerDigest([]byte(proposal.After)),
		ConfigSHA256: strings.Repeat("b", 64), BundleSHA256: digest}
	review, err := store.Submit(ctx, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Decide(ctx, trust.Organization, review.ID, review.Digest, 1, "approve", "Reviewed actual container",
		changereview.Principal{Issuer: actor.Issuer, Subject: actor.Subject}); err != nil {
		t.Fatal(err)
	}
	stage := stagedPolicyCandidate{ID: digest, RecordID: proposal.Target, RecordRevision: proposal.Revision,
		SourceSHA256: proposal.Candidate.SourceSHA256, ConfigSHA256: proposal.Candidate.ConfigSHA256,
		BundleSHA256: digest, BundleSize: int64(len(ready.Container))}
	body, proof, err := prepareReviewedPolicyPair(ctx, store, signer, trust.Organization, review.ID, op.OperationID,
		proposal, stage, ready.Container, initial, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var prepared operatordelegation.PolicyCommand
	if err = json.Unmarshal(body, &prepared); err != nil {
		t.Fatal(err)
	}
	ready.Bundle = prepared.Bundle
	applied, err := client.apply(ctx, store, body, proof, time.Now())
	if err != nil || applied.Status != "succeeded" {
		t.Fatalf("actual apply: status=%s err=%v result=%s", applied.Status, err, applied.Result)
	}
	var result changereview.PolicyApplyResult
	if err = json.Unmarshal(applied.Result, &result); err != nil || result.Receipt == nil || result.Readback == nil {
		t.Fatalf("missing exact success evidence: %v", err)
	}
	if err = operatordelegation.ValidatePolicyReceipt(op, *result.Receipt); err != nil {
		t.Fatal(err)
	}
	readback, err := client.current(ctx, time.Now())
	if err != nil || readback != *result.Readback || readback.CurrentDigest != digest || readback.CurrentVinst != initial.CurrentVinst+1 {
		t.Fatalf("independent authoritative readback: %+v %v", readback, err)
	}
	t.Log("actual reviewed apply and exact independent digest/vinst readback: succeeded")
	expectHTTP("exact proof replay", ready.WriteKey, body, 409)
	freshBody, freshProof := sign(op)
	replayed, err := client.apply(ctx, store, freshBody, freshProof, time.Now())
	if err != nil || replayed.Status != "succeeded" {
		t.Fatalf("durable PS replay: %+v %v", replayed, err)
	}
	// Force a receiver round trip too, not merely the PS's durable-success shortcut.
	var receipt operatordelegation.PolicyReceipt
	if err = client.request(ctx, "POST", operatordelegation.PolicyOperationRoute, freshBody, &receipt); err != nil ||
		!reflect.DeepEqual(receipt, *result.Receipt) {
		t.Fatalf("fresh-proof receiver replay differs: %+v %v", receipt, err)
	}
	stale := op
	stale.OperationID = "stale-cas"
	staleBody, _ := sign(stale)
	expectHTTP("stale CAS", ready.WriteKey, staleBody, 409)
	afterCAS, err := client.current(ctx, time.Now())
	if err != nil || afterCAS != readback {
		t.Fatalf("stale CAS changed the authoritative receiver state: %+v %v", afterCAS, err)
	}
	t.Log("fresh-proof receiver replay is identical; stale CAS rejected without state change")
	suppressed := op
	suppressed.OperationID = "suppressed-application-reply"
	suppressed.ExpectedCurrentDigest, suppressed.ExpectedCurrentVinst = readback.CurrentDigest, readback.CurrentVinst
	suppressedBody, suppressedProof := sign(suppressed)
	write("suppress-next-reply", nil)
	uncertain, err := client.apply(ctx, store, suppressedBody, suppressedProof, time.Now())
	if err != nil || uncertain.Status != "uncertain" {
		t.Fatalf("suppressed reply must be uncertain: %+v %v", uncertain, err)
	}
	afterSuppression, err := client.current(ctx, time.Now())
	if err != nil || afterSuppression.CurrentDigest != digest || afterSuppression.CurrentVinst != readback.CurrentVinst+1 {
		t.Fatalf("suppressed reply did not follow a real apply: %+v %v", afterSuppression, err)
	}
	var retainedReceipt operatordelegation.PolicyReceipt
	if recovery {
		receiptBody, _ := sign(suppressed)
		if err = client.request(ctx, "POST", operatordelegation.PolicyOperationRoute, receiptBody, &retainedReceipt); err != nil {
			t.Fatalf("read committed receipt before restart: %v", err)
		}
		oldPID := child.Process.Pid
		if err = child.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		if err = <-done; err == nil {
			t.Fatal("expected killed receiver process, not graceful in-memory reconstruction")
		}
		child = nil
		_ = log.Close()
		transport.CloseIdleConnections()
		oldDBPID, newDBPID := restartDatabase()
		startReceiver()
		reviewedBundle := append(json.RawMessage(nil), ready.Bundle...)
		waitReady()
		// Startup advertises a newly compressed fixture bundle; retries must keep
		// the exact originally reviewed representation, not just equivalent bytes.
		ready.Bundle = reviewedBundle
		if ready.ReceiverPID == oldPID || oldDBPID == newDBPID {
			t.Fatal("receiver and database must both be new OS processes")
		}
		client, err = newPolicyApplyClient(trust, endpoint, ready.WriteKey, transport, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		restarted, err := client.current(ctx, time.Now())
		if err != nil || restarted != afterSuppression || ready.InitialDigest != restarted.CurrentDigest || ready.InitialVinst != restarted.CurrentVinst {
			t.Fatalf("actual Redis-backed startup did not restore exact digest/vinst: %+v %v", restarted, err)
		}
		t.Logf("actual receiver SIGKILL/reap/restart PID %d -> %d; Valkey AOF graceful stop/reap/restart PID %d -> %d; exact digest/vinst retained", oldPID, ready.ReceiverPID, oldDBPID, newDBPID)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = changereview.Open(filepath.Join(dir, "operations.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if recovery {
		persisted, err := store.PolicyOperation(ctx, trust.Organization, suppressed.OperationID)
		if err != nil || !reflect.DeepEqual(persisted, uncertain) {
			t.Fatalf("reopened PS store did not retain exact uncertain operation/binding: %+v %v", persisted, err)
		}
		t.Log("Policy-Service SQLite store closed/reopened independently of both process restarts")
	}
	reconcileBody, reconcileProof := sign(suppressed)
	reconciled, err := client.apply(ctx, store, reconcileBody, reconcileProof, time.Now())
	if err != nil || reconciled.Status != "succeeded" {
		t.Fatalf("fresh-proof reconciliation: %+v %v", reconciled, err)
	}
	final, err := client.current(ctx, time.Now())
	if err != nil || final != afterSuppression {
		t.Fatalf("reconciliation must not apply twice: %+v %v", final, err)
	}
	if recovery {
		if reconciled.ID != uncertain.ID || reconciled.Binding != uncertain.Binding || reconciled.Version <= uncertain.Version {
			t.Fatal("reconciliation replaced operation identity/CAS binding or did not advance durable result")
		}
		var recoveredResult changereview.PolicyApplyResult
		if err = json.Unmarshal(reconciled.Result, &recoveredResult); err != nil ||
			recoveredResult.Receipt == nil || !reflect.DeepEqual(*recoveredResult.Receipt, retainedReceipt) {
			t.Fatalf("same operation did not retain its exact durable receiver receipt: %+v %v", recoveredResult, err)
		}
		if err = operatordelegation.ValidatePolicyReceipt(suppressed, retainedReceipt); err != nil {
			t.Fatal(err)
		}
		stale.OperationID = "stale-cas-after-restart"
		staleBody, _ = sign(stale)
		expectHTTP("stale CAS after persistent restart", ready.WriteKey, staleBody, 409)
		unchanged, err := client.current(ctx, time.Now())
		if err != nil || unchanged != final {
			t.Fatalf("post-restart CAS rejection changed state: %+v %v", unchanged, err)
		}
		t.Log("same operation/CAS binding and identical receipt reconciled after actual receiver + database restarts; no second install; fresh stale CAS rejected")
	}
	t.Log("suppressed completed application reply: uncertain -> reopened PS store -> fresh-proof succeeded, no second apply (not packet loss)")
}

func interopServerTLS(t *testing.T, now time.Time, write func(string, []byte)) *x509.CertPool {
	t.Helper()
	caPublic, caKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "private interop CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, caPublic, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverPublic, serverKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "owned loopback"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		IPAddresses: []net.IP{net.IPv4(127, 0, 0, 1)}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, cert, ca, serverPublic, caKey)
	if err != nil {
		t.Fatal(err)
	}
	key, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	write("server.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	write("server.key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}))
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})) {
		t.Fatal("generated CA was not accepted")
	}
	return roots
}
