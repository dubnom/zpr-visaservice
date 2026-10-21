package main

import (
	"bytes"
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
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

const runtimeSimulatorOrigin = "https://localhost:8890"

type runtimeOutputConfig struct {
	ControlServiceOrigin     string
	BindIP                   string
	OrganizationID           string
	OrganizationName         string
	OperatorIssuer           string
	OperatorSubject          string
	OperatorIdentityVerified bool
}

func runtimeOutputsConfigured(
	identityInputDir, output, domain string,
	now time.Time,
	providers providerConfig,
	input runtimeOutputConfig,
) error {
	if runtime.GOOS != "linux" {
		return errors.New("final runtime output must be generated on the Ubuntu/Linux service-state owner")
	}
	files, err := prepareRuntimeOutputFiles(identityInputDir, output, domain, now, providers, input)
	if err != nil {
		return err
	}
	return writeRuntimeOutputFiles(output, files)
}

func prepareRuntimeOutputFiles(
	identityInputDir, output, domain string,
	now time.Time,
	providers providerConfig,
	input runtimeOutputConfig,
) (map[string][]byte, error) {
	if !filepath.IsAbs(identityInputDir) || filepath.Clean(identityInputDir) != identityInputDir ||
		!filepath.IsAbs(output) || filepath.Clean(output) != output || output == string(filepath.Separator) {
		return nil, errors.New("identity-input-dir and output must be canonical absolute paths")
	}
	if err := providers.validate(domain); err != nil {
		return nil, err
	}
	if providers.SimulatorOrigin != runtimeSimulatorOrigin {
		return nil, errors.New("runtime outputs require the protected Simulator loopback origin https://localhost:8890")
	}
	if err := input.validate(domain, providers); err != nil {
		return nil, err
	}
	sourceFiles, err := readIdentityInput(identityInputDir, domain, providers, input, now)
	if err != nil {
		return nil, err
	}
	if err := prepareSimulatorLoopbackOverlay(sourceFiles, domain, providers.SimulatorOrigin, now); err != nil {
		return nil, err
	}
	var identityInventory inventory
	if err := json.Unmarshal(sourceFiles["lab-bootstrap.json"], &identityInventory); err != nil ||
		identityInventory.IdentityServices == nil || identityInventory.IdentityServices.Deployment == nil {
		return nil, errors.New("identity input inventory lacks its pinned identity image references")
	}
	files := make(map[string][]byte)
	if err := renderRuntimeOutputs(output, domain, now, providers, input, sourceFiles, files); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(map[string]any{
		"schema_version":                 1,
		"kind":                           "zpr-demo-runtime-outputs",
		"domain":                         domain,
		"identity_input_dir":             identityInputDir,
		"control_room_origin":            providers.RoomOrigin,
		"control_service_origin":         input.ControlServiceOrigin,
		"simulator_origin":               providers.SimulatorOrigin,
		"oidc_issuer":                    providers.OIDCOrigin,
		"identity_platform":              "linux/amd64",
		"openldap_qualification":         "demo-only and unqualified; image digest syntax is checked but no image inspection is performed",
		"ldap_image_reference":           identityInventory.IdentityServices.Deployment.LDAPImage,
		"dex_image_reference":            identityInventory.IdentityServices.Deployment.DexImage,
		"operator_identity_verification": "caller-confirmed against a successful named IdP login; offline generator did not contact the provider",
		"simulator_operator_permissions": []string{"organization.read", "scenario.read", "scenario.run"},
		"generated_at":                   now.UTC().Truncate(time.Second),
	}, "", "  ")
	if err != nil {
		return nil, errors.New("cannot encode runtime output inventory")
	}
	files["runtime-output.json"] = append(manifest, '\n')
	return files, nil
}

func writeRuntimeOutputFiles(output string, files map[string][]byte) error {
	if !filepath.IsAbs(output) || filepath.Clean(output) != output || output == string(filepath.Separator) {
		return errors.New("runtime output must be a canonical absolute path to a new directory")
	}
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return errors.New("cannot open runtime output parent")
	}
	defer parent.Close()
	parentInfo, err := parent.Stat(".")
	if err != nil || parentInfo.Mode().Perm()&0o022 != 0 {
		return errors.New("runtime output parent must be protected from group and other writes")
	}
	if stat, ok := parentInfo.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
		return errors.New("runtime output parent must be owned by the invoking service-state owner")
	}
	if err := parent.Mkdir(filepath.Base(output), 0o700); err != nil {
		return errors.New("cannot create runtime output directory; existing paths are never reused or overwritten")
	}
	root, err := parent.OpenRoot(filepath.Base(output))
	if err != nil {
		return errors.New("cannot open new runtime output directory; inspect partial output before retrying")
	}
	defer root.Close()
	for _, directory := range []string{"simulator-operator-auth", "budget-state", "identity-overlay"} {
		if err := root.Mkdir(directory, 0o700); err != nil {
			return errors.New("cannot create private runtime output directories; inspect partial output before retrying")
		}
	}
	names := make([]string, 0, len(files)-1)
	for name := range files {
		if name != "runtime-output.json" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	names = append(names, "runtime-output.json")
	for _, name := range names {
		if err := writePrivate(root, name, files[name]); err != nil {
			return fmt.Errorf("cannot persist %s; runtime output is incomplete, inspect it before retrying", name)
		}
	}
	return nil
}

