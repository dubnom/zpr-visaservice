package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The maintained per-organization provisioning command wraps the proven
// load-lab sequence behind one resumable, fail-closed entry point driven by a
// small operator-owned provisioning manifest: the base operator tree, the
// organization, its single pinned data-plane image and the declared
// on-the-fly assets. BAS is part of the minimum ZPR stack, so nothing
// BAS-specific is declared: the adapter runs the same pinned image and its
// listeners derive from the install's own recorded browser IP and port block,
// exactly like every path derives from the base-tree convention. Stages that
// already have maintained implementations are executed in-process and rely on
// their own resume-or-verify semantics (Simulator pairing, the
// pairing-derived enrollment administration trust, BAS provisioning). Stages
// whose outcomes are observable files are verified with explicit evidence
// (onboarded endpoint identity bundles, control-TLS material, the
// machine-runtime contract including the pinned controller digest, and the
// Visa Service's operator-configured trusted-service HTTPS wiring to BAS —
// without it endpoint admission fails only at the first machine scenario). Stages
// that live inside running services (the organization enrollment readiness
// preflight and the class-based workload policy applied through the
// self-approved pipeline) are never inferred: the manifest must carry the
// operator's explicit evidence pointers, and the command stops with the exact
// bounded next action until it does. Reruns are safe; nothing is created
// outside the stages' own maintained paths, and retained inventory is never
// provisioned here.

// evaluationProvisioningManifest is the operator-owned input. Every path and
// listener derives from the base tree and the install's own recorded
// configuration; the optional overrides exist for estates that deviate from
// the convention.
type evaluationProvisioningManifest struct {
	Version        int      `json:"version"`
	Organization   string   `json:"organization"`
	Base           string   `json:"base"`
	DataPlaneImage string   `json:"data_plane_image"`
	DeclaredAssets []string `json:"declared_assets"`
	// PreflightAttestation is the operator's explicit pointer to the saved
	// organization enrollment readiness preflight (who saved it and when);
	// the preflight itself lives in the Enrollment Service registry.
	PreflightAttestation string `json:"preflight_attestation,omitempty"`
	// PolicyRecordID/Revision point at the applied class-based workload
	// policy record from the self-approved pipeline; application is never
	// inferred from file contents here.
	PolicyRecordID       string                           `json:"policy_record_id,omitempty"`
	PolicyRecordRevision int                              `json:"policy_record_revision,omitempty"`
	Overrides            *evaluationProvisioningOverrides `json:"overrides,omitempty"`
}

// evaluationProvisioningOverrides deviate from the base-tree convention;
// paths are relative to base, listeners and ports are explicit.
type evaluationProvisioningOverrides struct {
	State                string `json:"state,omitempty"`
	OperatorServicesFile string `json:"operator_services_file,omitempty"`
	EnrollmentTrustFile  string `json:"enrollment_trust_file,omitempty"`
	EnrollmentConfig     string `json:"enrollment_config,omitempty"`
	BASDir               string `json:"bas_dir,omitempty"`
	EndpointsRoot        string `json:"endpoints_root,omitempty"`
	ControlTLSDir        string `json:"control_tls_dir,omitempty"`
	MachineRuntimeFile   string `json:"machine_runtime_file,omitempty"`
	SimulatorPort        int    `json:"simulator_port,omitempty"`
	BASAdapterListen     string `json:"bas_adapter_listen,omitempty"`
	BASTokenListen       string `json:"bas_token_listen,omitempty"`
}

// evaluationProvisioningPlan is the fully resolved per-stage input set.
type evaluationProvisioningPlan struct {
	Organization         string
	State                string
	SimulatorPort        int
	OperatorServicesFile string
	EnrollmentTrustFile  string
	EnrollmentConfig     string
	BASDir               string
	BASAdapterListen     string
	BASTokenListen       string
	BASAdapterImage      string
	EndpointsRoot        string
	DeclaredAssets       []string
	ControlTLSDir        string
	MachineRuntimeFile   string
	PreflightAttestation string
	PolicyRecordID       string
	PolicyRecordRevision int
}

