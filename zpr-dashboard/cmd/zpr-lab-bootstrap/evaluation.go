package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"neboagency.com/zpr-dashborad/internal/controlservice/adapterlogs"
)

const evaluationBanner = "EVALUATION ONLY — NOT PRODUCTION SUPPORTED. Local administrators only; no Simulator dependency."
const evaluationLDAPImage = "osixia/openldap@sha256:0de3850635911109f0daf8f5957808ec4f4eb827fbaaa339100b5f1bab925745"
const evaluationDexImage = "ghcr.io/dexidp/dex@sha256:5d0656fce7d453c0e3b2706abf40c0d0ce5b371fb0b73b3cf714d05f35fa5f86"
const evaluationLogImage = "public.ecr.aws/zinclabs/openobserve@sha256:d4a878fac1f6c56003764f7f2a1625668917388f167e222c8c810de3f54c56ba"

type evaluationConfig struct {
	BrowserIP     string `json:"browser_ip,omitempty"`
	DexImage      string `json:"oidc_image"`
	DexPlatform   string `json:"oidc_platform"`
	OIDCPort      int    `json:"oidc_port"`
	SimulatorPort int    `json:"simulator_port,omitempty"`
	ScenarioDemo  bool   `json:"scenario_demo,omitempty"`
	LDAPImage     string `json:"ldap_image"`
	LogImage      string `json:"logging_image"`
	LogPlatform   string `json:"logging_platform"`
	RoomPort      int    `json:"room_port"`
	ServicePort   int    `json:"service_port"`
	LDAPPort      int    `json:"ldap_port"`
	LogPort       int    `json:"logging_port"`
	// Optional loopback Visa Service; zero/false values keep earlier inventories valid.
	VisaService bool `json:"visa_service,omitempty"`
	VSAdminPort int  `json:"vs_admin_port,omitempty"`
	VSAPIPort   int  `json:"vs_api_port,omitempty"`
	ValkeyPort  int  `json:"valkey_port,omitempty"`
	// Control-Service pinned snapshot delivery listener for the loopback VS.
	SnapshotDeliveryPort int `json:"snapshot_delivery_port,omitempty"`
	// Loopback port that relays the protected DNS zone transfer (TSIG viewer
	// key only) to Control-Service for the DNS records page; zero when absent.
	DNSTransferPort int `json:"dns_transfer_port,omitempty"`
	// DeliveryHost is the data-plane Docker network gateway on Linux, where
	// containers cannot reach a host loopback listener; empty means loopback.
	DeliveryHost string `json:"-"`
	// DataPlane hosts the VS, Valkey, a node and the vs.zpr adapter in a Linux
	// container instead of the native loopback VS (VSAPI/Valkey stay internal).
	DataPlane bool `json:"data_plane,omitempty"`
	// ProtectedDNS runs an authoritative-only BIND service behind a ZPR adapter.
	ProtectedDNS bool `json:"protected_dns,omitempty"`
	// DataPlaneEndpoints admits endpoint-01..N by per-install bootstrap keys
	// (shortcut X15); the Simulator runs them from data-plane-endpoints.json.
	DataPlaneEndpoints int `json:"data_plane_endpoints,omitempty"`
	// NodeLocation is operator configuration for the data-plane node's map
	// position, written to vs.toml as [nodes."node"] latitude/longitude.
	NodeLocation *evaluationNodeLocation `json:"node_location,omitempty"`
	// DataPlaneNodes are additional nodes, each in its own container on the
	// install's data-plane network, fully meshed with the docking node.
	DataPlaneNodes []evaluationDataPlaneNode `json:"data_plane_nodes,omitempty"`
	// Directory replaces the starter directory with operator LDIF under the
	// operator's base DN; nil keeps dc=evaluation,dc=zpr.
	Directory *evaluationDirectory `json:"directory,omitempty"`
}

type evaluationNodeLocation struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func (location *evaluationNodeLocation) String() string {
	if location == nil {
		return ""
	}
	return strconv.FormatFloat(location.Latitude, 'f', -1, 64) + "," + strconv.FormatFloat(location.Longitude, 'f', -1, 64)
}

func parseEvaluationNodeLocation(value string) (*evaluationNodeLocation, error) {
	latitude, longitude, ok := strings.Cut(value, ",")
	if !ok {
		return nil, errors.New("-node-location must be LATITUDE,LONGITUDE")
	}
	lat, latErr := strconv.ParseFloat(strings.TrimSpace(latitude), 64)
	lon, lonErr := strconv.ParseFloat(strings.TrimSpace(longitude), 64)
	if latErr != nil || lonErr != nil || math.IsNaN(lat) || math.IsNaN(lon) || math.Abs(lat) > 90 || math.Abs(lon) > 180 {
		return nil, errors.New("-node-location must be finite LATITUDE,LONGITUDE within [-90,90],[-180,180]")
	}
	return &evaluationNodeLocation{Latitude: lat, Longitude: lon}, nil
}