func readIdentityInput(directory, domain string, providers providerConfig, input runtimeOutputConfig, now time.Time) (map[string][]byte, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return nil, errors.New("identity-input-dir must be an existing non-symlink mode-0700 directory")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
		return nil, errors.New("identity-input-dir must be owned by the invoking service-state owner")
	}
	required := []string{
		"lab-bootstrap.json", "identity-inputs.json", "lab-ca.pem", "lab-ca-key.pem",
		"room-chain.pem", "room-key.pem", "simulator-chain.pem", "simulator-key.pem",
		"oidc-room-client.secret", "oidc-simulator-client.secret", "identity-compose.yaml", "dex.yaml",
	}
	files := make(map[string][]byte, len(required))
	for _, name := range required {
		path := filepath.Join(directory, name)
		fileInfo, err := os.Lstat(path)
		if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 ||
			fileInfo.Mode().Perm() != 0o600 {
			return nil, fmt.Errorf("identity input %s must be a regular mode-0600 file", name)
		}
		if stat, ok := fileInfo.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
			return nil, fmt.Errorf("identity input %s must be owned by the invoking service-state owner", name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("cannot read identity input %s", name)
		}
		files[name] = data
	}
	var record inventory
	if err := json.Unmarshal(files["lab-bootstrap.json"], &record); err != nil ||
		record.Domain != domain || record.IdentityServices == nil {
		return nil, errors.New("identity input inventory must match the explicit domain and service origins")
	}
	identityProviders := *record.IdentityServices
	if identityProviders.RoomOrigin != providers.RoomOrigin ||
		identityProviders.OIDCOrigin != providers.OIDCOrigin ||
		!simulatorOriginCompatible(identityProviders.SimulatorOrigin, providers.SimulatorOrigin, domain) {
		return nil, errors.New("identity input inventory must match the explicit domain, Room, Simulator and OIDC origins")
	}
	if identityProviders.Deployment == nil ||
		identityProviders.Deployment.Platform != "linux/amd64" ||
		identityProviders.Deployment.BindIP != input.BindIP ||
		identityProviders.Deployment.UID != os.Geteuid() ||
		identityProviders.Deployment.GID != os.Getegid() {
		return nil, errors.New("runtime outputs require the matching linux/amd64 identity-compose bootstrap and bind IP")
	}

	if err := identityProviders.validate(domain); err != nil {
		return nil, errors.New("identity input inventory does not contain valid immutable identity image references")
	}
	var handoff struct {
		SchemaVersion int    `json:"schema_version"`
		Kind          string `json:"kind"`
		Issuer        string `json:"issuer"`
		CAFile        string `json:"ca_file"`
		GrantStatus   string `json:"grant_status"`
		Clients       struct {
			Room struct {
				ClientID     string `json:"client_id"`
				ClientSecret string `json:"client_secret_file"`
				RedirectURL  string `json:"redirect_url"`
			} `json:"room"`
			Simulator struct {
				ClientID     string `json:"client_id"`
				ClientSecret string `json:"client_secret_file"`
				RedirectURL  string `json:"redirect_url"`
			} `json:"simulator"`
		} `json:"clients"`
	}
	if err := json.Unmarshal(files["identity-inputs.json"], &handoff); err != nil ||
		handoff.SchemaVersion != 1 || handoff.Kind != "simulator-lab-identity-inputs" ||
		handoff.Issuer != providers.OIDCOrigin || handoff.CAFile != "lab-ca.pem" ||
		handoff.GrantStatus == "" || !strings.HasPrefix(handoff.GrantStatus, "not-configured:") ||
		handoff.Clients.Room.ClientID != "zpr-control-room" ||
		handoff.Clients.Room.ClientSecret != "oidc-room-client.secret" ||
		handoff.Clients.Room.RedirectURL != providers.RoomOrigin+"/auth/operator/callback" ||
		handoff.Clients.Simulator.ClientID != "zpr-simulator" ||
		handoff.Clients.Simulator.ClientSecret != "oidc-simulator-client.secret" ||
		handoff.Clients.Simulator.RedirectURL != identityProviders.SimulatorOrigin+"/auth/operator/callback" {
		return nil, errors.New("identity input handoff must contain the expected OIDC clients and remain grant-free")
	}
	if err := systemconfig.CheckServerCertificate(providers.RoomOrigin, files["room-chain.pem"], files["room-key.pem"], now); err != nil {
		return nil, fmt.Errorf("identity input Control Room certificate is invalid: %w", err)
	}
	if err := systemconfig.CheckServerCertificate(identityProviders.SimulatorOrigin, files["simulator-chain.pem"], files["simulator-key.pem"], now); err != nil {
		return nil, fmt.Errorf("identity input Simulator certificate is invalid: %w", err)
	}
	if identityProviders.SimulatorOrigin == runtimeSimulatorOrigin {
		path := filepath.Join(directory, "simulator-loopback-ca.pem")
		fileInfo, err := os.Lstat(path)
		if err != nil || !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 ||
			fileInfo.Mode().Perm() != 0o600 {
			return nil, errors.New("loopback Simulator identity requires a private simulator-loopback-ca.pem")
		}
		if stat, ok := fileInfo.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
			return nil, errors.New("loopback Simulator CA must be owned by the invoking service-state owner")
		}
		files["simulator-loopback-ca.pem"], err = os.ReadFile(path)
		if err != nil || validateLoopbackSimulatorCertificate(files["simulator-chain.pem"], files["simulator-loopback-ca.pem"], now) != nil {
			return nil, errors.New("identity input loopback Simulator certificate or CA is invalid")
		}
	}
	if err := validateInitializedIdentityState(directory); err != nil {
		return nil, err
	}
	ca, err := x509.ParseCertificate(firstPEMCertificate(files["lab-ca.pem"]))
	if err != nil || ca == nil || !ca.IsCA {
		return nil, errors.New("identity input lab-ca.pem is invalid")
	}
	caKey, err := parseCAKey(files["lab-ca-key.pem"])
	if err != nil {
		return nil, errors.New("identity input lab-ca-key.pem is invalid")
	}
	publicKey, err := x509.MarshalPKIXPublicKey(&caKey.PublicKey)
	if err != nil || !bytes.Equal(publicKey, ca.RawSubjectPublicKeyInfo) {
		return nil, errors.New("identity input CA key does not match its certificate")
	}
	for _, name := range []string{"oidc-room-client.secret", "oidc-simulator-client.secret"} {
		secret := strings.TrimSuffix(strings.TrimSuffix(string(files[name]), "\n"), "\r")
		if secret == "" || strings.TrimSpace(secret) != secret || strings.ContainsAny(secret, "\r\n\x00") {
			return nil, fmt.Errorf("identity input %s is not a valid single-line client secret", name)
		}
	}
	return files, nil
}