func validProvisioningOrganization(id string) bool {
	if id == "" || len(id) > 48 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func validProvisioningAsset(id string) bool {
	return id != "" && len(id) <= 128 && !strings.ContainsAny(id, "/\\.\x00 \t\r\n")
}

func validProvisioningListen(listen string) bool {
	host, port, err := net.SplitHostPort(listen)
	if err != nil || host == "" {
		return false
	}
	value, err := strconv.Atoi(port)
	return err == nil && value > 0 && value < 65536
}

func (m evaluationProvisioningManifest) validate() error {
	if m.Version != 1 {
		return errors.New("provisioning manifest requires version 1")
	}
	if !validProvisioningOrganization(m.Organization) {
		return errors.New("organization must be a lowercase DNS-safe identifier")
	}
	if !filepath.IsAbs(m.Base) || filepath.Clean(m.Base) != m.Base {
		return errors.New("base requires a canonical absolute operator-tree path")
	}
	digest, ok := strings.CutPrefix(m.DataPlaneImage, "sha256:")
	if raw, err := hex.DecodeString(digest); !ok || err != nil || len(raw) != sha256.Size {
		return errors.New("data_plane_image must be the organization's exact pinned sha256: image ID")
	}
	if len(m.DeclaredAssets) == 0 {
		return errors.New("declared_assets must list the organization's on-the-fly endpoint assets; retained inventory is never provisioned here")
	}
	seen := map[string]bool{}
	for _, asset := range m.DeclaredAssets {
		if !validProvisioningAsset(asset) || seen[asset] {
			return errors.New("declared_assets require unique plain asset identifiers")
		}
		seen[asset] = true
	}
	if m.PolicyRecordID != "" && m.PolicyRecordRevision < 1 {
		return errors.New("policy_record_id requires its applied policy_record_revision")
	}
	if m.Overrides == nil {
		return nil
	}
	for name, path := range map[string]string{
		"state": m.Overrides.State, "enrollment_trust_file": m.Overrides.EnrollmentTrustFile,
		"operator_services_file": m.Overrides.OperatorServicesFile,
		"enrollment_config":      m.Overrides.EnrollmentConfig, "bas_dir": m.Overrides.BASDir,
		"endpoints_root": m.Overrides.EndpointsRoot, "control_tls_dir": m.Overrides.ControlTLSDir,
		"machine_runtime_file": m.Overrides.MachineRuntimeFile,
	} {
		if path != "" && !filepath.IsLocal(path) {
			return fmt.Errorf("override %s must be a clean path relative to base", name)
		}
	}
	if m.Overrides.SimulatorPort != 0 && (m.Overrides.SimulatorPort < 1 || m.Overrides.SimulatorPort > 65535) {
		return errors.New("override simulator_port must be an explicit TCP port")
	}
	for name, listen := range map[string]string{
		"bas_adapter_listen": m.Overrides.BASAdapterListen, "bas_token_listen": m.Overrides.BASTokenListen,
	} {
		if listen != "" && !validProvisioningListen(listen) {
			return fmt.Errorf("override %s requires an explicit IP:port listener", name)
		}
	}
	return nil
}

func readEvaluationProvisioningManifest(path string) (evaluationProvisioningManifest, error) {
	var manifest evaluationProvisioningManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, errors.New("cannot read the provisioning manifest")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, errors.New("provisioning manifest must be strict JSON with only known fields")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return manifest, errors.New("provisioning manifest must contain exactly one JSON document")
	}
	if err := manifest.validate(); err != nil {
		return manifest, err
	}
	return manifest, nil
}

