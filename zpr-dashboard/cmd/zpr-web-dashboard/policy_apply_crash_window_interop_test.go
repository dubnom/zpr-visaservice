package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestPolicyApplyRustCrashWindowRecoveryInterop(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	executable := buildPolicyCrashWindowFixture(t, root)
	for _, mode := range []string{"pending-before-commit", "committed-pending-finalization"} {
		t.Run(mode, func(t *testing.T) {
			testPolicyApplyRustCrashWindow(t, executable, mode)
		})
	}
}

func testPolicyApplyRustCrashWindow(t *testing.T, executable, mode string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "zpr-policy-crash-window-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove owned crash-window fixture directory: %v", err)
		}
	})
	write := func(name string, data []byte) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	address, _ := ownedPolicyRecoveryValkey(t, dir)
	write("redis-address", []byte(address))

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
		Audience: "https://policy.crash-window.invalid", VisaServiceAudience: endpoint,
		Organization: "isolated-crash-window", TargetID: "owned-rust-router",
		Keys: []operatordelegation.PolicyKey{{
			KeyID: "generated", Issuer: "https://control.crash-window.invalid",
			PublicKey: base64.StdEncoding.EncodeToString(public), ExpiresUnix: now.Add(time.Hour).Unix(),
		}},
		Grants: []operatordelegation.PolicyGrant{{
			Issuer: "https://identity.crash-window.invalid", Subject: "named-reviewer",
			Organization: "isolated-crash-window", Actions: []string{"install"},
		}},
	}
	trustJSON, err := json.Marshal(map[string]any{
		"audience": endpoint, "policy_service_audience": trust.Audience,
		"organization": trust.Organization, "target_id": trust.TargetID,
		"keys": trust.Keys, "grants": trust.Grants,
	})
	if err != nil {
		t.Fatal(err)
	}
	write("trust.json", trustJSON)
	roots := interopServerTLS(t, now, write)

	transport := &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12},
		DialContext:         (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
	}
	defer transport.CloseIdleConnections()
	childCtx, childCancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer childCancel()
	var child *exec.Cmd
	var done chan error
	var log *os.File
	startReceiver := func(seedMode string) {
		t.Helper()
		_ = os.Remove(filepath.Join(dir, "ready.json"))
		_ = os.Remove(filepath.Join(dir, "ready.pending"))
		_ = os.Remove(filepath.Join(dir, "stop"))
		child = exec.CommandContext(childCtx, executable, "--exact",
			"admin_service::tests::policy_apply_rust_crash_window_fixture", "--ignored", "--nocapture")
		child.Dir = dir
		child.Env = crashWindowFixtureEnv(os.Environ(), dir, seedMode)
		child.ExtraFiles = []*os.File{fd}
		var openErr error
		log, openErr = os.OpenFile(filepath.Join(dir, "rust.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if openErr != nil {
			t.Fatal(openErr)
		}
		child.Stdout, child.Stderr = log, log
		if err := child.Start(); err != nil {
			_ = log.Close()
			t.Fatal(err)
		}
		done = make(chan error, 1)
		running, finished := child, done
		go func() { finished <- running.Wait() }()
	}
	waitReceiver := func() crashWindowReady {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for {
			select {
			case err := <-done:
				b, _ := os.ReadFile(filepath.Join(dir, "rust.log"))
				t.Fatalf("Rust fixture exited before readiness: %v\n%s", err, b)
			default:
			}
			data, readErr := os.ReadFile(filepath.Join(dir, "ready.json"))
			if readErr == nil {
				var ready crashWindowReady
				if err := json.Unmarshal(data, &ready); err != nil {
					t.Fatal(err)
				}
				if !ready.Persistent || ready.ReceiverPID != child.Process.Pid {
					t.Fatal("receiver readiness did not identify the owned persistent fixture")
				}
				return ready
			}
			if time.Now().After(until) {
				t.Fatal("Rust fixture did not become ready")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	stopReceiver := func(crash bool) {
		t.Helper()
		if child == nil {
			return
		}
		if crash {
			if err := child.Process.Kill(); err != nil {
				t.Errorf("kill exact owned receiver PID %d: %v", child.Process.Pid, err)
			}
		} else {
			_ = os.WriteFile(filepath.Join(dir, "stop"), nil, 0600)
		}
		select {
		case err := <-done:
			if crash && err == nil {
				t.Error("receiver crash window unexpectedly exited cleanly")
			}
			if !crash && err != nil {
				b, _ := os.ReadFile(filepath.Join(dir, "rust.log"))
				t.Errorf("owned Rust receiver failed while stopping: %v\n%s", err, b)
			}
		case <-time.After(5 * time.Second):
			_ = child.Process.Kill()
			<-done
			t.Error("owned Rust receiver exceeded its stop deadline")
		}
		_ = log.Close()
		child = nil
	}
	t.Cleanup(func() { stopReceiver(false) })

	startReceiver(mode)
	seededReceiver := waitReceiver()
	var seed struct {
		Mode                  string  `json:"mode"`
		OperationID           string  `json:"operation_id"`
		TargetID              string  `json:"target_id"`
		SourceRevision        uint64  `json:"source_revision"`
		CandidateDigest       string  `json:"candidate_digest"`
		ExpectedCurrentDigest string  `json:"expected_current_digest"`
		ExpectedCurrentVinst  uint64  `json:"expected_current_vinst"`
		RequestFingerprint    string  `json:"request_fingerprint"`
		SeedStatus            string  `json:"seed_status"`
		SeedAppliedDigest     *string `json:"seed_applied_digest"`
		SeedAppliedVinst      *uint64 `json:"seed_applied_vinst"`
		InitialDigest         string  `json:"initial_digest"`
		InitialVinst          uint64  `json:"initial_vinst"`
	}
	seedData, err := os.ReadFile(filepath.Join(dir, "crash-window-seed.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(seedData, &seed); err != nil {
		t.Fatal(err)
	}
	if seed.Mode != mode || seed.SeedStatus != "pending" || seed.OperationID == "" ||
		seed.TargetID != trust.TargetID || seed.CandidateDigest == "" ||
		seed.RequestFingerprint == "" {
		t.Fatalf("invalid persisted seed evidence: %+v", seed)
	}
	seedClient, err := newPolicyApplyClient(trust, endpoint, seededReceiver.WriteKey, transport, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	preCrash, err := seedClient.current(childCtx, time.Now())
	if err != nil {
		t.Fatalf("read seeded current state: %v", err)
	}
	wantSeedCurrent := operatordelegation.PolicyCurrentState{
		TargetID: trust.TargetID, Organization: trust.Organization,
		CurrentDigest: seed.InitialDigest, CurrentVinst: seed.InitialVinst,
	}
	if mode == "committed-pending-finalization" {
		wantSeedCurrent.CurrentDigest = seed.CandidateDigest
		wantSeedCurrent.CurrentVinst++
		if seed.SeedAppliedDigest == nil || *seed.SeedAppliedDigest != seed.CandidateDigest ||
			seed.SeedAppliedVinst == nil || *seed.SeedAppliedVinst != wantSeedCurrent.CurrentVinst {
			t.Fatalf("committed crash window lacks exact pending applied markers: %+v", seed)
		}
	} else if seed.SeedAppliedDigest != nil || seed.SeedAppliedVinst != nil {
		t.Fatalf("pre-commit crash window unexpectedly has applied markers: %+v", seed)
	}
	if preCrash != wantSeedCurrent || seededReceiver.InitialDigest != preCrash.CurrentDigest ||
		seededReceiver.InitialVinst != preCrash.CurrentVinst {
		t.Fatalf("seeded receiver state mismatch: ready=%+v readback=%+v want=%+v", seededReceiver, preCrash, wantSeedCurrent)
	}

	oldPID := seededReceiver.ReceiverPID
	stopReceiver(true)
	startReceiver("")
	recoveredReceiver := waitReceiver()
	if recoveredReceiver.ReceiverPID == oldPID {
		t.Fatal("receiver did not restart as a new process")
	}
	client, err := newPolicyApplyClient(trust, endpoint, recoveredReceiver.WriteKey, transport, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	beforeReplay, err := client.current(childCtx, time.Now())
	if err != nil || beforeReplay != wantSeedCurrent {
		t.Fatalf("actual RedisDb restart changed the seeded state: got %+v want %+v err=%v", beforeReplay, wantSeedCurrent, err)
	}
	op := operatordelegation.PolicyOperation{
		OperationID: seed.OperationID, TargetID: seed.TargetID, SourceRevision: seed.SourceRevision,
		CandidateDigest: seed.CandidateDigest, ExpectedCurrentDigest: seed.ExpectedCurrentDigest,
		ExpectedCurrentVinst: seed.ExpectedCurrentVinst,
	}
	if op.ExpectedCurrentDigest != seed.InitialDigest || op.ExpectedCurrentVinst != seed.InitialVinst ||
		op.CandidateDigest != policyContainerDigest(recoveredReceiver.Container) {
		t.Fatalf("recovery operation differs from seeded request: %+v seed=%+v", op, seed)
	}
	signer, err := operatordelegation.NewPolicySigner(trust, "generated", trust.Keys[0].Issuer, private, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	actor := operatorauth.Identity{Issuer: trust.Grants[0].Issuer, Subject: trust.Grants[0].Subject}
	sign := func() []byte {
		t.Helper()
		body, _, err := signer.SignPair(op, recoveredReceiver.Bundle, actor, "install", http.MethodPost,
			policyApplyProducerRoute, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	postOperation := func(body []byte, wantStatus int, out any) error {
		t.Helper()
		request, err := http.NewRequestWithContext(childCtx, http.MethodPost,
			endpoint+operatordelegation.PolicyOperationRoute, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-API-Key", recoveredReceiver.WriteKey)
		response, err := client.http.Do(request)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != wantStatus {
			return fmt.Errorf("HTTP %d, want %d", response.StatusCode, wantStatus)
		}
		if out != nil {
			return decodeOperatorConfigResponse(response, out)
		}
		return nil
	}
	var receipt operatordelegation.PolicyReceipt
	if err = postOperation(sign(), http.StatusOK, &receipt); err != nil {
		t.Fatalf("recover operation after actual receiver restart: %v", err)
	}
	if err = operatordelegation.ValidatePolicyReceipt(op, receipt); err != nil {
		t.Fatalf("recovered receipt binding: %v", err)
	}
	if receipt.Status != "succeeded" || receipt.RequestFingerprint != seed.RequestFingerprint ||
		receipt.AppliedDigest == nil || *receipt.AppliedDigest != seed.CandidateDigest ||
		receipt.AppliedVinst == nil || *receipt.AppliedVinst != seed.InitialVinst+1 {
		t.Fatalf("recovery did not return the seeded operation's exact-once success: %+v", receipt)
	}
	wantFinal := operatordelegation.PolicyCurrentState{
		TargetID: trust.TargetID, Organization: trust.Organization,
		CurrentDigest: seed.CandidateDigest, CurrentVinst: seed.InitialVinst + 1,
	}
	afterRecovery, err := client.current(childCtx, time.Now())
	if err != nil || afterRecovery != wantFinal {
		t.Fatalf("independent readback after recovery: got %+v want %+v err=%v", afterRecovery, wantFinal, err)
	}
	var replay operatordelegation.PolicyReceipt
	if err = postOperation(sign(), http.StatusOK, &replay); err != nil ||
		!reflect.DeepEqual(replay, receipt) {
		t.Fatalf("same operation replay changed its receipt: %+v err=%v", replay, err)
	}
	afterReplay, err := client.current(childCtx, time.Now())
	if err != nil || afterReplay != wantFinal {
		t.Fatalf("same-operation replay applied twice: got %+v want %+v err=%v", afterReplay, wantFinal, err)
	}
	stale := op
	stale.OperationID = "crash-window-stale-cas"
	staleBody, _, err := signer.SignPair(stale, recoveredReceiver.Bundle, actor, "install", http.MethodPost,
		policyApplyProducerRoute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = postOperation(staleBody, http.StatusConflict, nil); err != nil {
		t.Fatalf("stale CAS after recovery: %v", err)
	}
	afterStale, err := client.current(childCtx, time.Now())
	if err != nil || afterStale != wantFinal {
		t.Fatalf("stale CAS changed recovered state: got %+v want %+v err=%v", afterStale, wantFinal, err)
	}
	t.Logf("%s: seeded pending state survived receiver PID %d -> %d; one applied digest/vinst, stable receipt, stale CAS rejected",
		mode, oldPID, recoveredReceiver.ReceiverPID)
	stopReceiver(false)
}

type crashWindowReady struct {
	WriteKey      string          `json:"write_key"`
	Bundle        json.RawMessage `json:"bundle"`
	Container     []byte          `json:"container"`
	InitialDigest string          `json:"initial_digest"`
	InitialVinst  uint64          `json:"initial_vinst"`
	ReceiverPID   int             `json:"receiver_pid"`
	Persistent    bool            `json:"persistent_redis"`
}

func buildPolicyCrashWindowFixture(t *testing.T, root string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "cargo", "test", "-p", "vs", "--bin", "vs", "--no-run", "--message-format=json")
	build.Dir = root
	build.Env = append(os.Environ(), "CARGO_NET_OFFLINE=true")
	var diagnostics bytes.Buffer
	build.Stderr = &diagnostics
	output, err := build.Output()
	if err != nil {
		t.Fatalf("offline Rust fixture compilation: %v\n%s", err, diagnostics.String())
	}
	for _, line := range bytes.Split(output, []byte("\n")) {
		var artifact struct {
			Reason     string                `json:"reason"`
			Executable string                `json:"executable"`
			Target     struct{ Name string } `json:"target"`
		}
		if json.Unmarshal(line, &artifact) == nil && artifact.Reason == "compiler-artifact" &&
			artifact.Target.Name == "vs" && artifact.Executable != "" {
			return artifact.Executable
		}
	}
	t.Fatal("cargo did not identify the vs test executable")
	return ""
}

func crashWindowFixtureEnv(base []string, dir, seedMode string) []string {
	env := make([]string, 0, len(base)+3)
	for _, value := range base {
		if strings.HasPrefix(value, "ZPR_POLICY_INTEROP_DIR=") ||
			strings.HasPrefix(value, "ZPR_POLICY_CRASH_WINDOW_SEED=") {
			continue
		}
		env = append(env, value)
	}
	env = append(env, "ZPR_POLICY_INTEROP_DIR="+dir)
	if seedMode != "" {
		env = append(env, "ZPR_POLICY_CRASH_WINDOW_SEED="+seedMode)
	}
	return env
}

func decodeOperatorConfigResponse(response *http.Response, out any) error {
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(data) > 1<<20 {
		return fmt.Errorf("receiver response exceeds limit")
	}
	return json.Unmarshal(data, out)
}
