package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

type identity struct {
	Hostname        string `json:"hostname"`
	CertificateFile string `json:"certificate_file"`
	KeyFile         string `json:"key_file"`
}

type inventory struct {
	SchemaVersion    int                 `json:"schema_version"`
	Kind             string              `json:"kind"`
	Domain           string              `json:"domain"`
	CreatedAt        time.Time           `json:"created_at"`
	LeafExpiresAt    time.Time           `json:"leaf_expires_at"`
	CAFile           string              `json:"ca_file"`
	CAKeyFile        string              `json:"ca_key_file"`
	Identities       map[string]identity `json:"identities"`
	Secrets          map[string]string   `json:"secret_files"`
	DNS              *dnsConfig          `json:"dns,omitempty"`
	IdentityServices *providerConfig     `json:"identity_services,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "-evaluation" {
		return runEvaluation(args[1:], stdout, stderr)
	}
	flags := flag.NewFlagSet("zpr-lab-bootstrap", flag.ContinueOnError)
	flags.SetOutput(stderr)
	domain := flags.String("domain", "", "explicit lab DNS suffix (required; no DNS changes)")
	output := flags.String("output", "", "new absolute directory under an existing protected parent (required)")
	bootstrapIP := flags.String("bootstrap-ip", "", "private companion IPv4 for LDAP, OIDC and Simulator records (requires room-ip; does not enable a DNS server)")
	roomIP := flags.String("room-ip", "", "private IPv4 for the real package's Control Room")
	hostBootstrapDNS := flags.Bool("host-bootstrap-dns", false, "generate optional companion BIND bootstrap server files instead of using existing network DNS (requires bootstrap-ip and room-ip)")
	zprDNSIP := flags.String("zpr-dns-ip", "", "ULA IPv6 for BIND behind the ZPR DNS adapter")
	zprClients := flags.String("zpr-client-prefix", "", "canonical ULA IPv6 prefix allowed to query ZPR DNS")
	identityServices := flags.Bool("identity-services", false, "also render lab OpenLDAP/Dex configuration (requires three explicit HTTPS origins)")
	roomOrigin := flags.String("room-origin", "", "Control Room HTTPS origin for identity setup")
	simulatorOrigin := flags.String("simulator-origin", "", "Simulator HTTPS origin for identity setup")
	oidcOrigin := flags.String("oidc-origin", "", "Dex HTTPS issuer origin for identity setup")
	runtimeOutputs := flags.Bool("runtime-outputs", false, "generate final protected runtime configuration from an existing identity bootstrap; Linux only")
	identityInputDir := flags.String("identity-input-dir", "", "existing protected identity bootstrap output created and initialized on Ubuntu")
	controlServiceOrigin := flags.String("control-service-origin", "", "exact Control-Service HTTPS origin for runtime outputs")
	runtimeBindIP := flags.String("runtime-bind-ip", "", "private IPv4 address for Control Room and Control-Service runtime bindings")
	organizationID := flags.String("organization-id", "", "explicit organization ID for runtime operator grants")
	organizationName := flags.String("organization-name", "", "explicit organization name for runtime configuration")
	operatorIssuer := flags.String("operator-issuer", "", "exact verified issuer from a successful named IdP login")
	operatorSubject := flags.String("operator-subject", "", "exact verified subject from a successful named IdP login")
	operatorIdentityVerified := flags.Bool("operator-identity-verified", false, "confirm issuer and subject were verified from a successful named IdP login")
	identityCompose := flags.Bool("identity-compose", false, "also generate independent LDAP/Dex Compose and private state directories (requires identity-services, bind-ip and pinned images)")
	bindIP := flags.String("bind-ip", "", "explicit private IPv4 for LDAP/Dex host ports")
	ldapImage := flags.String("ldap-image", "", "immutable local image ID or repository@sha256 digest for demo-only, unqualified OpenLDAP")
	dexImage := flags.String("dex-image", "", "immutable local image ID or repository@sha256 digest for Dex")
	platform := flags.String("identity-platform", "linux/amd64", "identity container platform; linux/arm64 is engineering validation only")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *domain == "" || *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -domain lab.example.test -output /protected/parent/new-lab [DNS options; see -h]")
		return 2
	}
	if *runtimeOutputs {
		if *identityInputDir == "" || *identityServices || *identityCompose || *bootstrapIP != "" ||
			*roomIP != "" || *zprDNSIP != "" || *zprClients != "" || *hostBootstrapDNS ||
			*bindIP != "" || *ldapImage != "" || *dexImage != "" || *platform != "linux/amd64" {
			fmt.Fprintln(stderr, "zpr-lab-bootstrap: runtime-outputs requires an existing identity-input-dir and cannot regenerate or initialize identity services")
			return 2
		}
		providers := &providerConfig{RoomOrigin: *roomOrigin, SimulatorOrigin: *simulatorOrigin, OIDCOrigin: *oidcOrigin}
		runtimeConfig := &runtimeOutputConfig{
			ControlServiceOrigin:     *controlServiceOrigin,
			BindIP:                   *runtimeBindIP,
			OrganizationID:           *organizationID,
			OrganizationName:         *organizationName,
			OperatorIssuer:           *operatorIssuer,
			OperatorSubject:          *operatorSubject,
			OperatorIdentityVerified: *operatorIdentityVerified,
		}
		if err := runtimeOutputsConfigured(*identityInputDir, *output, *domain, time.Now().UTC(), *providers, *runtimeConfig); err != nil {
			fmt.Fprintf(stderr, "zpr-lab-bootstrap: %v\n", err)
			return 1
		}
		if _, err := fmt.Fprintln(stdout, "Protected runtime configuration created from the existing Ubuntu identity bootstrap. The identity source and initialized LDAP/Dex state were not changed.\nThe caller-confirmed issuer/subject are embedded in scoped grants; the offline generator did not contact or verify the IdP. No IdP, Docker or Compose action was performed."); err != nil {
			fmt.Fprintln(stderr, "zpr-lab-bootstrap: cannot write result; files may already be complete, inspect the output directory before retrying")
			return 1
		}
		return 0
	}
	if *identityInputDir != "" || *controlServiceOrigin != "" || *runtimeBindIP != "" ||
		*organizationID != "" || *organizationName != "" || *operatorIssuer != "" || *operatorSubject != "" || *operatorIdentityVerified {
		fmt.Fprintln(stderr, "zpr-lab-bootstrap: runtime configuration inputs require explicit -runtime-outputs")
		return 2
	}
	var dns *dnsConfig
	if *bootstrapIP != "" || *roomIP != "" || *zprDNSIP != "" || *zprClients != "" || *hostBootstrapDNS {
		dns = &dnsConfig{BootstrapIP: *bootstrapIP, RoomIP: *roomIP, ZPRDNSIP: *zprDNSIP, ZPRClientPrefix: *zprClients, HostBootstrapDNS: *hostBootstrapDNS}
	}
	var providers *providerConfig
	var deployment *identityStackConfig
	if *identityCompose {
		if !*identityServices {
			fmt.Fprintln(stderr, "zpr-lab-bootstrap: identity-compose requires identity-services")
			return 2
		}
		deployment = &identityStackConfig{BindIP: *bindIP, LDAPImage: *ldapImage, DexImage: *dexImage,
			Platform: *platform, UID: os.Geteuid(), GID: os.Getegid()}
	} else if *bindIP != "" || *ldapImage != "" || *dexImage != "" || *platform != "linux/amd64" {
		fmt.Fprintln(stderr, "zpr-lab-bootstrap: identity deployment settings require explicit -identity-compose")
		return 2
	}
	if *identityServices {
		providers = &providerConfig{RoomOrigin: *roomOrigin, SimulatorOrigin: *simulatorOrigin, OIDCOrigin: *oidcOrigin, Deployment: deployment}
	} else if *roomOrigin != "" || *simulatorOrigin != "" || *oidcOrigin != "" {
		fmt.Fprintln(stderr, "zpr-lab-bootstrap: identity origins require explicit -identity-services")
		return 2
	}
	if err := bootstrapConfigured(*domain, *output, time.Now().UTC(), dns, providers); err != nil {
		fmt.Fprintf(stderr, "zpr-lab-bootstrap: %v\n", err)
		return 1
	}
	message := "Lab-only TLS identities and service secrets created; lab-bootstrap.json is the completion inventory.\nExisting network DNS is the default; underlay record requests and protected ZPR DNS files are generated independently when selected. Companion bootstrap BIND requires explicit -host-bootstrap-dns.\nWith -identity-services, protected OpenLDAP/Dex configuration and initial directory data are generated, not applied. Named operator grants still require verified issuer/subject identities.\nWith -identity-compose, pinned identity services and private state directories are generated for separate initialization/startup; nothing is launched."
	if providers != nil && providers.Deployment != nil {
		message += "\nGenerate identity-compose service keys on Ubuntu as the intended service-state owner. OpenLDAP remains demo-only and unqualified; image digest syntax is checked, but local image identity/platform are not inspected."
	}
	message += "\nNo services, DNS records, users, OIDC clients or system trust were configured. This is NOT an installable companion package.\nExisting directories are never reused or overwritten. Keep the CA private key offline; distribute only the public CA to explicitly authorized lab clients."
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		fmt.Fprintln(stderr, "zpr-lab-bootstrap: cannot write result; files may already be complete, inspect the output directory before retrying")
		return 1
	}
	return 0
}

func bootstrap(domain, output string, now time.Time) error {
	return bootstrapConfigured(domain, output, now, nil, nil)
}

func bootstrapConfigured(domain, output string, now time.Time, dns *dnsConfig, providers *providerConfig) error {
	origin := "https://room." + domain
	address, err := url.Parse(origin)
	if err != nil || address.Hostname() != "room."+domain ||
		systemconfig.ValidateManagementEndpoint(origin, "") != nil {
		return errors.New("domain must be a lowercase DNS suffix without a port, path or localhost/.local name")
	}
	if !filepath.IsAbs(output) || filepath.Clean(output) != output || output == string(filepath.Separator) {
		return errors.New("output must be a canonical absolute path to a new directory")
	}
	if providers != nil && providers.Deployment != nil && dns != nil &&
		dns.BootstrapIP != "" && dns.BootstrapIP != providers.Deployment.BindIP {
		return errors.New("companion DNS record address must match the identity Compose bind-ip")
	}
	files, err := generateConfigured(domain, now, dns, providers)
	if err != nil {
		return fmt.Errorf("cannot generate lab files; no files were written: %w", err)
	}
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return errors.New("cannot open output parent; create a protected parent directory first")
	}
	defer parent.Close()
	info, err := parent.Stat(".")
	if err != nil || info.Mode().Perm()&0o022 != 0 {
		return errors.New("output parent must not be group- or other-writable")
	}
	name := filepath.Base(output)
	if err := parent.Mkdir(name, 0o700); err != nil {
		return errors.New("cannot create output directory; existing directories, files and symlinks are never reused or overwritten")
	}
	root, err := parent.OpenRoot(name)
	if err != nil {
		return errors.New("cannot open new output directory; partial output may remain and must be inspected")
	}
	defer root.Close()
	if providers != nil && providers.Deployment != nil {
		for _, directory := range []string{"ldap-state", "dex-state"} {
			if err := root.Mkdir(directory, 0o700); err != nil {
				return errors.New("cannot create private identity state directories; inspect partial output before retrying")
			}
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "lab-bootstrap.json" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	// The inventory is written last so partial output cannot look complete.
	names = append(names, "lab-bootstrap.json")
	for _, name := range names {
		if err := writePrivate(root, name, files[name]); err != nil {
			return fmt.Errorf("cannot persist %s; output is incomplete, inspect it before retrying (no automatic deletion)", name)
		}
	}
	return nil
}

func writePrivate(root *os.Root, name string, data []byte) error {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func generate(domain string, now time.Time) (map[string][]byte, error) {
	return generateConfigured(domain, now, nil, nil)
}

func generateConfigured(domain string, now time.Time, dns *dnsConfig, providers *providerConfig) (map[string][]byte, error) {
	if providers != nil {
		if err := providers.validate(domain); err != nil {
			return nil, err
		}
	}
	if dns != nil {
		if err := dns.validate(); err != nil {
			return nil, err
		}
	}
	now = now.UTC().Truncate(time.Second)
	ca, caKey, caPEM, caKeyPEM, err := certificate("ZPR simulator lab CA", domain, now, nil, nil)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{"lab-ca.pem": caPEM, "lab-ca-key.pem": caKeyPEM}
	record := inventory{
		SchemaVersion: 1, Kind: "simulator-lab-trust", Domain: domain,
		CreatedAt: now, LeafExpiresAt: now.Add(90 * 24 * time.Hour),
		CAFile: "lab-ca.pem", CAKeyFile: "lab-ca-key.pem",
		Identities: map[string]identity{}, Secrets: map[string]string{},
		DNS:              dns,
		IdentityServices: providers,
	}
	for _, service := range []string{"room", "simulator", "ldap", "oidc"} {
		host := service + "." + domain
		if service == "simulator" && providers != nil {
			simulatorOrigin, _ := url.Parse(providers.SimulatorOrigin)
			host = simulatorOrigin.Hostname()
		}
		identityCA, identityCAKey, identityCAPEM := ca, caKey, caPEM
		if service == "simulator" && host == "localhost" {
			var loopbackCAKeyPEM []byte
			identityCA, identityCAKey, identityCAPEM, loopbackCAKeyPEM, err =
				certificate("ZPR Simulator loopback CA", "localhost", now, nil, nil)
			if err != nil {
				return nil, err
			}
			clear(loopbackCAKeyPEM)
			files["simulator-loopback-ca.pem"] = append([]byte(nil), identityCAPEM...)
		}
		_, _, certPEM, keyPEM, err := certificate(host, host, now, identityCA, identityCAKey)
		if service == "simulator" && host == "localhost" {
			identityCAKey.D.SetInt64(0)
		}
		if err != nil {
			return nil, err
		}
		certFile, keyFile := service+"-chain.pem", service+"-key.pem"
		files[certFile] = append(certPEM, identityCAPEM...)
		files[keyFile] = keyPEM
		record.Identities[service] = identity{Hostname: host, CertificateFile: certFile, KeyFile: keyFile}
	}
	secrets := []string{"ldap-admin", "ldap-reader", "oidc-room-client", "oidc-simulator-client"}
	if providers != nil {
		secrets = append(secrets, "lab-operator", "lab-observer")
	}
	for _, secret := range secrets {
		value := make([]byte, 32)
		if _, err := rand.Read(value); err != nil {
			return nil, err
		}
		filename := secret + ".secret"
		files[filename] = []byte(base64.RawURLEncoding.EncodeToString(value) + "\n")
		record.Secrets[secret] = filename
	}
	if providers != nil {
		if err := providers.render(domain, files); err != nil {
			return nil, err
		}
	}
	if dns != nil {
		dnsFiles, err := dns.render(domain)
		if err != nil {
			return nil, err
		}
		for name, data := range dnsFiles {
			files[name] = data
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	files["lab-bootstrap.json"] = append(data, '\n')
	return files, nil
}

func certificate(commonName, hostname string, now time.Time, parent *x509.Certificate, signer *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte, error) {
	return certificateWithUsage(commonName, hostname, now, parent, signer, x509.ExtKeyUsageServerAuth)
}

func certificateWithUsage(commonName, hostname string, now time.Time, parent *x509.Certificate, signer *ecdsa.PrivateKey, usage x509.ExtKeyUsage) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte, error) {
	return certificateWithOrganization(commonName, hostname, "ZPR simulator lab only", now, parent, signer, usage)
}

func certificateWithOrganization(commonName, hostname, organization string, now time.Time, parent *x509.Certificate, signer *ecdsa.PrivateKey, usage x509.ExtKeyUsage, addresses ...net.IP) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, nil, nil, err
	}
	serial.Add(serial, big.NewInt(1))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName, Organization: []string{organization}},
		NotBefore:    now.Add(-5 * time.Minute), NotAfter: now.Add(90 * 24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, BasicConstraintsValid: true,
	}
	if parent == nil {
		template.IsCA = true
		template.MaxPathLenZero = true
		template.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
		template.NotAfter = now.Add(365 * 24 * time.Hour)
		// Limit DNS trust to this installation's lab suffix or exact loopback host.
		template.PermittedDNSDomainsCritical = true
		for _, address := range addresses {
			template.PermittedIPRanges = append(template.PermittedIPRanges, &net.IPNet{IP: address.To4(), Mask: net.CIDRMask(32, 32)})
		}
		if hostname == "localhost" {
			template.PermittedDNSDomains = []string{hostname}
		} else {
			template.PermittedDNSDomains = []string{"." + hostname}
		}
		parent, signer = template, key
	} else {
		template.DNSNames = []string{hostname}
		template.IPAddresses = addresses
		template.ExtKeyUsage = []x509.ExtKeyUsage{usage}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return parsed, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), nil
}
