package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	evaluationruntime "neboagency.com/zpr-dashborad/internal/simulator/evaluation"
	"neboagency.com/zpr-dashborad/internal/simulator/setupmode"
)

type evaluationSimulatorOutput struct {
	mu     *sync.Mutex
	writer io.Writer
}

func (output evaluationSimulatorOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.writer.Write(data)
}

func runEvaluationSimulator(args []string, stdout, stderr io.Writer) int {
	var outputMu sync.Mutex
	stdout = evaluationSimulatorOutput{mu: &outputMu, writer: stdout}
	stderr = evaluationSimulatorOutput{mu: &outputMu, writer: stderr}
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-simulator", flag.ContinueOnError)
	flags.SetOutput(stderr)
	evaluation := flags.String("evaluation-state", "", "existing independently running evaluation with -simulator-port")
	state := flags.String("state", "", "separate private Simulator state directory")
	binary := flags.String("binary", "", "absolute prebuilt Simulator executable")
	image := flags.String("data-plane-image", "", "exact paired data-plane image ID")
	catalog := flags.String("catalog", "", "optional packaged five-organization Simulator catalog")
	oidcOverride := flags.String("simulator-oidc-file", "", "optional protected Simulator OIDC session override that may add only explicit enrollment/budget permissions to the frozen named grants")
	retainedFile := flags.String("retained-machine-runtime-file", "", "optional protected retained topology/machine-runtime bindings for this installation's paired Simulator launch")
	enrollmentDeviceURL := flags.String("enrollment-device-url", "", "optional exact HTTPS origin of this installation's Enrollment Service device listener for explicit Simulator onboarding")
	basURL := flags.String("enrollment-bas-url", "", "optional exact HTTPS dial origin of this installation's enrollment BAS adapter listener for the explicit runtime credential check")
	basAuthority := flags.String("enrollment-bas-authority", "", "exact host:port authority the BAS adapter listener expects (required with enrollment-bas-url)")
	basCAFile := flags.String("enrollment-bas-ca-file", "", "protected CA bundle for the BAS adapter listener TLS (required with enrollment-bas-url)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "Simulator setup accepts flags only")
		return 2
	}
	if err := validateEvaluationSimulatorArguments(*evaluation, *state, *binary, *image); err != nil {
		fmt.Fprintf(stderr, "Simulator setup: %v\n", err)
		return 1
	}
	lock, err := lockEvaluationSimulator(*evaluation)
	if err != nil {
		fmt.Fprintf(stderr, "Simulator lifecycle: %v\n", err)
		return 1
	}
	defer lock.Close()
	env, listen, err := prepareEvaluationSimulator(*evaluation, *state, *binary, *image, *retainedFile)
	if err != nil {
		fmt.Fprintf(stderr, "Simulator setup: %v\n", err)
		return 1
	}
	if *oidcOverride != "" {
		env, err = applyEvaluationSimulatorOIDCOverride(*evaluation, *oidcOverride, env)
		if err != nil {
			fmt.Fprintf(stderr, "Simulator setup: %v\n", err)
			return 1
		}
	}
	if *enrollmentDeviceURL != "" {
		origin, err := url.Parse(*enrollmentDeviceURL)
		if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
			*enrollmentDeviceURL != "https://"+origin.Host {
			fmt.Fprintln(stderr, "Simulator setup: enrollment-device-url must be an exact HTTPS origin")
			return 1
		}
		env = append(env, "SIMULATOR_ENROLLMENT_DEVICE_URL="+*enrollmentDeviceURL,
			"SIMULATOR_ENROLLMENT_DEVICE_CA_FILE="+filepath.Join(*evaluation, "ca.pem"))
	}
	if *basURL != "" || *basAuthority != "" || *basCAFile != "" {
		origin, err := url.Parse(*basURL)
		host, port, hostErr := net.SplitHostPort(*basAuthority)
		if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil ||
			*basURL != "https://"+origin.Host || hostErr != nil || host == "" || port == "" ||
			!filepath.IsAbs(*basCAFile) || filepath.Clean(*basCAFile) != *basCAFile {
			fmt.Fprintln(stderr, "Simulator setup: the BAS credential check requires an exact HTTPS dial origin, the listener's exact host:port authority and an absolute CA file")
			return 1
		}
		env = append(env, "SIMULATOR_ENROLLMENT_BAS_URL="+*basURL,
			"SIMULATOR_ENROLLMENT_BAS_AUTHORITY="+*basAuthority,
			"SIMULATOR_ENROLLMENT_BAS_CA_FILE="+*basCAFile)
	}
	if *catalog != "" {
		env, err = prepareEvaluationSimulatorCatalog(*evaluation, *state, *catalog, env)
		if err != nil {
			fmt.Fprintf(stderr, "Simulator catalog: %v\n", err)
			return 1
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	command := exec.Command(*binary, "-mode", "simulator", "-listen", listen)
	command.Env, command.Dir = env, *state
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		fmt.Fprintf(stderr, "Simulator start: %v\n", err)
		return 1
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	fmt.Fprintln(stdout, "Simulator is separately supervised; stopping it does not stop ZPR. Only explicitly approved scenario runs provide scenario evidence; background traffic does not.")
	select {
	case err := <-done:
		if err != nil {
			fmt.Fprintf(stderr, "Simulator exited: %v\n", err)
			return 1
		}
	case <-ctx.Done():
		if err := command.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			fmt.Fprintf(stderr, "Simulator shutdown: %v\n", err)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			fmt.Fprintln(stderr, "Simulator did not stop within 10 seconds; terminating its process")
			if err := command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				fmt.Fprintf(stderr, "Simulator termination: %v\n", err)
			}
			<-done
			return 1
		}
	}
	return 0
}

