//go:build darwin && cgo

package enrollment

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestMacRuntimeIPCRequiresKernelAuditAndPinnedSignedPeer(t *testing.T) {
	requirement, err := currentRuntimeTestRequirement()
	if err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp("", "zpr-ipc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	path := filepath.Join(directory, "handoff.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	for _, connection := range []*net.UnixConn{client, server} {
		if err := verifyRuntimeConnection(connection, requirement, os.Geteuid()); err != nil {
			t.Fatalf("pinned local ad-hoc test peer rejected: %v", err)
		}
		for _, invalid := range []string{`identifier "com.zpr.unapproved"`, "not a signing requirement", "", requirement + "\x00"} {
			if err := verifyRuntimeConnection(connection, invalid, os.Geteuid()); err == nil {
				t.Fatal("unapproved or malformed code requirement accepted")
			}
		}
		if err := verifyRuntimeConnection(connection, requirement, os.Geteuid()+1); err == nil {
			t.Fatal("peer in a different user account accepted")
		}
	}
	if err := verifyRuntimeConnection(nil, requirement, os.Geteuid()); err == nil {
		t.Fatal("missing peer socket accepted")
	}
	// A different, genuinely signed executable must not pass our pinned identity.
	input, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer writer.Close()
	other := exec.Command("/usr/bin/nc", "-U", path)
	other.Stdin = input
	if err := other.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = other.Process.Kill()
		_ = other.Wait()
	}()
	if err := listener.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	unapproved, err := listener.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	defer unapproved.Close()
	if err := verifyRuntimeConnection(unapproved, requirement, os.Geteuid()); err == nil {
		t.Fatal("a different executable authenticated as the pinned setup app")
	}
}