func simulatorOriginCompatible(source, target, domain string) bool {
	if source == target {
		return true
	}
	sourceURL, sourceErr := url.Parse(source)
	targetURL, targetErr := url.Parse(target)
	return sourceErr == nil && targetErr == nil &&
		sourceURL.Scheme == "https" && targetURL.Scheme == "https" &&
		sourceURL.Hostname() == "simulator."+domain && targetURL.Hostname() == "localhost" &&
		sourceURL.Port() != "" && sourceURL.Port() == targetURL.Port() &&
		source == "https://"+sourceURL.Host && target == "https://"+targetURL.Host
}

func prepareSimulatorLoopbackOverlay(files map[string][]byte, domain, targetOrigin string, now time.Time) error {
	var record inventory
	if err := json.Unmarshal(files["lab-bootstrap.json"], &record); err != nil || record.IdentityServices == nil {
		return errors.New("identity input inventory lacks Simulator service origins")
	}
	sourceOrigin := record.IdentityServices.SimulatorOrigin
	if sourceOrigin == targetOrigin {
		if targetOrigin == runtimeSimulatorOrigin &&
			validateLoopbackSimulatorCertificate(files["simulator-chain.pem"], files["simulator-loopback-ca.pem"], now) != nil {
			return errors.New("identity input loopback Simulator certificate or CA is invalid")
		}
		return nil
	}
	if !simulatorOriginCompatible(sourceOrigin, targetOrigin, domain) {
		return errors.New("identity input cannot be adapted to the requested Simulator loopback origin")
	}

	ca, caKey, caPEM, caKeyPEM, err := certificate("ZPR Simulator loopback CA", "localhost", now, nil, nil)
	if err != nil {
		return errors.New("cannot create Simulator loopback CA")
	}
	clear(caKeyPEM)
	_, _, simulatorPEM, simulatorKeyPEM, err := certificate("ZPR Simulator", "localhost", now, ca, caKey)
	if err != nil {
		caKey.D.SetInt64(0)
		return errors.New("cannot create Simulator loopback certificate")
	}
	caKey.D.SetInt64(0)
	chain := append(append([]byte(nil), simulatorPEM...), caPEM...)
	if err := validateLoopbackSimulatorCertificate(chain, caPEM, now); err != nil {
		return errors.New("generated Simulator loopback certificate is invalid")
	}
	dexOverride, err := dexWithSimulatorRedirect(files["dex.yaml"], sourceOrigin, targetOrigin)
	if err != nil {
		return err
	}
	files["simulator-chain.pem"] = chain
	files["simulator-key.pem"] = simulatorKeyPEM
	files["simulator-loopback-ca.pem"] = caPEM
	files["dex-loopback-override.yaml"] = dexOverride
	return nil
}