// resolveEvaluationProvisioningPlan derives every stage input from the base
// tree and the install's recorded configuration. The BAS listeners and the
// Simulator callback port come from the install's own browser IP and port
// block (room port N787 anchors the block), and the BAS adapter runs the
// organization's single pinned data-plane image.
func resolveEvaluationProvisioningPlan(manifest evaluationProvisioningManifest) (evaluationProvisioningPlan, error) {
	overrides := manifest.Overrides
	if overrides == nil {
		overrides = &evaluationProvisioningOverrides{}
	}
	resolve := func(override string, conventional ...string) string {
		if override != "" {
			return filepath.Join(manifest.Base, override)
		}
		return filepath.Join(append([]string{manifest.Base}, conventional...)...)
	}
	organization := manifest.Organization
	plan := evaluationProvisioningPlan{
		Organization:         organization,
		State:                resolve(overrides.State, "state", organization),
		EnrollmentTrustFile:  resolve(overrides.EnrollmentTrustFile, "enrollment", organization, "delegation-trust-self-approval.json"),
		BASDir:               resolve(overrides.BASDir, "enrollment", organization, "bas"),
		EndpointsRoot:        resolve(overrides.EndpointsRoot, "endpoints", organization),
		ControlTLSDir:        resolve(overrides.ControlTLSDir, "enrollment", organization, "control"),
		MachineRuntimeFile:   resolve(overrides.MachineRuntimeFile, "enrollment", organization, "simulator-retained-machine-runtime.json"),
		BASAdapterImage:      manifest.DataPlaneImage,
		DeclaredAssets:       manifest.DeclaredAssets,
		PreflightAttestation: manifest.PreflightAttestation,
		PolicyRecordID:       manifest.PolicyRecordID,
		PolicyRecordRevision: manifest.PolicyRecordRevision,
	}
	if overrides.EnrollmentConfig != "" {
		plan.EnrollmentConfig = filepath.Join(manifest.Base, overrides.EnrollmentConfig)
	} else {
		// Mirror the organization service convention: the development
		// configuration wins when it exists.
		development := filepath.Join(manifest.Base, "enrollment", organization, "service-development.json")
		if info, err := os.Lstat(development); err == nil && info.Mode().IsRegular() {
			plan.EnrollmentConfig = development
		} else {
			plan.EnrollmentConfig = filepath.Join(manifest.Base, "enrollment", organization, "service.json")
		}
	}
	if overrides.OperatorServicesFile != "" {
		plan.OperatorServicesFile = filepath.Join(manifest.Base, overrides.OperatorServicesFile)
	} else {
		// Mirror the organization runtime convention: pairing derives the
		// paired service bindings from the operator-owned services file.
		for _, conventional := range []string{
			filepath.Join(manifest.Base, "enrollment", organization, "operator-services.json"),
			filepath.Join(manifest.Base, "policy", organization, "operator-services.json"),
		} {
			if info, err := os.Lstat(conventional); err == nil && info.Mode().IsRegular() {
				plan.OperatorServicesFile = conventional
				break
			}
		}
		if plan.OperatorServicesFile == "" {
			return plan, errors.New("no operator-owned operator-services.json exists for the organization; pairing cannot derive the paired service bindings")
		}
	}
	inventory, err := readEvaluationAdapterInventory(plan.State)
	if err != nil {
		return plan, fmt.Errorf("resolve install configuration from %s: %w", plan.State, err)
	}
	if inventory.Config.OrganizationID != "" && inventory.Config.OrganizationID != organization {
		return plan, fmt.Errorf("install at %s belongs to organization %q, not %q", plan.State, inventory.Config.OrganizationID, organization)
	}
	block := inventory.Config.RoomPort - 787
	if inventory.Config.RoomPort < 788 {
		return plan, errors.New("install room port does not anchor a derivable port block; set explicit overrides")
	}
	plan.SimulatorPort = overrides.SimulatorPort
	if plan.SimulatorPort == 0 {
		plan.SimulatorPort = inventory.Config.SimulatorPort
	}
	if plan.SimulatorPort == 0 {
		plan.SimulatorPort = block + 788
	}
	browser := inventory.Config.BrowserIP
	plan.BASAdapterListen = overrides.BASAdapterListen
	plan.BASTokenListen = overrides.BASTokenListen
	if plan.BASAdapterListen == "" || plan.BASTokenListen == "" {
		if browser == "" || block+997 > 65535 {
			return plan, errors.New("install configuration lacks a browser IP or port block for the BAS listeners; set explicit overrides")
		}
		if plan.BASAdapterListen == "" {
			plan.BASAdapterListen = net.JoinHostPort(browser, strconv.Itoa(block+996))
		}
		if plan.BASTokenListen == "" {
			plan.BASTokenListen = net.JoinHostPort(browser, strconv.Itoa(block+997))
		}
	}
	return plan, nil
}