func lockEvaluationSimulator(evaluation string) (*os.File, error) {
	lock, err := os.OpenFile(filepath.Join(evaluation, "simulator-runtime.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, errors.New("cannot open paired Simulator lifecycle lock")
	}
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		lock.Close()
		return nil, errors.New("paired Simulator lifecycle lock is unsafe")
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("paired Simulator is running or administrator recovery is active")
	}
	return lock, nil
}

func validateEvaluationSimulatorArguments(evaluation, state, binary, image string) error {
	if _, err := setupmode.Parse(os.Getenv(setupmode.Environment)); err != nil {
		return err
	}
	for _, path := range []string{evaluation, state, binary} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
			return errors.New("evaluation, Simulator state and binary require canonical absolute non-root paths")
		}
		resolved, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil || resolved != filepath.Dir(path) {
			return errors.New("paths must have existing non-symlink parents")
		}
	}
	if state == evaluation || strings.HasPrefix(state, evaluation+string(filepath.Separator)) ||
		strings.HasPrefix(evaluation, state+string(filepath.Separator)) {
		return errors.New("Simulator state must be separate from evaluation state")
	}
	info, err := os.Lstat(evaluation)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return errors.New("evaluation state must be an existing real private directory")
	}
	info, err = os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Mode().Perm()&0o111 == 0 {
		return errors.New("Simulator binary must be a nonempty regular executable")
	}
	if !immutableImage.MatchString(image) || len(image) != len("sha256:")+64 || image[:7] != "sha256:" {
		return errors.New("an exact data-plane image ID is required")
	}
	return nil
}