func validateLoopbackSimulatorCertificate(chainPEM, caPEM []byte, now time.Time) error {
	caBlock, caRest := pem.Decode(caPEM)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" || len(bytes.TrimSpace(caRest)) != 0 {
		return errors.New("invalid Simulator loopback CA")
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil || !ca.IsCA || !ca.PermittedDNSDomainsCritical ||
		len(ca.PermittedDNSDomains) != 1 || ca.PermittedDNSDomains[0] != "localhost" {
		return errors.New("Simulator loopback CA must be constrained to the exact localhost name")
	}
	leafBlock, rest := pem.Decode(chainPEM)
	if leafBlock == nil || leafBlock.Type != "CERTIFICATE" {
		return errors.New("invalid Simulator loopback certificate chain")
	}
	chainCABlock, rest := pem.Decode(rest)
	if chainCABlock == nil || chainCABlock.Type != "CERTIFICATE" ||
		!bytes.Equal(chainCABlock.Bytes, ca.Raw) || len(bytes.TrimSpace(rest)) != 0 {
		return errors.New("Simulator loopback chain must contain only its exact CA")
	}
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil || leaf.VerifyHostname("localhost") != nil {
		return errors.New("Simulator loopback certificate must cover localhost")
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, err = leaf.Verify(x509.VerifyOptions{
		Roots: roots, DNSName: "localhost", CurrentTime: now,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func dexWithSimulatorRedirect(data []byte, sourceOrigin, targetOrigin string) ([]byte, error) {
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, errors.New("identity input Dex configuration is invalid")
	}
	clients, ok := document["staticClients"].([]any)
	if !ok {
		return nil, errors.New("identity input Dex configuration has no static clients")
	}
	found := false
	for _, raw := range clients {
		client, ok := raw.(map[string]any)
		if !ok || client["id"] != "zpr-simulator" {
			continue
		}
		if found {
			return nil, errors.New("identity input Dex configuration has duplicate Simulator clients")
		}
		redirects, ok := client["redirectURIs"].([]any)
		if !ok || len(redirects) != 1 || redirects[0] != sourceOrigin+"/auth/operator/callback" {
			return nil, errors.New("identity input Dex Simulator callback does not match its inventory")
		}
		client["redirectURIs"] = []any{targetOrigin + "/auth/operator/callback"}
		found = true
	}
	if !found {
		return nil, errors.New("identity input Dex configuration lacks the Simulator client")
	}
	updated, err := yaml.Marshal(document)
	if err != nil {
		return nil, errors.New("cannot encode Dex loopback override")
	}
	return updated, nil
}

func validateInitializedIdentityState(directory string) error {
	for _, name := range []string{"ldap-state", "dex-state"} {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return fmt.Errorf("identity source %s must be an existing private non-symlink directory", name)
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
			return fmt.Errorf("identity source %s must be owned by the invoking service-state owner", name)
		}
	}
	for _, path := range []string{
		filepath.Join(directory, "ldap-state", "lab-initialized"),
		filepath.Join(directory, "ldap-state", "data.mdb"),
		filepath.Join(directory, "dex-state", "dex.db"),
	} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("runtime outputs require the already initialized LDAP state and started Dex state; do not rerun initialization")
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
			return errors.New("initialized LDAP/Dex state must be owned by the invoking service-state owner")
		}
		if strings.HasSuffix(path, "data.mdb") || strings.HasSuffix(path, "dex.db") {
			if info.Size() == 0 {
				return errors.New("initialized LDAP and Dex database files must be nonempty")
			}
		}
	}
	return nil
}