type evaluationProvisioningStage struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // executed, verified, attested
	Status   string `json:"status"`
	Evidence string `json:"evidence,omitempty"`
}

type evaluationProvisioningRecord struct {
	Organization string                        `json:"organization"`
	Manifest     string                        `json:"manifest"`
	Complete     bool                          `json:"complete"`
	Stages       []evaluationProvisioningStage `json:"stages"`
	RecordedAt   time.Time                     `json:"recorded_at"`
}

// evaluationProvisioningDelegates lets tests substitute the maintained stage
// commands; defaults call the real in-process implementations.
type evaluationProvisioningDelegates struct {
	pairing         func(args []string, stdout, stderr io.Writer) int
	enrollmentTrust func(args []string, stdout, stderr io.Writer) int
	bas             func(args []string, stdout, stderr io.Writer) int
	now             func() time.Time
}

func defaultEvaluationProvisioningDelegates() evaluationProvisioningDelegates {
	return evaluationProvisioningDelegates{
		pairing:         runEvaluationSimulatorPairing,
		enrollmentTrust: runEvaluationSimulatorEnrollmentTrust,
		bas:             runEvaluationBAS,
		now:             time.Now,
	}
}

func runEvaluationProvisionOrganization(args []string, stdout, stderr io.Writer) int {
	return runEvaluationProvisionOrganizationWith(args, stdout, stderr, defaultEvaluationProvisioningDelegates())
}

func runEvaluationProvisionOrganizationWith(args []string, stdout, stderr io.Writer, delegates evaluationProvisioningDelegates) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-provision-organization", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", "", "operator-owned provisioning manifest JSON (required, absolute path)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *manifestPath == "" || flags.NArg() != 0 || !filepath.IsAbs(*manifestPath) {
		fmt.Fprintln(stderr, "usage: zpr-lab-bootstrap -evaluation-provision-organization -manifest /absolute/provisioning-manifest.json")
		return 2
	}
	fmt.Fprintln(stdout, evaluationBanner)
	manifest, err := readEvaluationProvisioningManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "Provisioning manifest: %v\n", err)
		return 1
	}
	plan, err := resolveEvaluationProvisioningPlan(manifest)
	if err != nil {
		fmt.Fprintf(stderr, "Provisioning: %v\n", err)
		return 1
	}
	record := evaluateProvisioningStages(plan, *manifestPath, stdout, stderr, delegates)
	for _, stage := range record.Stages {
		fmt.Fprintf(stdout, "stage %-20s %-9s %-10s %s\n", stage.Name, stage.Kind, stage.Status, stage.Evidence)
	}
	recordPath := filepath.Join(filepath.Dir(*manifestPath), manifest.Organization+"-provisioning-record.json")
	if err := writeProvisioningRecord(recordPath, record); err != nil {
		fmt.Fprintf(stderr, "Provisioning record could not be written: %v\n", err)
		return 1
	}
	if !record.Complete {
		fmt.Fprintln(stdout, "Provisioning is incomplete; the first blocked stage above names the exact bounded next action. Rerun after completing it — every stage is resume-or-verify.")
		return 3
	}
	fmt.Fprintf(stdout, "Organization %s is provisioned on the on-the-fly model: maintained stages executed, file evidence verified, and operator-attested stages recorded in %s.\n", manifest.Organization, recordPath)
	return 0
}