// Runtime-only executables are not frozen: upgrading them keeps install identity.
type evaluationRuntimeOptions struct {
	ControlRoomBinary string
	VisaServiceBinary string
	ValkeyServer      string
	DataPlaneImage    string
}

type evaluationInventory struct {
	Version   int               `json:"version"`
	Kind      string            `json:"kind"`
	InstallID string            `json:"install_id"`
	Config    evaluationConfig  `json:"config"`
	Files     map[string]string `json:"files_sha256"`
}

func runEvaluation(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation", flag.ContinueOnError)
	flags.SetOutput(stderr)
	state := flags.String("state", "", "protected persistent install directory (required)")
	resetAdmin := flags.Bool("reset-admin", false, "EVALUATION ONLY: reset the bundled operator login while stopped")
	passwordFile := flags.String("password-file", "", "reset only: exclusively create a private 0600 password delivery file instead of terminal output")
	renderOnly := flags.Bool("render-only", false, "render/validate protected files only; no builds, Docker or services")
	config := evaluationConfig{}
	flags.StringVar(&config.BrowserIP, "browser-ip", "", "optional explicit private IPv4 address for browser HTTPS and OIDC; localhost redirects to this address")
	flags.StringVar(&config.LDAPImage, "ldap-image", evaluationLDAPImage, "current packaged OpenLDAP immutable image")
	flags.StringVar(&config.LogImage, "logging-image", evaluationLogImage, "current packaged OpenObserve immutable image")
	flags.StringVar(&config.LogPlatform, "logging-platform", "linux/arm64", "platform of the current packaged OpenObserve image (linux/amd64 requires a matching image override)")
	flags.StringVar(&config.DexImage, "oidc-image", evaluationDexImage, "current packaged Dex immutable image")
	flags.StringVar(&config.DexPlatform, "oidc-platform", "linux/arm64", "platform of the current packaged Dex image")
	flags.IntVar(&config.OIDCPort, "oidc-port", 15556, "named-operator OIDC HTTPS port")
	flags.IntVar(&config.SimulatorPort, "simulator-port", 0, "optional separate Simulator HTTPS callback port; registers a client only, never starts or requires Simulator")
	flags.BoolVar(&config.ScenarioDemo, "scenario-demo", false, "enable independent budget authority and the bounded paired endpoint scenario (requires Simulator callback, data plane, endpoints and delegation)")
	flags.IntVar(&config.RoomPort, "room-port", 18787, "Control Room HTTPS port")
	flags.IntVar(&config.ServicePort, "service-port", 18790, "loopback Control-Service mTLS port")
	flags.IntVar(&config.LDAPPort, "ldap-port", 11636, "loopback starter LDAPS port")
	flags.IntVar(&config.LogPort, "logging-port", 15080, "loopback OpenObserve HTTPS port")
	flags.BoolVar(&config.VisaService, "visa-service", false, "also run a loopback Visa Service (bundled evaluation policy, private Valkey); not enforcing")
	flags.IntVar(&config.VSAdminPort, "vs-admin-port", 18182, "with -visa-service: loopback Visa Service admin HTTPS port")
	flags.IntVar(&config.VSAPIPort, "vs-api-port", 15002, "with -visa-service: loopback Visa Service VSAPI port")
	flags.IntVar(&config.ValkeyPort, "valkey-port", 16379, "with -visa-service: loopback private Valkey port")
	flags.IntVar(&config.SnapshotDeliveryPort, "snapshot-delivery-port", 18793, "with -visa-service: loopback Control-Service subject snapshot delivery mTLS port; 0 disables the subject-link publication cycle (installs created before it existed)")
	flags.IntVar(&config.DNSTransferPort, "dns-transfer-port", 15353, "with -protected-dns: loopback port relaying the read-only DNS zone transfer used by the Control Room DNS records page")
	flags.BoolVar(&config.DataPlane, "data-plane", false, "with -visa-service: run the VS with Valkey, one node and the vs.zpr adapter in a privileged Linux data-plane container (build: packaging/evaluation/data-plane/build.sh)")
	flags.BoolVar(&config.ProtectedDNS, "protected-dns", false, "with -data-plane: run authoritative BIND behind an admitted ZPR DNS adapter and verify a protected TCP query")
	flags.IntVar(&config.DataPlaneEndpoints, "data-plane-endpoints", 0, fmt.Sprintf("with -data-plane: admit endpoint-01..N (0 or %d-%d) by per-install bootstrap keys for Simulator endpoints (EVALUATION ONLY shortcut X15)", evaluationDataPlaneMinEndpoints, evaluationDataPlaneMaxEndpoints))
	flags.Func("node-location", "with -data-plane: LATITUDE,LONGITUDE of the data-plane node for the Control Room map", func(value string) error {
		location, err := parseEvaluationNodeLocation(value)
		config.NodeLocation = location
		return err
	})
	flags.Func("data-plane-node", fmt.Sprintf("with -data-plane: NAME@LATITUDE,LONGITUDE of an additional node in its own container, linked to every other node (repeatable, up to %d)", evaluationDataPlaneMaxNodes), func(value string) error {
		node, err := parseEvaluationDataPlaneNode(value)
		if err == nil {
			config.DataPlaneNodes = append(config.DataPlaneNodes, node)
		}
		return err
	})
	directory := evaluationDirectory{}
	flags.StringVar(&directory.BaseDN, "directory-base-dn", "", "serve an operator directory under this base DN instead of the starter directory (requires -directory-ldif)")
	flags.Func("directory-ldif", "with -directory-base-dn: absolute path of an operator LDIF to import at creation (repeatable, in order); entries with userPassword are not imported", func(value string) error {
		directory.LDIFFiles = append(directory.LDIFFiles, value)
		return nil
	})
	runtime := evaluationRuntimeOptions{}
	flags.StringVar(&runtime.ControlRoomBinary, "control-room-binary", "", "absolute path of a prebuilt production Control Room/Control-Service executable; skips the source build")
	flags.StringVar(&runtime.DataPlaneImage, "data-plane-image", evaluationDataPlaneImage, "with -data-plane: local data-plane image")
	flags.StringVar(&runtime.VisaServiceBinary, "vs-binary", "", "with -visa-service: absolute path of a built vs executable (cargo build -p vs)")
	flags.StringVar(&runtime.ValkeyServer, "valkey-server", "valkey-server", "with -visa-service: valkey-server executable name or absolute path")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	visaOnly, nativeOnly, planeOnly := false, false, false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "vs-admin-port", "vs-api-port", "valkey-port", "snapshot-delivery-port", "vs-binary", "valkey-server", "data-plane", "data-plane-image", "data-plane-endpoints", "node-location", "data-plane-node":
			visaOnly = true
		}
		switch f.Name {
		case "vs-api-port", "valkey-port", "vs-binary", "valkey-server":
			nativeOnly = true
		case "data-plane-image", "data-plane-endpoints", "node-location", "data-plane-node":
			planeOnly = true
		}
	})
	if !config.VisaService {
		if visaOnly {
			fmt.Fprintln(stderr, "Visa Service port/executable flags require -visa-service")
			return 2
		}
		config.VSAdminPort, config.VSAPIPort, config.ValkeyPort, config.SnapshotDeliveryPort = 0, 0, 0, 0
	}
	if !config.ProtectedDNS {
		config.DNSTransferPort = 0
	}
	if config.DataPlane {
		if nativeOnly {
			fmt.Fprintln(stderr, "-data-plane keeps VSAPI and Valkey inside the container; -vs-api-port, -valkey-port, -vs-binary and -valkey-server do not apply")
			return 2
		}
		config.VSAPIPort, config.ValkeyPort = 0, 0
	} else if planeOnly {
		fmt.Fprintln(stderr, "-data-plane-image, -data-plane-endpoints, -node-location and -data-plane-node require -data-plane")
		return 2
	}
	if directory.BaseDN != "" || len(directory.LDIFFiles) != 0 {
		if directory.BaseDN == "" {
			fmt.Fprintln(stderr, "-directory-ldif requires -directory-base-dn")
			return 2
		}
		config.Directory = &directory
	}
	if *state == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation -state DIRECTORY [-visa-service -vs-binary FILE] [-render-only | -reset-admin [-password-file FILE]]")
		return 2
	}
	if *resetAdmin {
		invalid := false
		flags.Visit(func(f *flag.Flag) {
			if f.Name != "reset-admin" && f.Name != "state" && f.Name != "password-file" {
				invalid = true
			}
		})
		if invalid {
			fmt.Fprintln(stderr, "Reset accepts only -state and optional -password-file; stop the install first.")
			return 2
		}
		return runEvaluationAdminRecovery(*state, *passwordFile, stdout, stderr)
	}
	if *passwordFile != "" {
		fmt.Fprintln(stderr, "-password-file requires -reset-admin")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	absolute, err := filepath.Abs(*state)
	if err == nil {
		err = config.validate()
	}
	var inventory evaluationInventory
	if err == nil {
		inventory, err = prepareEvaluation(absolute, config, time.Now().UTC())
	}
	if err != nil {
		fmt.Fprintf(stderr, "Evaluation bootstrap: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Install %s: protected state validated; secrets are in private files, never printed.\n", inventory.InstallID)
	if *renderOnly {
		fmt.Fprintln(stdout, "RENDER ONLY: no services started; protocol readiness NOT verified.")
		return 0
	}
	if err := startEvaluation(absolute, inventory, runtime, stdout); err != nil {
		fmt.Fprintf(stderr, "Evaluation bootstrap: %v\n", err)
		return 1
	}
	return 0
}

func (config evaluationConfig) validate() error {
	if config.BrowserIP != "" {
		ip := net.ParseIP(config.BrowserIP)
		if ip == nil || ip.To4() == nil || !ip.IsPrivate() || ip.String() != config.BrowserIP {
			return errors.New("browser-ip must be an explicit canonical private IPv4 address, not a wildcard, loopback address or hostname")
		}
	}
	if !immutableImage.MatchString(config.DexImage) || !immutableImage.MatchString(config.LDAPImage) || !immutableImage.MatchString(config.LogImage) {
		return errors.New("evaluation images must be immutable image IDs or repository@sha256 digests")
	}
	if config.DexPlatform != "linux/amd64" && config.DexPlatform != "linux/arm64" {
		return errors.New("OIDC platform must be linux/amd64 or linux/arm64")
	}
	if config.LogPlatform != "linux/amd64" && config.LogPlatform != "linux/arm64" {
		return errors.New("logging platform must be linux/amd64 or linux/arm64")
	}
	ports := map[int]bool{}
	if !config.VisaService && (config.VSAdminPort != 0 || config.VSAPIPort != 0 || config.ValkeyPort != 0 || config.SnapshotDeliveryPort != 0) {
		return errors.New("Visa Service ports require -visa-service")
	}
	if config.DataPlane && (!config.VisaService || config.VSAPIPort != 0 || config.ValkeyPort != 0) {
		return errors.New("data plane requires -visa-service and keeps VSAPI/Valkey ports inside the container")
	}
	if config.ProtectedDNS && !config.DataPlane {
		return errors.New("protected DNS requires -data-plane")
	}
	if config.DataPlaneEndpoints != 0 && (!config.DataPlane || config.DataPlaneEndpoints < evaluationDataPlaneMinEndpoints || config.DataPlaneEndpoints > evaluationDataPlaneMaxEndpoints) {
		return fmt.Errorf("data-plane endpoints require -data-plane and must be 0 or %d-%d", evaluationDataPlaneMinEndpoints, evaluationDataPlaneMaxEndpoints)
	}
	if location := config.NodeLocation; location != nil {
		if !config.DataPlane {
			return errors.New("node location requires -data-plane")
		}
		if _, err := parseEvaluationNodeLocation(location.String()); err != nil {
			return err
		}
	}
	if err := validateEvaluationDataPlaneNodes(config); err != nil {
		return err
	}
	if err := config.Directory.validate(); err != nil {
		return err
	}
	for _, port := range config.ports() {
		if port < 1024 || port > 65535 || ports[port] {
			return errors.New("evaluation ports must be distinct unprivileged TCP ports")
		}
		ports[port] = true
	}
	if config.SimulatorPort != 0 && (config.SimulatorPort < 1024 || config.SimulatorPort > 65535 || ports[config.SimulatorPort]) {
		return errors.New("simulator-port must be a distinct unprivileged TCP port")
	}
	if config.ScenarioDemo && (config.SimulatorPort == 0 || !config.DataPlane || config.DataPlaneEndpoints < 2 || !config.subjectLinks() ||
		len(config.DataPlaneNodes) != 0 || config.Directory != nil) {
		return errors.New("scenario-demo requires simulator-port, single-node data-plane, at least two endpoints, starter directory and snapshot-delivery-port for named delegation")
	}
	return nil
}

func (config evaluationConfig) ports() []int {
	ports := []int{config.OIDCPort, config.RoomPort, config.ServicePort, config.LDAPPort, config.LogPort}
	if config.VisaService {
		ports = append(ports, config.VSAdminPort)
		if !config.DataPlane {
			ports = append(ports, config.VSAPIPort, config.ValkeyPort)
		}
	}
	if config.subjectLinks() {
		ports = append(ports, config.SnapshotDeliveryPort)
	}
	if config.ProtectedDNS && config.DNSTransferPort != 0 {
		ports = append(ports, config.DNSTransferPort)
	}
	return ports
}

// subjectLinks reports whether the subject-link review, signing and pinned
// delivery cycle to the loopback VS is configured.
func (config evaluationConfig) subjectLinks() bool {
	return config.VisaService && config.SnapshotDeliveryPort != 0
}

func (config evaluationConfig) browserHost() string {
	if config.BrowserIP != "" {
		return config.BrowserIP
	}
	return "localhost"
}

func (config evaluationConfig) browserOrigin(port int) string {
	return fmt.Sprintf("https://%s:%d", config.browserHost(), port)
}

func (config evaluationConfig) roomListen() string {
	host := config.BrowserIP
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, fmt.Sprint(config.RoomPort))
}