func (config runtimeOutputConfig) validate(domain string, providers providerConfig) error {
	service, err := url.Parse(config.ControlServiceOrigin)
	if err != nil || systemconfig.ValidateManagementEndpoint(config.ControlServiceOrigin, "") != nil ||
		service.Hostname() != "control."+domain {
		return errors.New("runtime outputs require an exact HTTPS Control-Service origin for control.<domain>")
	}
	ip := net.ParseIP(config.BindIP)
	if ip == nil || ip.To4() == nil || ip.String() != config.BindIP ||
		!ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
		return errors.New("runtime outputs require an explicit canonical private IPv4 bind address")
	}
	if config.OperatorIssuer != providers.OIDCOrigin {
		return errors.New("runtime operator issuer must exactly match the explicitly configured OIDC issuer origin")
	}
	if !config.OperatorIdentityVerified {
		return errors.New("runtime outputs require explicit confirmation of a successful named IdP login")
	}
	if config.OperatorSubject == "" || len(config.OperatorSubject) > 256 ||
		strings.TrimSpace(config.OperatorSubject) != config.OperatorSubject {
		return errors.New("runtime outputs require the exact nonempty operator subject from a successful named IdP login")
	}
	for _, char := range config.OperatorSubject {
		if char < 32 || char == 127 {
			return errors.New("runtime operator subject must not contain control characters")
		}
	}
	return nil
}