func evaluateProvisioningStages(plan evaluationProvisioningPlan, manifestPath string, stdout, stderr io.Writer, delegates evaluationProvisioningDelegates) evaluationProvisioningRecord {
	record := evaluationProvisioningRecord{
		Organization: plan.Organization, Manifest: manifestPath, RecordedAt: delegates.now().UTC(),
	}
	blocked := false
	add := func(stage evaluationProvisioningStage) {
		if blocked && stage.Status == "" {
			stage.Status = "not-reached"
		}
		record.Stages = append(record.Stages, stage)
		if stage.Status == "blocked" {
			blocked = true
		}
	}
	execute := func(name string, delegate func([]string, io.Writer, io.Writer) int, args ...string) {
		if blocked {
			add(evaluationProvisioningStage{Name: name, Kind: "executed"})
			return
		}
		fmt.Fprintf(stdout, "== stage %s\n", name)
		if code := delegate(args, stdout, stderr); code != 0 {
			add(evaluationProvisioningStage{Name: name, Kind: "executed", Status: "blocked",
				Evidence: fmt.Sprintf("maintained command exited %d; resolve its explicit message and rerun", code)})
			return
		}
		add(evaluationProvisioningStage{Name: name, Kind: "executed", Status: "completed"})
	}

	execute("simulator-pairing", delegates.pairing,
		"-state", plan.State, "-simulator-port", strconv.Itoa(plan.SimulatorPort),
		"-operator-services-file", plan.OperatorServicesFile)
	execute("enrollment-trust", delegates.enrollmentTrust,
		"-state", plan.State, "-enrollment-trust-file", plan.EnrollmentTrustFile)
	execute("bas", delegates.bas,
		"-state", plan.State, "-enrollment-config", plan.EnrollmentConfig,
		"-bas-dir", plan.BASDir, "-adapter-listen", plan.BASAdapterListen,
		"-token-listen", plan.BASTokenListen, "-adapter-image", plan.BASAdapterImage)

	if blocked {
		add(evaluationProvisioningStage{Name: "visa-bas-trust", Kind: "verified"})
		add(evaluationProvisioningStage{Name: "enrollment-preflight", Kind: "attested"})
		add(evaluationProvisioningStage{Name: "endpoint-onboarding", Kind: "verified"})
		add(evaluationProvisioningStage{Name: "control-tls", Kind: "verified"})
		add(evaluationProvisioningStage{Name: "machine-runtime", Kind: "verified"})
		add(evaluationProvisioningStage{Name: "workload-policy", Kind: "attested"})
		return record
	}

	add(verifyProvisioningVisaBASTrust(plan))

	if plan.PreflightAttestation == "" {
		add(evaluationProvisioningStage{Name: "enrollment-preflight", Kind: "attested", Status: "blocked",
			Evidence: "save the organization enrollment readiness preflight through the Room, then record preflight_attestation (who saved it and when) in the manifest"})
	} else {
		add(evaluationProvisioningStage{Name: "enrollment-preflight", Kind: "attested", Status: "recorded",
			Evidence: plan.PreflightAttestation})
	}

	add(verifyProvisioningOnboarding(plan))
	add(verifyProvisioningControlTLS(plan))
	add(verifyProvisioningMachineRuntime(plan))

	if plan.PolicyRecordID == "" {
		add(evaluationProvisioningStage{Name: "workload-policy", Kind: "attested", Status: "blocked",
			Evidence: "apply the class-based workload policy (BAS organization/profile/type classes, no asset pins) through the self-approved policy pipeline, then record policy_record_id and policy_record_revision in the manifest"})
	} else {
		add(evaluationProvisioningStage{Name: "workload-policy", Kind: "attested", Status: "recorded",
			Evidence: fmt.Sprintf("policy record %s revision %d (applied through the self-approved pipeline)", plan.PolicyRecordID, plan.PolicyRecordRevision)})
	}
	record.Complete = true
	for _, stage := range record.Stages {
		if stage.Status != "completed" && stage.Status != "verified" && stage.Status != "recorded" {
			record.Complete = false
		}
	}
	return record
}

var provisioningIdentityFiles = []string{"identity.json", "runtime-auth.key", "runtime-auth-ca.pem", "runtime-auth.toml"}

func verifyProvisioningOnboarding(plan evaluationProvisioningPlan) evaluationProvisioningStage {
	stage := evaluationProvisioningStage{Name: "endpoint-onboarding", Kind: "verified"}
	var missing []string
	for _, asset := range plan.DeclaredAssets {
		for _, name := range provisioningIdentityFiles {
			info, err := os.Lstat(filepath.Join(plan.EndpointsRoot, asset, "identity", name))
			if err != nil || !info.Mode().IsRegular() {
				missing = append(missing, asset)
				break
			}
		}
	}
	if len(missing) > 0 {
		stage.Status = "blocked"
		stage.Evidence = fmt.Sprintf("assets without assembled endpoint identity bundles: %s; onboard each through the Simulator single-call onboarding endpoint and assemble its identity directory from the delivered bundle", strings.Join(missing, ", "))
		return stage
	}
	stage.Status = "verified"
	stage.Evidence = fmt.Sprintf("%d declared assets have complete identity bundles under %s", len(plan.DeclaredAssets), plan.EndpointsRoot)
	return stage
}

