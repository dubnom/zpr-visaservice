//go:build linux || darwin

package enrollment

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This opt-in stdin-controlled fixture has no administrative HTTP endpoint.
// Only the browser test runner can ask its independent registry to decide.
func TestEnrollmentBrowserFixture(t *testing.T) {
	if os.Getenv("ZPR_ENROLLMENT_BROWSER_FIXTURE") != "1" {
		t.Skip("browser-runner fixture only")
	}
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	t.Setenv("SIMULATION_MANIFEST", "/does/not/exist")
	config, _ := serverConfig(t)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	config.Listen, config.Audience = listener.Addr().String(), "https://localhost:"+port
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	serving := make(chan error, 1)
	go func() { serving <- server.ServeTLS(listener, "", "") }()
	defer func() {
		server.Close()
		if err := <-serving; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Error(err)
		}
	}()
	reviewer, err := Open(config.DatabaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer reviewer.Close()
	invitation, code := createInvitation(t, store, time.Now().UTC())
	local := SetupConfig{Version: 1, Audience: config.Audience, CAFile: config.CertificateFile,
		StateDirectory: filepath.Join(t.TempDir(), "identity"), AllowSoftwareDevelopment: true}
	var setup *SetupServer
	var cancel context.CancelFunc
	var done chan error
	stop := func() {
		if setup == nil {
			return
		}
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		if err := setup.Close(); err != nil {
			t.Error(err)
		}
		setup = nil
	}
	defer stop()
	start := func() {
		t.Helper()
		setup, err = NewSetupServer(local)
		if err != nil {
			t.Fatal(err)
		}
		var ctx context.Context
		ctx, cancel = context.WithCancel(context.Background())
		done = make(chan error, 1)
		go func(current *SetupServer) { done <- current.Run(ctx) }(setup)
	}
	emit := func(value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stdout.Write(append(append([]byte("ZPR-FIXTURE "), data...), '\n')); err != nil {
			t.Fatal(err)
		}
	}
	start()
	emit(map[string]string{"url": setup.URL(), "invitation": invitation.ID, "code": code,
		"state_directory": local.StateDirectory, "audience": config.Audience})
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024)
	for scanner.Scan() {
		switch scanner.Text() {
		case "restart":
			stop()
			start()
			emit(map[string]string{"url": setup.URL()})
		case "approved", "rejected":
			item, err := reviewer.Get(context.Background(), "company", invitation.ID, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			decision, err := reviewer.Decide(context.Background(), "company", invitation.ID,
				"browser-fixture-reviewer", scanner.Text(), "Verified disposable browser test",
				item.KeyFingerprint, item.Revision, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			emit(map[string]any{"state": decision.State, "fingerprint": decision.KeyFingerprint})
		case "stop":
			return
		default:
			t.Fatal("invalid browser fixture command")
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}