func renderRuntimeOutputs(
	output, domain string,
	now time.Time,
	providers providerConfig,
	input runtimeOutputConfig,
	identityFiles map[string][]byte,
	files map[string][]byte,
) error {
	ca, err := x509.ParseCertificate(firstPEMCertificate(identityFiles["lab-ca.pem"]))
	if err != nil || ca == nil || !ca.IsCA {
		return errors.New("runtime outputs require the generated lab CA")
	}
	caKey, err := parseCAKey(identityFiles["lab-ca-key.pem"])
	if err != nil {
		return errors.New("runtime outputs require the generated lab CA private key")
	}
	serviceURL, _ := url.Parse(input.ControlServiceOrigin)
	serviceHost := serviceURL.Hostname()
	servicePort := serviceURL.Port()
	if servicePort == "" {
		servicePort = "443"
	}

	controlClientCert, _, controlClientPEM, controlClientKeyPEM, err := certificateWithUsage(
		"Control Room budget client", "room."+domain, now, ca, caKey, x509.ExtKeyUsageClientAuth,
	)
	if err != nil {
		return fmt.Errorf("cannot create Control Room client identity: %w", err)
	}
	_, _, simulatorClientPEM, simulatorClientKeyPEM, err := certificateWithUsage(
		"Simulator budget client", "simulator."+domain, now, ca, caKey, x509.ExtKeyUsageClientAuth,
	)
	if err != nil {
		return fmt.Errorf("cannot create Simulator client identity: %w", err)
	}
	_, _, serviceCertPEM, serviceKeyPEM, err := certificateWithUsage(
		"ZPR Control-Service", serviceHost, now, ca, caKey, x509.ExtKeyUsageServerAuth,
	)
	if err != nil {
		return fmt.Errorf("cannot create Control-Service server identity: %w", err)
	}
	controlClientChain := append(append([]byte(nil), controlClientPEM...), identityFiles["lab-ca.pem"]...)
	simulatorClientChain := append(append([]byte(nil), simulatorClientPEM...), identityFiles["lab-ca.pem"]...)
	serviceChain := append(append([]byte(nil), serviceCertPEM...), identityFiles["lab-ca.pem"]...)
	if _, err := tls.X509KeyPair(controlClientChain, controlClientKeyPEM); err != nil {
		return errors.New("generated Control Room client certificate and key do not match")
	}
	if _, err := tls.X509KeyPair(simulatorClientChain, simulatorClientKeyPEM); err != nil {
		return errors.New("generated Simulator client certificate and key do not match")
	}
	if err := systemconfig.CheckServerCertificate(input.ControlServiceOrigin, serviceChain, serviceKeyPEM, now); err != nil {
		return fmt.Errorf("generated Control-Service identity is invalid: %w", err)
	}

	signerPublic, signerPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return errors.New("cannot generate Control Room budget delegation signer")
	}
	signerPrivateDER, err := x509.MarshalPKCS8PrivateKey(signerPrivate)
	if err != nil {
		clear(signerPrivate)
		return errors.New("cannot encode Control Room budget delegation signer")
	}
	clear(signerPrivate)
	signerKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: signerPrivateDER})
	clear(signerPrivateDER)
	signerKeyID := "control-room-budget-v1"
	signerConfig := operatordelegation.SignerConfig{
		Version: 1, Audience: input.ControlServiceOrigin, KeyID: signerKeyID,
	}
	if err := signerConfig.Validate(); err != nil {
		return fmt.Errorf("invalid generated delegation signer configuration: %w", err)
	}
	certificatePin := sha256.Sum256(controlClientCert.Raw)
	grant := operatorauth.Grant{
		Issuer: input.OperatorIssuer, Subject: input.OperatorSubject,
		Organizations: []string{input.OrganizationID},
		Permissions:   []string{"monitor.read", "budget.approve"},
	}
	delegationTrust := operatordelegation.Config{
		Version: 1, Audience: input.ControlServiceOrigin,
		Keys: []operatordelegation.TrustedKey{{
			KeyID: signerKeyID, PublicKey: base64.RawURLEncoding.EncodeToString(signerPublic),
			CertificateSHA256: hex.EncodeToString(certificatePin[:]),
		}},
		Grants: []operatorauth.Grant{grant},
	}
	if err := delegationTrust.Validate(); err != nil {
		return fmt.Errorf("invalid generated Control-Service delegation trust: %w", err)
	}
	oidcConfig := operatorauth.Config{
		Version: 1, Issuer: input.OperatorIssuer, ClientID: "zpr-control-room",
		RedirectURL:            providers.RoomOrigin + "/auth/operator/callback",
		SessionLifetimeSeconds: 3600, Grants: []operatorauth.Grant{grant},
	}
	system := systemconfig.Config{
		SchemaVersion: 2,
		Organization:  systemconfig.Organization{ID: input.OrganizationID, Name: input.OrganizationName},
		ControlRoom: systemconfig.ControlRoom{
			Origin: providers.RoomOrigin, ManagementIP: input.BindIP,
			CertificateFile: runtimePath(output, "room-chain.pem"),
			KeyFile:         runtimePath(output, "room-key.pem"),
		},
		ControlService: &systemconfig.ControlService{
			URL:                   input.ControlServiceOrigin,
			ClientCertificateFile: runtimePath(output, "control-room-client-chain.pem"),
			ClientKeyFile:         runtimePath(output, "control-room-client-key.pem"),
			CAFile:                runtimePath(output, "control-service-client-ca.pem"),
			TLSServerName:         serviceHost,
		},
		BudgetAuthority: &systemconfig.BudgetAuthority{
			DatabaseFile:              runtimePath(output, "budget-state", "authority.sqlite"),
			DelegationTrustFile:       runtimePath(output, "delegation-trust.json"),
			ServiceListenAddress:      net.JoinHostPort(input.BindIP, servicePort),
			ServiceCertificateFile:    runtimePath(output, "control-service-chain.pem"),
			ServiceKeyFile:            runtimePath(output, "control-service-key.pem"),
			ServiceClientCAFile:       runtimePath(output, "budget-authority-client-ca.pem"),
			ControlRoomSignerFile:     runtimePath(output, "control-room-signer.json"),
			ControlRoomSignerKeyFile:  runtimePath(output, "control-room-signer-key.pem"),
			ControlRoomSignerAudience: input.ControlServiceOrigin,
		},
		OIDC: systemconfig.OIDC{
			Config: oidcConfig, ClientSecretFile: runtimePath(output, "oidc-room-client.secret"),
			CAFile: runtimePath(output, "control-room-oidc-ca.pem"),
		},
	}
	if err := system.Validate(); err != nil {
		return fmt.Errorf("generated schema-v2 runtime configuration is invalid: %w", err)
	}
	simulatorConfig := simulatorbudgetconfig.Config{
		SchemaVersion: 1, URL: input.ControlServiceOrigin,
		ClientCertificateFile: runtimePath(output, "simulator-operator-auth", "budget-client.crt"),
		ClientKeyFile:         runtimePath(output, "simulator-operator-auth", "budget-client.key"),
		CAFile:                runtimePath(output, "simulator-operator-auth", "budget-ca.crt"),
	}
	if err := simulatorConfig.Validate(); err != nil {
		return fmt.Errorf("generated Simulator budget client configuration is invalid: %w", err)
	}

	companyJSON, err := json.MarshalIndent(system, "", "  ")
	if err != nil {
		return errors.New("cannot encode schema-v2 runtime configuration")
	}
	trustJSON, err := json.MarshalIndent(delegationTrust, "", "  ")
	if err != nil {
		return errors.New("cannot encode Control-Service delegation trust")
	}
	signerJSON, err := json.MarshalIndent(signerConfig, "", "  ")
	if err != nil {
		return errors.New("cannot encode Control Room delegation signer configuration")
	}
	simulatorJSON, err := json.MarshalIndent(simulatorConfig, "", "  ")
	if err != nil {
		return errors.New("cannot encode Simulator budget client configuration")
	}
	simulatorGrant := operatorauth.Grant{
		Issuer: input.OperatorIssuer, Subject: input.OperatorSubject,
		Organizations: []string{input.OrganizationID},
		Permissions:   []string{"organization.read", "scenario.read", "scenario.run"},
	}
	simulatorOIDCConfig := operatorauth.Config{
		Version: 1, Issuer: input.OperatorIssuer, ClientID: "zpr-simulator",
		RedirectURL:            providers.SimulatorOrigin + "/auth/operator/callback",
		SessionLifetimeSeconds: 3600, Grants: []operatorauth.Grant{simulatorGrant},
	}
	if err := simulatorOIDCConfig.Validate(); err != nil {
		return fmt.Errorf("generated Simulator operator configuration is invalid: %w", err)
	}
	simulatorOIDC, err := json.MarshalIndent(simulatorOIDCConfig, "", "  ")
	if err != nil {
		return errors.New("cannot encode Simulator OIDC callback configuration")
	}
	files["company-config.json"] = append(companyJSON, '\n')
	files["delegation-trust.json"] = append(trustJSON, '\n')
	files["control-room-signer.json"] = append(signerJSON, '\n')
	files["control-room-signer-key.pem"] = signerKeyPEM
	files["control-room-client-chain.pem"] = controlClientChain
	files["control-room-client-key.pem"] = controlClientKeyPEM
	files["control-service-chain.pem"] = serviceChain
	files["control-service-key.pem"] = serviceKeyPEM
	files["control-service-client-ca.pem"] = append([]byte(nil), identityFiles["lab-ca.pem"]...)
	files["budget-authority-client-ca.pem"] = append([]byte(nil), identityFiles["lab-ca.pem"]...)
	files["control-room-oidc-ca.pem"] = append([]byte(nil), identityFiles["lab-ca.pem"]...)
	files["simulator-budget-client.json"] = append(simulatorJSON, '\n')
	files["room-chain.pem"] = append([]byte(nil), identityFiles["room-chain.pem"]...)
	files["room-key.pem"] = append([]byte(nil), identityFiles["room-key.pem"]...)
	files["oidc-room-client.secret"] = append([]byte(nil), identityFiles["oidc-room-client.secret"]...)
	files["simulator-operator-auth/simulator.crt"] = append([]byte(nil), identityFiles["simulator-chain.pem"]...)
	files["simulator-operator-auth/simulator.key"] = append([]byte(nil), identityFiles["simulator-key.pem"]...)
	files["simulator-operator-auth/oidc.json"] = append(simulatorOIDC, '\n')
	files["simulator-operator-auth/client.secret"] = append([]byte(nil), identityFiles["oidc-simulator-client.secret"]...)
	files["simulator-operator-auth/oidc-ca.crt"] = append([]byte(nil), identityFiles["lab-ca.pem"]...)
	files["simulator-operator-auth/budget-client.crt"] = simulatorClientChain
	files["simulator-operator-auth/budget-client.key"] = simulatorClientKeyPEM
	files["simulator-operator-auth/budget-ca.crt"] = append([]byte(nil), identityFiles["lab-ca.pem"]...)
	if loopbackCA := identityFiles["simulator-loopback-ca.pem"]; len(loopbackCA) != 0 {
		files["simulator-loopback-ca.pem"] = append([]byte(nil), loopbackCA...)
	}
	if dexOverride := identityFiles["dex-loopback-override.yaml"]; len(dexOverride) != 0 {
		files["identity-overlay/dex.yaml"] = append([]byte(nil), dexOverride...)
		composeOverride, err := json.MarshalIndent(map[string]any{
			"services": map[string]any{
				"oidc": map[string]any{
					"volumes": []map[string]any{{
						"type": "bind", "source": runtimePath(output, "identity-overlay", "dex.yaml"),
						"target": "/etc/zpr/lab/dex.yaml", "read_only": true,
						"bind": map[string]bool{"create_host_path": false},
					}},
				},
			},
		}, "", "  ")
		if err != nil {
			return errors.New("cannot encode Dex loopback Compose override")
		}
		files["identity-overlay/compose.override.json"] = append(composeOverride, '\n')
	}

	var identityInputs map[string]any
	if err := json.Unmarshal(identityFiles["identity-inputs.json"], &identityInputs); err != nil {
		return errors.New("cannot update identity handoff for explicit runtime grants")
	}
	clients, ok := identityInputs["clients"].(map[string]any)
	if !ok {
		return errors.New("identity handoff lacks OIDC clients")
	}
	simulatorClient, ok := clients["simulator"].(map[string]any)
	if !ok {
		return errors.New("identity handoff lacks the Simulator OIDC client")
	}
	simulatorClient["redirect_url"] = providers.SimulatorOrigin + "/auth/operator/callback"
	identityInputs["grant_status"] = "caller-confirmed issuer/subject configured with explicit organization and permission scopes; offline generator did not contact the provider"
	identityInputs["simulator_operator_permissions"] = []string{"organization.read", "scenario.read", "scenario.run"}
	identityInputs["runtime_acceptance_todo"] = "Before use, verify the configured issuer and subject against a successful login to the named OIDC provider; this offline generator does not contact or verify the provider."
	handoffJSON, err := json.MarshalIndent(identityInputs, "", "  ")
	if err != nil {
		return errors.New("cannot encode identity handoff")
	}
	files["identity-inputs.json"] = append(handoffJSON, '\n')
	return nil
}

func runtimePath(output string, elements ...string) string {
	return filepath.Join(append([]string{output}, elements...)...)
}

func firstPEMCertificate(data []byte) []byte {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil
	}
	return block.Bytes
}

func parseCAKey(data []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PRIVATE KEY" {
		return nil, errors.New("CA private key PEM is invalid")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	result, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("CA private key is not ECDSA")
	}
	return result, nil
}