// provisioningVisaBASFiles are the BAS-issued files the Visa Service needs to
// validate on-the-fly endpoint admissions over HTTPS, relative to the BAS dir.
func provisioningVisaBASFiles(plan evaluationProvisioningPlan) []string {
	return []string{
		filepath.Join(plan.BASDir, "bas-ca.pem"),
		filepath.Join(plan.BASDir, plan.Organization+"-visa-service-runtime.pem"),
		filepath.Join(plan.BASDir, plan.Organization+"-visa-service-runtime.key"),
		filepath.Join(plan.BASDir, "token-signing.key"),
	}
}

// verifyProvisioningVisaBASTrust verifies the operator Visa Service
// configuration wires the organization's BAS as an HTTPS trusted service:
// without [trusted_service_http.<org>-enrollment-bas] and the BAS runtime
// files mounted read-only, VS authorize_connect fails only at the first
// machine scenario ("missing HTTPS and token-key configuration").
func verifyProvisioningVisaBASTrust(plan evaluationProvisioningPlan) evaluationProvisioningStage {
	stage := evaluationProvisioningStage{Name: "visa-bas-trust", Kind: "verified"}
	blockedWith := func(format string, args ...any) evaluationProvisioningStage {
		stage.Status = "blocked"
		stage.Evidence = fmt.Sprintf(format, args...)
		return stage
	}
	serviceID := plan.Organization + "-enrollment-bas"
	data, err := os.ReadFile(plan.OperatorServicesFile)
	if err != nil {
		return blockedWith("operator services file %s is unreadable", plan.OperatorServicesFile)
	}
	var services struct {
		VisaServiceConfigFile    string   `json:"visa_service_config_file"`
		VisaServiceReadOnlyFiles []string `json:"visa_service_read_only_files"`
	}
	if json.Unmarshal(data, &services) != nil || services.VisaServiceConfigFile == "" {
		return blockedWith("operator services file %s must name the operator Visa configuration in visa_service_config_file", plan.OperatorServicesFile)
	}
	required := provisioningVisaBASFiles(plan)
	for _, name := range required {
		info, err := os.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return blockedWith("BAS Visa Service runtime file %s is missing; the BAS stage emits it next to the adapter material", name)
		}
		if !slices.Contains(services.VisaServiceReadOnlyFiles, name) {
			return blockedWith("operator services visa_service_read_only_files must mount %s into the data plane for the Visa Service", name)
		}
	}
	table, err := provisioningTOMLTable(services.VisaServiceConfigFile, "trusted_service_http."+serviceID)
	if err != nil {
		return blockedWith("operator Visa configuration %s: %v", services.VisaServiceConfigFile, err)
	}
	_, tokenPort, err := net.SplitHostPort(plan.BASTokenListen)
	if err != nil {
		return blockedWith("BAS token listener %q is not an IP:port", plan.BASTokenListen)
	}
	expected := map[string]string{
		"url":                         "https://auth.zpr:" + tokenPort,
		"ca_cert":                     required[0],
		"client_cert":                 required[1],
		"client_key":                  required[2],
		"token_verification_key_file": required[3],
	}
	for key, value := range expected {
		if table[key] != value {
			return blockedWith("operator Visa configuration %s: [trusted_service_http.%s] must set %s = %q so endpoint admission can validate against this organization's BAS", services.VisaServiceConfigFile, serviceID, key, value)
		}
	}
	stage.Status = "verified"
	stage.Evidence = fmt.Sprintf("operator Visa configuration wires %s over HTTPS at auth.zpr:%s with its BAS runtime files mounted read-only", serviceID, tokenPort)
	return stage
}