func prepareEvaluation(output string, config evaluationConfig, now time.Time) (evaluationInventory, error) {
	empty := evaluationInventory{}
	if err := config.validate(); err != nil {
		return empty, err
	}
	if output == string(filepath.Separator) || !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return empty, errors.New("state must be a canonical absolute non-root path")
	}
	parent, err := os.OpenRoot(filepath.Dir(output))
	if err != nil {
		return empty, errors.New("create an existing protected state parent first")
	}
	defer parent.Close()
	info, err := parent.Stat(".")
	if err != nil || info.Mode().Perm()&0o022 != 0 {
		return empty, errors.New("state parent must not be group/other writable")
	}
	if info, err := os.Lstat(output); err == nil {
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			return empty, errors.New("existing state must be a real private (0700) directory, not a symlink")
		}
		return readEvaluation(output, config)
	} else if !errors.Is(err, os.ErrNotExist) {
		return empty, errors.New("cannot inspect state directory")
	}
	files, inventory, err := generateEvaluation(output, config, now)
	if err != nil {
		return empty, err
	}
	if err := parent.Mkdir(filepath.Base(output), 0o700); err != nil {
		return empty, errors.New("cannot exclusively create state directory; inspect before retrying")
	}
	root, err := parent.OpenRoot(filepath.Base(output))
	if err != nil {
		return empty, err
	}
	defer root.Close()
	dirs := []string{"ldap-state", "logging-state", "oidc-state", "logs", "build"}
	if config.VisaService {
		dirs = append(dirs, "valkey-state", "vs-trusted-services")
	}
	if config.DataPlane {
		dirs = append(dirs, "data-plane")
		if config.ProtectedDNS {
			dirs = append(dirs, filepath.Join("data-plane", "dns"))
		}
	}
	for _, name := range dirs {
		if err := root.Mkdir(name, 0o700); err != nil {
			return empty, err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := writePrivate(root, name, files[name]); err != nil {
			return empty, errors.New("incomplete evaluation state; inspect it before retrying, never reseed")
		}
	}
	data, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return empty, err
	}
	if err := writePrivate(root, "evaluation.json", append(data, '\n')); err != nil {
		return empty, err
	}
	// Mutable operator registries are deliberately not frozen in the inventory.
	mutable := map[string]any{
		"assertion-sources.json": map[string]any{"default_source": "", "sources": []any{}},
		"adapter-logs.json": adapterlogs.Inventory{Adapters: []adapterlogs.Target{{
			ID: "logs_" + inventory.InstallID, Name: "OpenObserve (EVALUATION ONLY)",
			Sources: []adapterlogs.SourceConfig{evaluationLogSource(output, inventory)},
		}}},
	}
	for name, value := range mutable {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return empty, err
		}
		if err := writePrivate(root, name, append(data, '\n')); err != nil {
			return empty, err
		}
	}
	return inventory, nil
}

