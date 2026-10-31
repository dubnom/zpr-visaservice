//go:build !production

package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMachineEnrollmentAuthenticationNeverFallsBackToBootstrap(t *testing.T) {
	root := t.TempDir()
	runtime := filepath.Join(root, "runtime")
	assets := filepath.Join(root, "assets")
	authentication := filepath.Join(assets, "authentication")
	for _, directory := range []string{runtime, assets, authentication} {
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
	}
	key := filepath.Join(authentication, "finance-client.key")
	ca := filepath.Join(authentication, "ca.pem")
	config := filepath.Join(authentication, "finance-client.toml")
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)})
	if err := os.WriteFile(key, keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	certificate, _, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	data := []byte(fmt.Sprintf("[global]\n[adapter]\n[authentication]\nbas_key = %q\ntls_ca_file = %q\nbas_client_id = \"velocity-machine-03-finance-client\"\n", key, ca))
	if err := os.WriteFile(config, data, 0600); err != nil {
		t.Fatal(err)
	}
	mode := filepath.Join(runtime, machineAuthenticationModeFile)
	if _, err := machineWorkloadAuthenticationArguments(runtime, assets, "finance-client", "bootstrap.key"); err == nil {
		t.Fatal("missing authentication mode fell back")
	}
	if err := os.WriteFile(mode, []byte("enrollment-bas\n"), 0600); err != nil {
		t.Fatal(err)
	}
	want := []string{"--config-file", config}
	got, err := machineWorkloadAuthenticationArguments(runtime, assets, "finance-client", "bootstrap.key")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("BAS authentication: %v %v", got, err)
	}
	if err := os.Remove(config); err != nil {
		t.Fatal(err)
	}
	if _, err := machineWorkloadAuthenticationArguments(runtime, assets, "finance-client", "bootstrap.key"); err == nil {
		t.Fatal("missing BAS config fell back")
	}
	if err := os.WriteFile(config, append(data, []byte("\n[global]\nbootstrap_key = \"forbidden.key\"\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := machinePHAuthenticationArguments("", config); err == nil {
		t.Fatal("mixed authentication accepted")
	}
	if _, err := machinePHAuthenticationArguments("bootstrap.key", config); err == nil {
		t.Fatal("dual controller credentials accepted")
	}
	if _, err := machinePHAuthenticationArguments("", ""); err == nil {
		t.Fatal("missing controller credentials accepted")
	}
	if err := os.WriteFile(config, data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(config, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := machinePHAuthenticationArguments("", config); err == nil {
		t.Fatal("public authentication config accepted")
	}
	if err := os.WriteFile(mode, []byte("unknown\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := machineWorkloadAuthenticationArguments(runtime, assets, "finance-client", "bootstrap.key"); err == nil {
		t.Fatal("unknown mode fell back")
	}
	if err := os.WriteFile(mode, []byte("bootstrap\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = machineWorkloadAuthenticationArguments(runtime, assets, "finance-client", "bootstrap.key")
	if err != nil || !reflect.DeepEqual(got, []string{"--bootstrap-key", "bootstrap.key"}) {
		t.Fatal("legacy explicit bootstrap changed", got, err)
	}
}