// provisioningTOMLTable extracts the string keys of one named TOML table with
// a strict minimal scan; it exists to verify operator-owned wiring, not to be
// a general TOML parser.
func provisioningTOMLTable(path, name string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("configuration file is unreadable")
	}
	table := map[string]string{}
	inTable := false
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inTable = line == "["+name+"]"
			continue
		}
		if !inTable || line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		table[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	if len(table) == 0 {
		return nil, fmt.Errorf("missing the [%s] table", name)
	}
	return table, nil
}

func verifyProvisioningControlTLS(plan evaluationProvisioningPlan) evaluationProvisioningStage {
	stage := evaluationProvisioningStage{Name: "control-tls", Kind: "verified"}
	for _, name := range []string{"control-ca.crt", "server.crt", "server.key"} {
		info, err := os.Lstat(filepath.Join(plan.ControlTLSDir, name))
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			stage.Status = "blocked"
			stage.Evidence = fmt.Sprintf("control-TLS material is missing %s; run the generic prepare-retained-machine-control script into %s", name, plan.ControlTLSDir)
			return stage
		}
	}
	stage.Status = "verified"
	stage.Evidence = "control CA and server material present in " + plan.ControlTLSDir
	return stage
}

type provisioningMachineRuntime struct {
	RetainedMachineRuntime struct {
		Version        int    `json:"version"`
		ControllerFile string `json:"controller_file"`
		ControllerSHA  string `json:"controller_sha256"`
		EndpointRoot   string `json:"endpoint_root"`
		ControlTLSDir  string `json:"control_tls_dir"`
		ControlAsset   string `json:"control_asset"`
		Machines       []struct {
			ControllerAsset string `json:"controller_asset"`
			WorkloadAsset   string `json:"workload_asset"`
		} `json:"machines"`
	} `json:"retained_machine_runtime"`
}

func verifyProvisioningMachineRuntime(plan evaluationProvisioningPlan) evaluationProvisioningStage {
	stage := evaluationProvisioningStage{Name: "machine-runtime", Kind: "verified"}
	blockedWith := func(format string, args ...any) evaluationProvisioningStage {
		stage.Status = "blocked"
		stage.Evidence = fmt.Sprintf(format, args...)
		return stage
	}
	data, err := os.ReadFile(plan.MachineRuntimeFile)
	if err != nil {
		return blockedWith("machine runtime contract %s is unreadable; write the bindings with the granted-address control rendezvous", plan.MachineRuntimeFile)
	}
	var runtime provisioningMachineRuntime
	if json.Unmarshal(data, &runtime) != nil || runtime.RetainedMachineRuntime.Version != 1 {
		return blockedWith("machine runtime contract %s is not a version-1 binding", plan.MachineRuntimeFile)
	}
	binding := runtime.RetainedMachineRuntime
	if binding.EndpointRoot != plan.EndpointsRoot || binding.ControlTLSDir != plan.ControlTLSDir {
		return blockedWith("machine runtime binding endpoint_root/control_tls_dir differ from the resolved plan; reconcile the binding before provisioning is recorded")
	}
	declared := map[string]bool{}
	for _, asset := range plan.DeclaredAssets {
		declared[asset] = true
	}
	referenced := []string{binding.ControlAsset}
	for _, machine := range binding.Machines {
		referenced = append(referenced, machine.ControllerAsset, machine.WorkloadAsset)
	}
	for _, asset := range referenced {
		if !declared[asset] {
			return blockedWith("machine runtime binding references asset %q outside declared_assets; the on-the-fly contract provisions only declared endpoints", asset)
		}
	}
	controller, err := os.ReadFile(binding.ControllerFile)
	if err != nil {
		return blockedWith("pinned controller %s is unreadable", binding.ControllerFile)
	}
	digest := sha256.Sum256(controller)
	if hex.EncodeToString(digest[:]) != binding.ControllerSHA {
		return blockedWith("pinned controller %s does not match the recorded contract digest; refuse the stale binding", binding.ControllerFile)
	}
	stage.Status = "verified"
	stage.Evidence = fmt.Sprintf("version-1 binding, controller digest verified, %d machines reference only declared assets", len(binding.Machines))
	return stage
}

func writeProvisioningRecord(path string, record evaluationProvisioningRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