func readEvaluation(output string, config evaluationConfig) (evaluationInventory, error) {
	inventory := evaluationInventory{}
	if _, err := os.Lstat(filepath.Join(output, "admin-recovery.pending")); !errors.Is(err, os.ErrNotExist) {
		return inventory, errors.New("interrupted admin recovery: keep stopped and restore the complete offline backup; never reseed")
	}
	if err := filepath.WalkDir(output, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || (!entry.IsDir() && !info.Mode().IsRegular()) {
			return errors.New("state contains a symlink or special file; refuse unsafe reuse")
		}
		return nil
	}); err != nil {
		return inventory, err
	}
	data, err := os.ReadFile(filepath.Join(output, "evaluation.json"))
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.Version != 1 || inventory.Kind != "zpr-evaluation-only" || len(inventory.InstallID) != 24 {
		return inventory, errors.New("incomplete/invalid evaluation inventory; inspect state, never automatically reset it")
	}
	if _, err := hex.DecodeString(inventory.InstallID); err != nil {
		return inventory, errors.New("invalid install identity")
	}
	if !reflect.DeepEqual(inventory.Config, config) {
		return inventory, errors.New("existing install configuration differs; reuse original image/port/browser-ip flags or choose a NEW state directory; do not edit protected inventory to change an issuer")
	}
	for _, name := range []string{"assertion-sources.json", "adapter-logs.json"} {
		info, err := os.Lstat(filepath.Join(output, name))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return inventory, errors.New("operator registry missing or unprotected; refuse reseed")
		}
	}
	required := []string{"ca.pem", "ca-key.pem", "server-chain.pem", "server-key.pem", "client-chain.pem", "client-key.pem", "ldap-admin.secret", "ldap-reader.secret", "logging-admin.secret", "openobserve.env", "slapd.conf", "directory.ldif", "serve-directory.sh", "compose.json", "dex.json", "operator-admin.secret", "oidc-room-client.secret", "EVALUATION.txt", "source-registration.json"}
	if config.SimulatorPort != 0 {
		required = append(required, "oidc-simulator-client.secret")
	}
	if config.VisaService {
		required = append(required, evaluationVisaRequired(config)...)
	}
	if config.subjectLinks() {
		required = append(required, evaluationSubjectLinkFiles...)
	}
	if config.Directory != nil {
		required = append(required, evaluationDirectorySchemaFile)
	}
	if len(inventory.Files) != len(required) {
		return inventory, errors.New("evaluation inventory is missing required files")
	}
	for _, name := range required {
		expected, ok := inventory.Files[name]
		if !ok {
			return inventory, errors.New("evaluation inventory is missing required files")
		}
		info, err := os.Stat(filepath.Join(output, name))
		if err != nil || info.Mode().Perm() != 0o600 {
			return inventory, fmt.Errorf("protected file %s must remain private (0600)", name)
		}
		data, err := os.ReadFile(filepath.Join(output, name))
		sum := sha256.Sum256(data)
		if err != nil || hex.EncodeToString(sum[:]) != expected {
			return inventory, fmt.Errorf("protected file %s missing or changed; refuse regeneration", name)
		}
	}
	return inventory, nil
}

