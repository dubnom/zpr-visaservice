package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// This disposable smoke runs the real companion executable against the real
// public registry handlers over verified mTLS. No Docker or installed service.
func TestOrdinaryCompanionPublicContractSmoke(t *testing.T) {
	runtime, entries, company, _ := registryEmpty(t)
	starter := entries[0]
	starter.Name = "directory_fixture"
	starter.DisplayName = "Starter evaluation directory"
	companySource := entries[1]
	companySource.Name = "company_directory"
	starter.Organizations = map[string]assertionSourceScope{"northstar": starter.Organizations["northstar"]}
	config := assertionSourcesConfig{DefaultSource: starter.Name, Sources: []assertionSourceConfig{starter, companySource}}
	if err := runtime.persistSources(config); err != nil {
		t.Fatal(err)
	}
	if err := runtime.buildSources(config); err != nil {
		t.Fatal(err)
	}
	runtime.sourcesConfig = config
	ca, caKey, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	serverCert, _, err := createTestPolicyLeaf(ca, caKey, "control-service", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, true)
	if err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey, err := createTestPolicyLeaf(ca, caKey, "ordinary-setup", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	mux := http.NewServeMux()
	runtime.register(mux)
	service := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TLS.VerifiedChains) == 0 || r.TLS.PeerCertificates[0].Subject.CommonName != "ordinary-setup" {
			w.WriteHeader(403)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	service.TLS = &tls.Config{Certificates: []tls.Certificate{serverCert}, MinVersion: tls.VersionTLS12, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots}
	service.StartTLS()
	defer service.Close()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, Certificates: []tls.Certificate{clientCert}, MinVersion: tls.VersionTLS12}}}
	defer client.CloseIdleConnections()
	scratch := t.TempDir()
	caPath, certPath, keyPath := filepath.Join(scratch, "api-ca.pem"), filepath.Join(scratch, "client.pem"), filepath.Join(scratch, "client-key.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSCertificate(certPath, clientCert); err != nil {
		t.Fatal(err)
	}
	if err := writeTLSPrivateKey(keyPath, clientKey); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(scratch, "companion")
	build := exec.Command("go", "build", "-o", binary, "../zpr-simulator-companion")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	state := filepath.Join(scratch, "private-source")
	invalidMode := exec.Command(binary, "-mode", "legacy-lab", "-action", "init", "-state", state)
	if err := invalidMode.Run(); err == nil {
		t.Fatal("legacy setup mode accepted")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("invalid mode provisioned state")
	}
	common := []string{"-mode", "ordinary-client", "-state", state}
	api := append(append([]string{}, common...), "-endpoint", service.URL, "-api-ca", caPath, "-client-cert", certPath, "-client-key", keyPath)
	init := exec.Command(binary, append(api, "-action", "init", "-source", "companion_directory", "-starter-source", starter.Name, "-port", fmt.Sprint(port))...)
	if output, err := init.CombinedOutput(); err != nil {
		t.Fatalf("init: %v %s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	serve := exec.CommandContext(ctx, binary, append(api, "-action", "serve")...)
	output, err := os.OpenFile(filepath.Join(scratch, "companion.log"), os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	serve.Stdout, serve.Stderr = output, output
	if err := serve.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	stop := func() {
		if !stopped {
			_ = serve.Process.Signal(os.Interrupt)
			_ = serve.Wait()
			stopped = true
		}
	}
	defer stop()
	registered := false
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		response, err := client.Get(service.URL + "/api/assertions/sources")
		if err != nil {
			t.Fatal(err)
		}
		var data struct {
			Sources []struct {
				ID string `json:"source_id"`
			} `json:"sources"`
		}
		err = json.NewDecoder(response.Body).Decode(&data)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range data.Sources {
			if source.ID == "companion_directory" {
				registered = true
			}
		}
		if registered {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !registered {
		t.Fatal("companion failed public registration")
	}
	response, err := client.Get(service.URL + "/api/assertions/source?source=companion_directory")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("companion read: %d", response.StatusCode)
	}
	if company.searches.Load() != 0 {
		t.Fatal("synthetic setup touched bundled LDAP")
	}
	stop()
	// The synthetic provider is down; an independently selected starter still reads.
	directory, err := runtime.sources[starter.Name].read(ctx, "northstar", "", "")
	if err != nil || len(directory.People) == 0 {
		t.Fatalf("starter after stop: %v", err)
	}
	if company.searches.Load() == 0 {
		t.Fatal("starter was not exercised")
	}
	response, err = client.Get(service.URL + "/api/assertions/source?source=companion_directory")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 502 {
		t.Fatalf("absent companion: %d", response.StatusCode)
	}
	unconfirmed := exec.Command(binary, append(api, "-action", "remove")...)
	if err := unconfirmed.Run(); err == nil {
		t.Fatal("unconfirmed removal accepted")
	}
	remove := exec.Command(binary, append(api, "-action", "remove", "-confirm-source", "companion_directory")...)
	if output, err := remove.CombinedOutput(); err != nil {
		t.Fatalf("remove: %v %s", err, output)
	}
	runtime.mu.Lock()
	if len(runtime.sourcesConfig.Sources) != 2 || !reflect.DeepEqual(runtime.sourcesConfig.Sources[0], starter) || !reflect.DeepEqual(runtime.sourcesConfig.Sources[1], companySource) || runtime.defaultSource != starter.Name {
		t.Error("starter configuration/default changed")
	}
	runtime.mu.Unlock()
	restarted, err := newAssertionRuntime()
	if err != nil {
		t.Fatal(err)
	}
	if len(restarted.sources) != 2 || restarted.defaultSource != starter.Name {
		t.Fatal("cleanup did not persist")
	}
}
