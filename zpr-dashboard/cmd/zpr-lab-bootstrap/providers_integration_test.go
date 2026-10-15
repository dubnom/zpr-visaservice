package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-ldap/ldap/v3"
	"golang.org/x/net/html"
)

func TestLabIdentityLiveProviders(t *testing.T) {
	ldapImage, dexImage := os.Getenv("ZPR_LAB_LDAP_VALIDATION_IMAGE"), os.Getenv("ZPR_LAB_DEX_VALIDATION_IMAGE")
	if ldapImage == "" && dexImage == "" {
		t.Skip("isolated live-provider validation requires explicit local LDAP and Dex image IDs")
	}
	for _, image := range []string{ldapImage, dexImage} {
		if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(image) {
			t.Fatal("both live-provider images must be explicit immutable local image IDs")
		}
	}
	t.Setenv("ZPR_SIMULATOR_URL", "https://simulator-unavailable.invalid")
	config := testProviderConfig()
	files, err := generateConfigured("lab.example.test", time.Now(), nil, &config)
	if err != nil {
		t.Fatal(err)
	}
	ldapDirectory, dexDirectory := t.TempDir(), t.TempDir()
	for directory, names := range map[string][]string{
		ldapDirectory: {"slapd.conf", "directory.ldif", "initialize-directory.sh", "serve-directory.sh", "ldap-chain.pem", "ldap-key.pem", "lab-ca.pem"},
		dexDirectory:  {"dex.yaml", "oidc-chain.pem", "oidc-key.pem", "lab-ca.pem"},
	} {
		for _, name := range names {
			if err := os.WriteFile(filepath.Join(directory, name), files[name], 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	id := make([]byte, 8)
	if _, err := rand.Read(id); err != nil {
		t.Fatal(err)
	}
	network := "zpr-lab-identity-test-" + hex.EncodeToString(id)
	dockerCommand(t, "network", "create", network)
	t.Cleanup(func() { dockerCommand(t, "network", "rm", network) })
	user := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	ldapState := t.TempDir()
	dockerCommand(t, "run", "--rm", "--pull", "never", "--network", "none",
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--user", user, "--tmpfs", "/tmp:rw",
		"--mount", "type=bind,source="+ldapState+",target=/var/lib/ldap",
		"--mount", "type=bind,source="+ldapDirectory+",target=/etc/zpr/lab,readonly",
		"--entrypoint", "/bin/sh", ldapImage, "/etc/zpr/lab/initialize-directory.sh")
	ldapID := dockerCommand(t, "run", "-d", "--pull", "never",
		"--network", network, "--network-alias", "ldap.lab.example.test",
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--user", user, "--tmpfs", "/tmp:rw",
		"--mount", "type=bind,source="+ldapState+",target=/var/lib/ldap",
		"--publish", "127.0.0.1::1636",
		"--mount", "type=bind,source="+ldapDirectory+",target=/etc/zpr/lab,readonly",
		"--entrypoint", "/bin/sh", ldapImage, "/etc/zpr/lab/serve-directory.sh")
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(dockerCommand(t, "logs", ldapID))
		}
		dockerCommand(t, "rm", "-f", ldapID)
	})
	ldapAddress := dockerCommand(t, "port", ldapID, "1636/tcp")
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(files["lab-ca.pem"]) {
		t.Fatal("invalid generated lab CA")
	}
	ldapTLS := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "ldap.lab.example.test"}
	readerDN := "cn=zpr-reader,ou=Services,dc=lab,dc=zpr"
	password := func(name string) string { return strings.TrimSpace(string(files[name+".secret"])) }
	connection := waitLDAP(t, ldapAddress, ldapTLS, readerDN, password("ldap-reader"))
	defer connection.Close()
	guardContext, guardCancel := context.WithTimeout(context.Background(), 10*time.Second)
	guardOutput, guardErr := exec.CommandContext(guardContext, "docker", "exec", ldapID,
		"/bin/sh", "/etc/zpr/lab/initialize-directory.sh").CombinedOutput()
	guardCancel()
	if guardErr == nil || !strings.Contains(string(guardOutput), "existing or partial state is never reset") {
		t.Fatal("directory initializer did not explicitly refuse existing state")
	}
	search, err := connection.Search(ldap.NewSearchRequest(
		"ou=People,dc=lab,dc=zpr", ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 10, 3, false,
		"(uid=lab-operator)", []string{"uid", "entryUUID", "userPassword"}, nil))
	if err != nil || len(search.Entries) != 1 ||
		search.Entries[0].GetAttributeValue("entryUUID") == "" ||
		len(search.Entries[0].GetAttributeValues("userPassword")) != 0 {
		t.Fatal("reader cannot search a stable user identity or can read password hashes")
	}
	change := ldap.NewModifyRequest(search.Entries[0].DN, nil)
	change.Replace("sn", []string{"Unauthorized"})
	if err := connection.Modify(change); !ldap.IsErrorWithCode(err, ldap.LDAPResultInsufficientAccessRights) {
		t.Fatal("read-only LDAP account can mutate directory")
	}
	if err := connection.Bind(search.Entries[0].DN, "incorrect-password"); !ldap.IsErrorWithCode(err, ldap.LDAPResultInvalidCredentials) {
		t.Fatal("incorrect LDAP password accepted")
	}
	dexState := t.TempDir()
	dexID := dockerCommand(t, "run", "-d", "--pull", "never",
		"--network", network, "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges", "--user", user,
		"--tmpfs", "/tmp:rw",
		"--mount", "type=bind,source="+dexState+",target=/var/lib/dex",
		"--publish", "127.0.0.1::5556",
		"--mount", "type=bind,source="+dexDirectory+",target=/etc/zpr/lab,readonly",
		"--entrypoint", "dex", dexImage, "serve", "/etc/zpr/lab/dex.yaml")
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(dockerCommand(t, "logs", dexID))
		}
		dockerCommand(t, "rm", "-f", dexID)
	})
	dexAddress := dockerCommand(t, "port", dexID, "5556/tcp")
	issuerURL, _ := url.Parse(config.OIDCOrigin)
	transport := &http.Transport{
		Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != issuerURL.Host {
				return nil, fmt.Errorf("unexpected provider connection target")
			}
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, dexAddress)
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(oidc.ClientContext(context.Background(), client), 40*time.Second)
	defer cancel()
	var provider *oidc.Provider
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		provider, err = oidc.NewProvider(ctx, config.OIDCOrigin)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if provider == nil {
		t.Fatal("generated Dex configuration did not serve verified HTTPS OIDC discovery")
	}
	operator := providerLogin(t, ctx, client, provider, config, "lab-operator", password("lab-operator"), password("oidc-room-client"))
	observer := providerLogin(t, ctx, client, provider, config, "lab-observer", password("lab-observer"), password("oidc-room-client"))
	if operator.Subject == "" || operator.Subject == observer.Subject ||
		operator.Email != "lab-operator@lab.example.test" ||
		len(operator.Groups) != 1 || operator.Groups[0] != "lab-operators" ||
		len(observer.Groups) != 1 || observer.Groups[0] != "lab-observers" {
		t.Fatal("live OIDC user/subject/group mapping does not match generated directory")
	}
	dockerCommand(t, "restart", dexID)
	dockerCommand(t, "restart", ldapID)
	restartedLDAP := waitLDAP(t, dockerCommand(t, "port", ldapID, "1636/tcp"), ldapTLS, readerDN, password("ldap-reader"))
	restartedLDAP.Close()
	transport.CloseIdleConnections()
	dexAddress = dockerCommand(t, "port", dexID, "5556/tcp")
	deadline = time.Now().Add(25 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		if _, err := oidc.NewProvider(ctx, config.OIDCOrigin); err == nil {
			ready = true
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !ready {
		t.Fatal("Dex did not return after restart with its preserved state")
	}
	again := providerLogin(t, ctx, client, provider, config, "lab-operator", password("lab-operator"), password("oidc-room-client"))
	if again.Subject != operator.Subject {
		t.Fatal("Dex restart changed the issuer's user subject")
	}
	t.Log("verified LDAPS, reader ACLs, real Dex authorization-code/PKCE login, signed OIDC issuer/audience/nonce/subjects/groups, and LDAP/Dex restart without Simulator")
}

func waitLDAP(t *testing.T, address string, config *tls.Config, dn, password string) *ldap.Conn {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		socket, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", address, config)
		if err == nil {
			candidate := ldap.NewConn(socket, true)
			candidate.Start()
			candidate.SetTimeout(3 * time.Second)
			if err := candidate.Bind(dn, password); err == nil {
				return candidate
			}
			candidate.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("generated OpenLDAP configuration did not become ready over verified LDAPS")
	return nil
}

type providerClaims struct {
	Subject string   `json:"sub"`
	Email   string   `json:"email"`
	Groups  []string `json:"groups"`
	Nonce   string   `json:"nonce"`
}

func providerLogin(t *testing.T, ctx context.Context, client *http.Client, provider *oidc.Provider, config providerConfig, username, password, clientSecret string) providerClaims {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	browser := &http.Client{Transport: client.Transport, Jar: jar, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	verifier := strings.Repeat("v", 64)
	challenge := sha256.Sum256([]byte(verifier))
	callback := config.RoomOrigin + "/auth/operator/callback"
	query := url.Values{
		"client_id": {"zpr-control-room"}, "redirect_uri": {callback}, "response_type": {"code"},
		"scope": {"openid email profile groups"}, "state": {"test-state"}, "nonce": {"test-nonce"},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"},
	}
	next, err := url.Parse(provider.Endpoint().AuthURL + "?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	var code string
	method := http.MethodGet
	var form url.Values
	for step := 0; step < 15; step++ {
		if next.Scheme != "https" || next.Host != strings.TrimPrefix(config.OIDCOrigin, "https://") {
			t.Fatal("OIDC login attempted to leave selected issuer")
		}
		var body io.Reader
		if method == http.MethodPost {
			body = strings.NewReader(form.Encode())
		}
		request, err := http.NewRequestWithContext(ctx, method, next.String(), body)
		if err != nil {
			t.Fatal(err)
		}
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		response, err := browser.Do(request)
		if err != nil {
			t.Fatal("live OIDC login request failed")
		}
		data, readErr := io.ReadAll(io.LimitReader(response.Body, 256<<10))
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			location, err := response.Location()
			if err != nil {
				t.Fatal(err)
			}
			if location.Scheme+"://"+location.Host+location.Path == callback {
				code = location.Query().Get("code")
				if code == "" || location.Query().Get("state") != "test-state" {
					t.Fatal("live OIDC callback missing code/state")
				}
				break
			}
			next, method = location, http.MethodGet
			continue
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("live OIDC login returned HTTP %d", response.StatusCode)
		}
		action, values := providerForm(t, data)
		target, err := url.Parse(action)
		if err != nil {
			t.Fatal(err)
		}
		next = next.ResolveReference(target)
		if _, ok := values["login"]; ok {
			values.Set("login", username)
			values.Set("password", password)
		} else {
			values.Set("approval", "approve")
		}
		method, form = http.MethodPost, values
	}
	if code == "" {
		t.Fatal("live OIDC login did not produce an authorization code")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.Endpoint().TokenURL,
		strings.NewReader(url.Values{"grant_type": {"authorization_code"}, "code": {code},
			"redirect_uri": {callback}, "code_verifier": {verifier}}.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth("zpr-control-room", clientSecret)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("live OIDC token request failed")
	}
	defer response.Body.Close()
	var tokens struct {
		IDToken string `json:"id_token"`
	}
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&tokens) != nil {
		t.Fatal("live OIDC token exchange failed")
	}
	token, err := provider.Verifier(&oidc.Config{ClientID: "zpr-control-room"}).Verify(ctx, tokens.IDToken)
	if err != nil {
		t.Fatal("live OIDC signature/issuer/audience verification failed")
	}
	var claims providerClaims
	if err := token.Claims(&claims); err != nil || claims.Nonce != "test-nonce" {
		t.Fatal("live OIDC claims/nonce verification failed")
	}
	return claims
}

func providerForm(t *testing.T, data []byte) (string, url.Values) {
	t.Helper()
	root, err := html.Parse(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{}
	action, found := "", false
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "form" {
			found = true
			for _, attr := range node.Attr {
				if attr.Key == "action" {
					action = attr.Val
				}
			}
		}
		if node.Type == html.ElementNode && node.Data == "input" {
			name, value := "", ""
			for _, attr := range node.Attr {
				switch attr.Key {
				case "name":
					name = attr.Val
				case "value":
					value = attr.Val
				}
			}
			if name != "" {
				form.Set(name, value)
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	if !found {
		t.Fatal("live Dex page has no login/consent form")
	}
	return action, form
}

func dockerCommand(t *testing.T, args ...string) string {
	t.Helper()
	timeout := 30 * time.Second
	if len(args) > 0 && args[0] == "compose" {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("isolated provider Docker operation %s failed: %v %s", args[0], err, output)
	}
	return strings.TrimSpace(string(output))
}