func generateEvaluation(state string, config evaluationConfig, now time.Time) (map[string][]byte, evaluationInventory, error) {
	files := map[string][]byte{}
	inventory := evaluationInventory{Version: 1, Kind: "zpr-evaluation-only", Config: config, Files: map[string]string{}}
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return nil, inventory, err
	}
	inventory.InstallID = hex.EncodeToString(random)
	var addresses []net.IP
	if config.BrowserIP != "" {
		addresses = []net.IP{net.ParseIP(config.BrowserIP)}
	}
	ca, key, caPEM, caKey, err := certificateWithOrganization("ZPR evaluation-only CA", "localhost", "ZPR EVALUATION ONLY", now, nil, nil, x509.ExtKeyUsageServerAuth, addresses...)
	if err != nil {
		return nil, inventory, err
	}
	defer key.D.SetInt64(0)
	files["ca.pem"], files["ca-key.pem"] = caPEM, caKey
	for name, usage := range map[string]x509.ExtKeyUsage{"server": x509.ExtKeyUsageServerAuth, "client": x509.ExtKeyUsageClientAuth} {
		_, leafKey, leaf, secret, err := certificateWithOrganization("ZPR evaluation-only "+name, "localhost", "ZPR EVALUATION ONLY", now, ca, key, usage, addresses...)
		if err != nil {
			return nil, inventory, err
		}
		leafKey.D.SetInt64(0)
		files[name+"-chain.pem"], files[name+"-key.pem"] = append(leaf, caPEM...), secret
	}
	secrets := []string{"ldap-admin", "ldap-reader", "logging-admin", "operator-admin", "oidc-room-client"}
	if config.subjectLinks() {
		secrets = append(secrets, "operator-reviewer")
	}
	for _, name := range secrets {
		random := make([]byte, 32)
		if _, err := rand.Read(random); err != nil {
			return nil, inventory, err
		}
		files[name+".secret"] = []byte(base64.RawURLEncoding.EncodeToString(random) + "!Aa1")
	}
	admin, err := ldapPasswordHash(strings.TrimSpace(string(files["ldap-admin.secret"])))
	if err != nil {
		return nil, inventory, err
	}
	reader, err := ldapPasswordHash(strings.TrimSpace(string(files["ldap-reader.secret"])))
	if err != nil {
		return nil, inventory, err
	}
	schemaInclude := ""
	if config.Directory != nil {
		files[evaluationDirectorySchemaFile] = []byte(evaluationDirectorySchema)
		schemaInclude = "include /etc/zpr/evaluation/" + evaluationDirectorySchemaFile + "\n"
	}
	baseDN := config.directoryBaseDN()
	files["slapd.conf"] = fmt.Appendf(nil, `include /etc/ldap/schema/core.schema
include /etc/ldap/schema/cosine.schema
include /etc/ldap/schema/inetorgperson.schema
%spidfile /var/lib/ldap/slapd.pid
argsfile /var/lib/ldap/slapd.args
modulepath /usr/lib/ldap
moduleload back_mdb
TLSCertificateFile /etc/zpr/evaluation/server-chain.pem
TLSCertificateKeyFile /etc/zpr/evaluation/server-key.pem
TLSCACertificateFile /etc/zpr/evaluation/ca.pem
TLSProtocolMin 3.3
disallow bind_anon
require authc
security simple_bind=128
database mdb
maxsize 1073741824
suffix "%s"
rootdn "cn=admin,%s"
rootpw %s
directory /var/lib/ldap
index objectClass eq
access to attrs=userPassword
    by anonymous auth
    by * none
access to *
    by dn.exact="%s" read
    by * none
`, schemaInclude, baseDN, baseDN, admin, config.directoryReaderDN())
	files["directory.ldif"] = fmt.Appendf(nil, `dn: dc=evaluation,dc=zpr
objectClass: top
objectClass: dcObject
objectClass: organization
dc: evaluation
o: ZPR EVALUATION ONLY %s

dn: cn=zpr-reader,dc=evaluation,dc=zpr
objectClass: organizationalRole
objectClass: simpleSecurityObject
cn: zpr-reader
userPassword: %s

dn: ou=People,dc=evaluation,dc=zpr
objectClass: organizationalUnit
ou: People

dn: ou=Groups,dc=evaluation,dc=zpr
objectClass: organizationalUnit
ou: Groups
`, inventory.InstallID, reader)
	var skippedDirectoryEntries []string
	if config.Directory != nil {
		if files["directory.ldif"], skippedDirectoryEntries, err = renderEvaluationDirectoryLDIF(config.Directory, reader); err != nil {
			return nil, inventory, err
		}
	}
	subjectUUID := ""
	if config.subjectLinks() {
		if subjectUUID, err = evaluationSubjectUUID(); err != nil {
			return nil, inventory, err
		}
		files["directory.ldif"] = append(files["directory.ldif"], evaluationSubjectLDIF(baseDN, subjectUUID)...)
	}
	files["serve-directory.sh"] = []byte(`#!/bin/sh
set -eu
umask 077
if [ ! -e /var/lib/ldap/evaluation-initialized ]; then
    if [ -n "$(find /var/lib/ldap -mindepth 1 -maxdepth 1 -print -quit)" ]; then
        echo 'Partial LDAP state: refusing reseed; inspect private state.' >&2
        exit 1
    fi
    slaptest -u -f /etc/zpr/evaluation/slapd.conf
    slapadd -f /etc/zpr/evaluation/slapd.conf -l /etc/zpr/evaluation/directory.ldif
    test -s /var/lib/ldap/data.mdb
    : > /var/lib/ldap/evaluation-initialized
fi
test -s /var/lib/ldap/data.mdb
exec slapd -f /etc/zpr/evaluation/slapd.conf -h ldaps://0.0.0.0:1636 -d 0
`)
	files["openobserve.env"] = fmt.Appendf(nil, `ZO_ROOT_USER_EMAIL=admin@evaluation.invalid
ZO_ROOT_USER_PASSWORD=%s
ZO_DATA_DIR=/data
ZO_HTTP_ADDR=0.0.0.0
ZO_HTTP_PORT=5080
ZO_HTTP_TLS_ENABLED=true
ZO_HTTP_TLS_CERT_PATH=/etc/zpr/evaluation/server-chain.pem
ZO_HTTP_TLS_KEY_PATH=/etc/zpr/evaluation/server-key.pem
ZO_COMPACT_DATA_RETENTION_DAYS=7
ZO_TELEMETRY=false
ZO_LOG_LEVEL=warn
`, strings.TrimSpace(string(files["logging-admin.secret"])))
	mount := func(source, target string, readonly bool) map[string]any {
		return map[string]any{"type": "bind", "source": "./" + source, "target": target, "read_only": readonly, "bind": map[string]bool{"create_host_path": false}}
	}
	service := func(image, state, target string, port int, containerPort int, names ...string) map[string]any {
		mounts := []map[string]any{mount(state, target, false)}
		for _, name := range names {
			mounts = append(mounts, mount(name, "/etc/zpr/evaluation/"+name, true))
		}
		return map[string]any{"image": image, "platform": "linux/amd64", "pull_policy": "never", "user": fmt.Sprintf("%d:%d", os.Geteuid(), os.Getegid()), "read_only": true, "cap_drop": []string{"ALL"}, "security_opt": []string{"no-new-privileges:true"}, "volumes": mounts,
			"ports": []string{fmt.Sprintf("127.0.0.1:%d:%d", port, containerPort)}, "pids_limit": 256, "mem_limit": "1g", "cpus": 2,
			"logging": map[string]any{"driver": "none"},
		}
	}
	ldapFiles := []string{"server-chain.pem", "server-key.pem", "ca.pem", "slapd.conf", "directory.ldif", "serve-directory.sh", "ldap-reader.secret"}
	if config.Directory != nil {
		ldapFiles = append(ldapFiles, evaluationDirectorySchemaFile)
	}
	ldap := service(config.LDAPImage, "ldap-state", "/var/lib/ldap", config.LDAPPort, 1636, ldapFiles...)
	ldap["entrypoint"] = []string{"/bin/sh", "/etc/zpr/evaluation/serve-directory.sh"}
	logging := service(config.LogImage, "logging-state", "/data", config.LogPort, 5080, "server-chain.pem", "server-key.pem")
	logging["platform"] = config.LogPlatform
	logging["env_file"] = []string{"./openobserve.env"}
	logging["environment"] = map[string]string{"RUST_LOG": "warn"}
	hash, err := bcrypt.GenerateFromPassword(files["operator-admin.secret"], bcrypt.DefaultCost)
	if err != nil {
		return nil, inventory, errors.New("cannot protect bootstrap administrator credential")
	}
	staticPasswords := []map[string]string{{"email": "admin@evaluation.invalid", "hash": string(hash), "username": "Evaluation Administrator", "userID": inventory.InstallID}}
	if config.subjectLinks() {
		reviewer, err := evaluationReviewerPassword(files["operator-reviewer.secret"])
		if err != nil {
			return nil, inventory, err
		}
		staticPasswords = append(staticPasswords, reviewer)
	}
	dexConfig := map[string]any{
		"issuer":           config.browserOrigin(config.OIDCPort),
		"storage":          map[string]any{"type": "sqlite3", "config": map[string]string{"file": "/var/lib/dex/dex.db"}},
		"web":              map[string]string{"https": "0.0.0.0:5556", "tlsCert": "/etc/zpr/evaluation/server-chain.pem", "tlsKey": "/etc/zpr/evaluation/server-key.pem"},
		"oauth2":           map[string]any{"responseTypes": []string{"code"}, "passwordConnector": "local"},
		"enablePasswordDB": true,
		"staticClients":    []map[string]any{{"id": "zpr-control-room", "name": "ZPR Evaluation Control Room", "secret": string(files["oidc-room-client.secret"]), "redirectURIs": []string{config.browserOrigin(config.RoomPort) + "/auth/operator/callback"}}},
		"staticPasswords":  staticPasswords,
		"logger":           map[string]string{"level": "error", "format": "json"},
	}
	if config.SimulatorPort != 0 {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return nil, inventory, err
		}
		files["oidc-simulator-client.secret"] = []byte(base64.RawURLEncoding.EncodeToString(secret))
		dexConfig["staticClients"] = append(dexConfig["staticClients"].([]map[string]any), map[string]any{
			"id": "zpr-simulator", "name": "ZPR Evaluation Simulator",
			"secret":       string(files["oidc-simulator-client.secret"]),
			"redirectURIs": []string{config.browserOrigin(config.SimulatorPort) + "/auth/operator/callback"},
		})
	}
	data, err := json.MarshalIndent(dexConfig, "", "  ")
	if err != nil {
		return nil, inventory, err
	}
	files["dex.json"] = append(data, '\n')
	dex := service(config.DexImage, "oidc-state", "/var/lib/dex", config.OIDCPort, 5556, "server-chain.pem", "server-key.pem", "dex.json")
	if config.BrowserIP != "" {
		dex["ports"] = []string{fmt.Sprintf("127.0.0.1:%d:5556", config.OIDCPort), fmt.Sprintf("%s:%d:5556", config.BrowserIP, config.OIDCPort)}
	}
	dex["platform"] = config.DexPlatform
	dex["entrypoint"] = []string{"dex", "serve", "/etc/zpr/evaluation/dex.json"}
	compose := map[string]any{"name": "zpr-evaluation-" + inventory.InstallID, "services": map[string]any{"ldap": ldap, "logging": logging, "oidc": dex}}
	data, err = json.MarshalIndent(compose, "", "  ")
	if err != nil {
		return nil, inventory, err
	}
	files["compose.json"] = append(data, '\n')
	files["EVALUATION.txt"] = []byte(evaluationBanner + "\nOpenObserve AGPL-3.0 accepted for evaluation; 7-day retention; admin-only loopback access.\nRegistration and lifecycle logging use ordinary production contracts. No policy enforcement or production install is claimed.\n")
	if config.BrowserIP != "" {
		files["EVALUATION.txt"] = append(files["EVALUATION.txt"], []byte("Browser HTTPS/OIDC explicitly enabled at "+config.BrowserIP+"; restrict access to intended clients. Localhost Room redirects to the canonical network origin.\n")...)
	}
	if config.VisaService {
		if err := generateEvaluationVisa(files, state, inventory, now); err != nil {
			return nil, inventory, err
		}
		note := "Loopback Visa Service with the bundled test-signed evaluation policy and private Valkey. No nodes, adapters or enforcement.\n"
		if config.DataPlane {
			note = "Data-plane container: Visa Service, private Valkey, one node and the vs.zpr adapter with unique per-install keys and a per-install policy. No endpoints or production enforcement.\n"
		}
		files["EVALUATION.txt"] = append(files["EVALUATION.txt"], []byte(note)...)
	}
	if config.subjectLinks() {
		if err := generateEvaluationSubjectLinks(files, state, inventory, now, subjectUUID, ca, key); err != nil {
			return nil, inventory, err
		}
		files["EVALUATION.txt"] = append(files["EVALUATION.txt"], []byte("Subject-link review, publication and pinned snapshot delivery to the loopback VS use EVALUATION-ONLY keys; see SUBJECT-LINKS.txt. Snapshots are fetched and checkpointed, never enforced.\n")...)
	}
	if config.Directory != nil {
		note := "Directory: operator LDIF imported under " + baseDN + " at creation; the install's reader is " + config.directoryReaderDN() + ".\n"
		if len(skippedDirectoryEntries) > 0 {
			note += "Not imported (entries carrying userPassword): " + strings.Join(skippedDirectoryEntries, "; ") + ".\n"
		}
		files["EVALUATION.txt"] = append(files["EVALUATION.txt"], []byte(note)...)
	}
	registration := evaluationSourceRegistration(state, inventory)
	data, err = json.MarshalIndent(registration, "", "  ")
	if err != nil {
		return nil, inventory, err
	}
	files["source-registration.json"] = append(data, '\n')
	for name, data := range files {
		sum := sha256.Sum256(data)
		inventory.Files[name] = hex.EncodeToString(sum[:])
	}
	return files, inventory, nil
}