func prepareEvaluationSimulator(evaluation, state, binary, image, retainedFile string) ([]string, string, error) {
	setupMode, setupModeSet := os.LookupEnv(setupmode.Environment)
	if err := validateEvaluationSimulatorArguments(evaluation, state, binary, image); err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(filepath.Join(evaluation, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil {
		return nil, "", errors.New("evaluation inventory unavailable")
	}
	if err := inventory.Config.validate(); err != nil {
		return nil, "", fmt.Errorf("invalid evaluation inventory: %w", err)
	}
	if _, err := readEvaluation(evaluation, inventory.Config); err != nil {
		return nil, "", err
	}
	if inventory.Config.SimulatorPort == 0 {
		return nil, "", errors.New("register the Simulator client first with -evaluation-simulator-pairing (or create the install with -simulator-port); existing state is never implicitly migrated")
	}
	// Without bootstrap-admitted endpoints (X15) the install-owned data plane
	// must exist so the published topology stays real; nothing is fabricated.
	if inventory.Config.DataPlaneEndpoints == 0 && !inventory.Config.DataPlane {
		return nil, "", errors.New("Simulator launch requires the install-owned data plane or bootstrap-admitted evaluation endpoints")
	}
	data, err = os.ReadFile(filepath.Join(evaluation, "simulator-oidc.json"))
	var login operatorauth.Config
	if err != nil || json.Unmarshal(data, &login) != nil || login.Validate() != nil ||
		login.ClientID != "zpr-simulator" || login.Issuer != inventory.Config.browserOrigin(inventory.Config.OIDCPort) ||
		login.RedirectURL != inventory.Config.browserOrigin(inventory.Config.SimulatorPort)+"/auth/operator/callback" {
		return nil, "", errors.New("verified Simulator identity configuration unavailable; wait for ZPR readiness")
	}
	organization := evaluationOrganization(inventory)
	// An operator directory is supported only when subject links are
	// configured: that is what seeds the reserved uid=evaluation-subject
	// health identity under the operator base DN (reserved at import), so the
	// published health_uid below remains truthful.
	if inventory.Config.Directory != nil && !inventory.Config.subjectLinks() {
		return nil, "", errors.New("an operator-directory install requires subject links for its seeded health identity; the profile health_uid would not exist")
	}
	for _, grant := range login.Grants {
		if !reflect.DeepEqual(grant.Organizations, []string{organization}) {
			return nil, "", errors.New("Simulator grants do not match the paired organization")
		}
	}
	if info, err := os.Lstat(state); errors.Is(err, os.ErrNotExist) {
		parent, err := os.Stat(filepath.Dir(state))
		if err != nil || parent.Mode().Perm()&0o022 != 0 {
			return nil, "", errors.New("Simulator state parent must be protected")
		}
		if err := os.Mkdir(state, 0o700); err != nil {
			return nil, "", err
		}
	} else if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		return nil, "", errors.New("Simulator state must be a real private directory")
	}
	for _, directory := range []string{"organizations", "scenarios", "workspace"} {
		path := filepath.Join(state, directory)
		if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			return nil, "", errors.New("Simulator subdirectory is unsafe")
		}
	}
	write := func(name string, value any) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(state, name))
		if err == nil && (!info.Mode().IsRegular() || info.Mode().Perm() != 0o600) {
			return errors.New("Simulator configuration must be a private regular file")
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return writeEvaluationOnce(state, name, append(data, '\n'))
	}
	if err := write("pairing.json", map[string]string{
		"evaluation_install_id": inventory.InstallID, "data_plane_image": image,
	}); err != nil {
		return nil, "", err
	}
	var machines []map[string]any
	for index := 1; index <= 20; index++ {
		machines = append(machines, map[string]any{
			"id": fmt.Sprintf("machine-%02d", index), "type": "desktop",
			"model": "Unprovisioned scenario fixture", "location": "Not provisioned", "secure": false,
		})
	}
	manifestName := "Evaluation endpoint traffic (scenario machines not provisioned)"
	profileDescription := "Live admitted evaluation endpoints; scenario machine provisioning is not configured."
	if inventory.Config.DataPlaneEndpoints == 0 {
		manifestName = "Evaluation install without bootstrap-admitted endpoints (scenario machines not provisioned)"
		profileDescription = "No bootstrap-admitted evaluation endpoints; runtime endpoints are managed outside this install. Scenario machine provisioning is not configured."
	}
	if err := write("manifest.json", map[string]any{
		"name":            manifestName,
		"organization_id": organization, "machines": machines,
		"agents": []any{}, "services": []any{}, "components": []any{},
	}); err != nil {
		return nil, "", err
	}
	var nodes []map[string]string
	for _, node := range evaluationDataPlaneTopology(inventory) {
		substrate := node.Substrate
		if substrate == "" {
			substrate = "10.0.0.1"
		}
		nodes = append(nodes, map[string]string{
			"id": node.Name, "location": "Evaluation host", "substrate_address": substrate, "zpr_address": node.ZPR,
		})
	}
	topology := "single-node"
	if len(nodes) > 1 {
		topology = "multi-node"
	}
	profile := map[string]any{
		"id": organization, "name": "ZPR Evaluation",
		"description": profileDescription,
		"runtime":     map[string]any{"driver": "docker-multinode", "topology": topology, "nodes": nodes},
		"directory": map[string]any{
			"base_dn": inventory.Config.directoryBaseDN(), "seed_mode": "ldif", "ldif_files": []string{"evaluation.ldif"},
			"health_uid": "evaluation-subject", "identity_mappings": map[string]string{"device.zpr.adapter.cn": "uid"},
			"attributes": []string{"uid"}, "people": []any{}, "groups": []any{}, "departments": []any{},
		}, "policies": []any{}, "services": []any{},
	}
	if inventory.Config.ScenarioDemo {
		tlsConfig, err := evaluationTLS(evaluation, false)
		if err != nil {
			return nil, "", err
		}
		transport := &http.Transport{TLSClientConfig: tlsConfig}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport, Timeout: 3 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		key, err := os.ReadFile(filepath.Join(evaluation, "vs-admin-api.key"))
		if err != nil {
			return nil, "", err
		}
		policy, err := evaluationruntime.ReadPolicyDigest(context.Background(), client,
			fmt.Sprintf("https://localhost:%d", inventory.Config.VSAdminPort), strings.TrimSpace(string(key)))
		if err != nil {
			return nil, "", err
		}
		handoff, err := os.ReadFile(filepath.Join(evaluation, evaluationDataPlaneEndpointsFile))
		if err != nil {
			return nil, "", err
		}
		profile["evaluation_runtime"] = evaluationruntime.Contract{Version: 1, InstallID: inventory.InstallID,
			ImageID: image, HandoffDigest: evaluationruntime.Digest(handoff), PolicyDigest: policy,
			EndpointCount: inventory.Config.DataPlaneEndpoints, ListenerCount: len(evaluationDataPlaneListeners)}
		var topologyNodes []map[string]string
		for _, node := range nodes {
			topologyNodes = append(topologyNodes, map[string]string{
				"id": node["id"], "zpr_address": node["zpr_address"], "substrate": node["substrate_address"],
			})
		}
		if err := write(filepath.Join("scenarios", "evaluation-endpoint-access.json"), map[string]any{
			"id": "evaluation-endpoint-access", "organization_id": organization, "name": "Verify endpoint access policy",
			"description": "Fresh bounded peer-access probes with healthy denied listeners and matching live TCP/UDP Visa denials. No VS reachability claim, machine provisioning or policy changes.",
			"topology":    map[string]any{"nodes": topologyNodes, "links": []any{}},
			"steps":       []map[string]any{{"action": "verify_evaluation_peer_access", "timeout_seconds": 120}},
		}); err != nil {
			return nil, "", err
		}
	}
	if err := write(filepath.Join("organizations", organization+".json"), profile); err != nil {
		return nil, "", err
	}
	target := map[string]any{
		"organization_id":      organization,
		"control_room_url":     inventory.Config.browserOrigin(inventory.Config.RoomPort),
		"operator_service_url": fmt.Sprintf("https://localhost:%d", inventory.Config.ServicePort),
		"client_cert_file":     filepath.Join(evaluation, "simulator-client-chain.pem"),
		"client_key_file":      filepath.Join(evaluation, "simulator-client-key.pem"),
		"service_ca_file":      filepath.Join(evaluation, "ca.pem"),
	}
	if inventory.Config.DataPlaneEndpoints != 0 {
		target["data_plane_endpoints_file"] = filepath.Join(evaluation, evaluationDataPlaneEndpointsFile)
	}
	if inventory.Config.ScenarioDemo {
		target["visa_admin_url"] = fmt.Sprintf("https://localhost:%d", inventory.Config.VSAdminPort)
		target["visa_admin_key_file"] = filepath.Join(evaluation, "vs-admin-api.key")
	}
	registryName := "installs.json"
	if retainedFile != "" {
		binding, runtime, err := loadEvaluationSimulatorRetainedBindings(retainedFile, inventory)
		if err != nil {
			return nil, "", err
		}
		target["retained_topology_binding"] = binding
		target["retained_machine_runtime"] = runtime
		// The retained-augmented registry is its own frozen artifact so the
		// original verified registry is never rewritten or silently rebound.
		registryName = "installs-retained.json"
	}
	if err := write(registryName, map[string]any{
		"version": 1, "kind": "zpr-simulator-evaluation-installs", "evaluation_only": true, "installs": []any{target},
	}); err != nil {
		return nil, "", err
	}
	env := []string{}
	for _, name := range []string{"PATH", "HOME", "TMPDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	values := map[string]string{
		"SIMULATION_MANIFEST":                     filepath.Join(state, "manifest.json"),
		"SIMULATION_ORGANIZATIONS_DIR":            filepath.Join(state, "organizations"),
		"SIMULATION_SCENARIOS_DIR":                filepath.Join(state, "scenarios"),
		"SIMULATION_WORKSPACE_DB_DIR":             filepath.Join(state, "workspace"),
		"SIMULATOR_EVALUATION_INSTALLS_FILE":      filepath.Join(state, registryName),
		"SIMULATOR_DATA_PLANE_IMAGE":              image,
		"ZPR_SIMULATOR_OPERATOR_CERT_FILE":        filepath.Join(evaluation, "server-chain.pem"),
		"ZPR_SIMULATOR_OPERATOR_KEY_FILE":         filepath.Join(evaluation, "server-key.pem"),
		"ZPR_SIMULATOR_OPERATOR_ORIGIN":           inventory.Config.browserOrigin(inventory.Config.SimulatorPort),
		"ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE": filepath.Join(evaluation, "simulator-oidc.json"),
		"ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE": filepath.Join(evaluation, "oidc-simulator-client.secret"),
		"ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE":     filepath.Join(evaluation, "ca.pem"),
	}
	if setupModeSet {
		values[setupmode.Environment] = setupMode
	}
	// Every paired install carries the independent Simulator delegation
	// signer; authorized operator calls remain gated by the named session
	// grants and each backend's own delegation trust.
	values["ZPR_OPERATOR_DELEGATION_SIGNER_FILE"] = filepath.Join(evaluation, "simulator-delegation-signer.json")
	values["ZPR_OPERATOR_DELEGATION_KEY_FILE"] = filepath.Join(evaluation, "simulator-delegation-key.pem")
	values["ZPR_OPERATOR_DELEGATION_AUDIENCE"] = evaluationServiceAudience(inventory.Config)
	if inventory.Config.ScenarioDemo {
		values["SIMULATOR_DATA_PLANE_WORKLOAD_INTERVAL"] = "off"
		values["ZPR_BUDGET_AUTHORITY_URL"] = fmt.Sprintf("https://localhost:%d", inventory.Config.ServicePort)
		values["ZPR_BUDGET_AUTHORITY_CLIENT_CERT_FILE"] = filepath.Join(evaluation, "simulator-client-chain.pem")
		values["ZPR_BUDGET_AUTHORITY_CLIENT_KEY_FILE"] = filepath.Join(evaluation, "simulator-client-key.pem")
		values["ZPR_BUDGET_AUTHORITY_CA_FILE"] = filepath.Join(evaluation, "ca.pem")
	}
	if inventory.Config.BrowserIP != "" {
		values["ZPR_SIMULATOR_OPERATOR_DIRECT_IP"] = inventory.Config.BrowserIP
	}
	for name, value := range values {
		env = append(env, name+"="+value)
	}
	host := inventory.Config.BrowserIP
	if host == "" {
		host = "127.0.0.1"
	}
	return env, net.JoinHostPort(host, strconv.Itoa(inventory.Config.SimulatorPort)), nil
}
