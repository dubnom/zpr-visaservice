package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMachineWorkloadNodeAddressDefaultsForLinuxOneNode(t *testing.T) {
	address, err := machineWorkloadNodeAddress(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if address != "10.0.0.1:5000" {
		t.Fatalf("node address = %q; want one-node default", address)
	}
}

func TestMachineWorkloadNodeAddressUsesMultinodeTarget(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "node-address"), []byte("172.30.0.13:5000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	address, err := machineWorkloadNodeAddress(runtimeDir)
	if err != nil {
		t.Fatal(err)
	}
	if address != "172.30.0.13:5000" {
		t.Fatalf("node address = %q; want configured multinode target", address)
	}
}

func TestMachineWorkloadNodeAddressRejectsInvalidConfig(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "node-address"), []byte("not-an-ip:5000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := machineWorkloadNodeAddress(runtimeDir); err == nil {
		t.Fatal("invalid machine workload node address was accepted")
	}
}
