//go:build linux || darwin

package enrollment

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func administrationFixture(t *testing.T) (ServerConfig, *x509.CertPool, tls.Certificate, *operatordelegation.Signer) {
	t.Helper()
	config, roots := serverConfig(t)
	directory := filepath.Dir(config.DatabaseFile)
	caData := mustReadEnrollmentFile(t, config.CertificateFile)
	caBlock, _ := pem.Decode(caData)
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	keyBlock, _ := pem.Decode(mustReadEnrollmentFile(t, config.KeyFile))
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	caKey := key.(*ecdsa.PrivateKey)
	ca.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	rootDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err = x509.ParseCertificate(rootDER)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.CertificateFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	roots = x509.NewCertPool()
	roots.AddCert(ca)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(2), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	client := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: caKey}
	public, signingKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(der)
	admin := &AdministrationConfig{
		Listen: "127.0.0.1:9444", Audience: "https://localhost:9444",
		CertificateFile: config.CertificateFile, KeyFile: config.KeyFile,
		ClientCAFile: config.CertificateFile, CatalogFile: filepath.Join(directory, "catalog.json"),
		DelegationTrustFile: filepath.Join(directory, "trust.json"),
	}
	config.Administration = admin
	catalog := Config{
		Version: 1, InvitationLifetimeSeconds: 600,
		Organizations: map[string]Organization{"company": {Profiles: []string{"standard"}, Types: []string{"laptop"}}},
	}
	grant := operatorauth.Grant{Issuer: "https://identity.example", Subject: "operator",
		Organizations: []string{"company"}, Permissions: []string{"read", "cancel"}}
	trust := operatordelegation.Config{Version: 1, Audience: admin.Audience,
		Keys: []operatordelegation.TrustedKey{{KeyID: "room", PublicKey: base64.RawURLEncoding.EncodeToString(public),
			CertificateSHA256: hex.EncodeToString(pin[:])}}, Grants: []operatorauth.Grant{grant}}
	for path, value := range map[string]any{admin.CatalogFile: catalog, admin.DelegationTrustFile: trust} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	signer, err := operatordelegation.NewSigner(operatordelegation.SignerConfig{Version: 1, Audience: admin.Audience, KeyID: "room"}, signingKey)
	if err != nil {
		t.Fatal(err)
	}
	return config, roots, client, signer
}

func mustReadEnrollmentFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestEnrollmentAdministrationOwnsDeviceRegistry(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/must/not/be/read")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	config, roots, clientCertificate, signer := administrationFixture(t)
	device, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	admin, err := newAdministrationServer(*config.Administration, store)
	if err != nil {
		t.Fatal(err)
	}
	deviceServer := httptest.NewUnstartedServer(device.Handler)
	deviceServer.TLS = device.TLSConfig
	deviceServer.StartTLS()
	defer deviceServer.Close()
	adminServer := httptest.NewUnstartedServer(admin.Handler)
	adminServer.TLS = admin.TLSConfig
	adminServer.StartTLS()
	defer adminServer.Close()
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13,
			Certificates: []tls.Certificate{clientCertificate}},
	}}
	defer client.CloseIdleConnections()
	invitation, _ := createInvitation(t, store, time.Now())
	call := func(target, path, host, token string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequest("GET", target+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Host = host
		if token != "" {
			request.Header.Set(operatordelegation.Header, token)
		}
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, body
	}
	sign := func(path string) string {
		t.Helper()
		request := httptest.NewRequest("GET", config.Administration.Audience+path, nil)
		token, err := signer.Sign(request, nil, operatorauth.Identity{Issuer: "https://identity.example", Subject: "operator"}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	path := operatordelegation.Prefix + "invitations?organization=company"
	token := sign(path)
	if code, data := call(adminServer.URL, path, "localhost:9444", token); code != 200 || !bytes.Contains(data, []byte(invitation.ID)) {
		t.Fatalf("single-owner registry not visible through API: %d %s", code, data)
	}
	if code, _ := call(adminServer.URL, path, "localhost:9444", token); code != 403 {
		t.Fatalf("delegation replay accepted: %d", code)
	}
	cancelPath := operatordelegation.Prefix + "invitations/" + invitation.ID + "/cancel"
	body := []byte(`{"organization":"company","revision":1,"key_fingerprint":"","reason":"Owned service test"}`)
	signedRequest := httptest.NewRequest("POST", config.Administration.Audience+cancelPath, bytes.NewReader(body))
	cancelToken, err := signer.Sign(signedRequest, body, operatorauth.Identity{
		Issuer: "https://identity.example", Subject: "operator",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest("POST", adminServer.URL+cancelPath, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "localhost:9444"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(operatordelegation.Header, cancelToken)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("cancel=%d %s err=%v", response.StatusCode, result, err)
	}
	record, err := store.Get(context.Background(), "company", invitation.ID, time.Now())
	if err != nil || record.State != "cancelled" {
		t.Fatalf("administration did not update owner registry: %+v %v", record, err)
	}
	for _, test := range []struct {
		target, path, host, token string
		want                      int
	}{
		{adminServer.URL, path, "localhost:9444", "", 403},
		{adminServer.URL, path, "wrong.example", sign(path), 421},
		{adminServer.URL, DeviceAPIPrefix + "challenges", "localhost:9444", "", 403},
		{adminServer.URL, APIPrefix + "catalog", "localhost:9444", "", 403},
		{deviceServer.URL, path, "localhost:9443", sign(path), 404},
	} {
		if code, _ := call(test.target, test.path, test.host, test.token); code != test.want {
			t.Fatalf("%s status=%d want=%d", test.path, code, test.want)
		}
	}
	noCertificate := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS13},
	}}
	defer noCertificate.CloseIdleConnections()
	if response, err := noCertificate.Get(adminServer.URL + path); err == nil {
		response.Body.Close()
		t.Fatal("administration accepted no client certificate")
	}
}

func TestEnrollmentAdministrationConfiguration(t *testing.T) {
	config, _, _, _ := administrationFixture(t)
	if err := config.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		edit func(*AdministrationConfig)
	}{
		{"wildcard", func(c *AdministrationConfig) { c.Listen = "0.0.0.0:9444" }},
		{"public", func(c *AdministrationConfig) { c.Listen = "8.8.8.8:9444" }},
		{"hostname", func(c *AdministrationConfig) { c.Listen = "localhost:9444" }},
		{"same-listener", func(c *AdministrationConfig) { c.Listen = config.Listen }},
		{"ephemeral", func(c *AdministrationConfig) { c.Listen = "127.0.0.1:0" }},
		{"http", func(c *AdministrationConfig) { c.Audience = "http://localhost:9444" }},
		{"path", func(c *AdministrationConfig) { c.Audience += "/" }},
		{"query", func(c *AdministrationConfig) { c.Audience += "?" }},
		{"fragment", func(c *AdministrationConfig) { c.Audience += "#" }},
		{"wrong-port", func(c *AdministrationConfig) { c.Audience = "https://localhost:9445" }},
		{"missing-ca", func(c *AdministrationConfig) { c.ClientCAFile = "" }},
		{"missing-catalog", func(c *AdministrationConfig) { c.CatalogFile = "" }},
		{"missing-trust", func(c *AdministrationConfig) { c.DelegationTrustFile = "" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := config
			admin := *config.Administration
			input.Administration = &admin
			test.edit(&admin)
			if input.Validate() == nil {
				t.Fatal("unsafe administration configuration accepted")
			}
		})
	}
	path := filepath.Join(filepath.Dir(config.DatabaseFile), "device.json")
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServerConfig(path); err != nil {
		t.Fatalf("administration config was not loadable: %v", err)
	}
	admin := *config.Administration
	admin.DelegationTrustFile = admin.CatalogFile
	store, err := Open(config.DatabaseFile)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := newAdministrationServer(admin, store); err == nil {
		t.Fatal("invalid delegation trust accepted")
	}
}

func TestEnrollmentListenerBindingIsAtomic(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := first.Addr().String()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	err = runEnrollmentServers(context.Background(), []*http.Server{
		{Addr: address}, {Addr: occupied.Addr().String()},
	})
	if err == nil {
		t.Fatal("occupied administration listener was ignored")
	}
	reopened, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("device listener leaked on administration bind failure: %v", err)
	}
	reopened.Close()
	config, _, _, _ := administrationFixture(t)
	server, store, err := newDeviceServer(config)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server.Addr = "127.0.0.1:0"
	admin, err := newAdministrationServer(*config.Administration, store)
	if err != nil {
		t.Fatal(err)
	}
	admin.Addr = "127.0.0.1:0"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- runEnrollmentServers(ctx, []*http.Server{server, admin}) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dual listeners did not shut down")
	}
}
