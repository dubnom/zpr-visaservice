package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

func runEvaluationSimulator(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("zpr-lab-bootstrap -evaluation-simulator", flag.ContinueOnError)
	flags.SetOutput(stderr)
	evaluation := flags.String("evaluation-state", "", "existing independently running evaluation with -simulator-port")
	state := flags.String("state", "", "separate private Simulator state directory")
	binary := flags.String("binary", "", "absolute prebuilt Simulator executable")
	image := flags.String("data-plane-image", "", "exact paired data-plane image ID")
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
	env, listen, err := prepareEvaluationSimulator(*evaluation, *state, *binary, *image)
	if err != nil {
		fmt.Fprintf(stderr, "Simulator setup: %v\n", err)
		return 1
	}
	lock, err := lockEvaluationSimulator(*evaluation)
	if err != nil {
		fmt.Fprintf(stderr, "Simulator lifecycle: %v\n", err)
		return 1
	}
	defer lock.Close()
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
	fmt.Fprintln(stdout, "Simulator is separately supervised; stopping it does not stop ZPR. Endpoint traffic is evaluation-only, not scenario acceptance.")
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

func prepareEvaluationSimulator(evaluation, state, binary, image string) ([]string, string, error) {
	for _, path := range []string{evaluation, state, binary} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
			return nil, "", errors.New("evaluation, Simulator state and binary require canonical absolute non-root paths")
		}
		resolved, err := filepath.EvalSymlinks(filepath.Dir(path))
		if err != nil || resolved != filepath.Dir(path) {
			return nil, "", errors.New("paths must have existing non-symlink parents")
		}
	}
	if state == evaluation || strings.HasPrefix(state, evaluation+string(filepath.Separator)) ||
		strings.HasPrefix(evaluation, state+string(filepath.Separator)) {
		return nil, "", errors.New("Simulator state must be separate from evaluation state")
	}
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Mode().Perm()&0o111 == 0 {
		return nil, "", errors.New("Simulator binary must be a nonempty regular executable")
	}
	if !immutableImage.MatchString(image) || len(image) != len("sha256:")+64 || image[:7] != "sha256:" {
		return nil, "", errors.New("an exact data-plane image ID is required")
	}
	data, err := os.ReadFile(filepath.Join(evaluation, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil {
		return nil, "", errors.New("evaluation inventory unavailable")
	}
	if _, err := readEvaluation(evaluation, inventory.Config); err != nil {
		return nil, "", err
	}
	if inventory.Config.SimulatorPort == 0 || inventory.Config.DataPlaneEndpoints == 0 {
		return nil, "", errors.New("create a separate evaluation with -simulator-port and -data-plane-endpoints; existing state cannot be implicitly migrated")
	}
	data, err = os.ReadFile(filepath.Join(evaluation, "simulator-oidc.json"))
	var login operatorauth.Config
	if err != nil || json.Unmarshal(data, &login) != nil || login.Validate() != nil ||
		login.ClientID != "zpr-simulator" || login.Issuer != inventory.Config.browserOrigin(inventory.Config.OIDCPort) ||
		login.RedirectURL != inventory.Config.browserOrigin(inventory.Config.SimulatorPort)+"/auth/operator/callback" {
		return nil, "", errors.New("verified Simulator identity configuration unavailable; wait for ZPR readiness")
	}
	organization := evaluationOrganization(inventory)
	if inventory.Config.Directory != nil {
		return nil, "", errors.New("automatic Simulator pairing currently supports the starter directory only")
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
	if err := write("manifest.json", map[string]any{
		"name":            "Evaluation endpoint traffic (scenario machines not provisioned)",
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
	if err := write(filepath.Join("organizations", organization+".json"), map[string]any{
		"id": organization, "name": "ZPR Evaluation",
		"description": "Live admitted evaluation endpoints; scenario machine provisioning is not configured.",
		"runtime":     map[string]any{"driver": "docker-multinode", "topology": topology, "nodes": nodes},
		"directory": map[string]any{
			"base_dn": inventory.Config.directoryBaseDN(), "seed_mode": "ldif", "ldif_files": []string{"evaluation.ldif"},
			"health_uid": "evaluation-subject", "identity_mappings": map[string]string{"device.zpr.adapter.cn": "uid"},
			"attributes": []string{"uid"}, "people": []any{}, "groups": []any{}, "departments": []any{},
		}, "policies": []any{}, "services": []any{},
	}); err != nil {
		return nil, "", err
	}
	target := map[string]any{
		"organization_id":           organization,
		"control_room_url":          inventory.Config.browserOrigin(inventory.Config.RoomPort),
		"operator_service_url":      fmt.Sprintf("https://localhost:%d", inventory.Config.ServicePort),
		"client_cert_file":          filepath.Join(evaluation, "client-chain.pem"),
		"client_key_file":           filepath.Join(evaluation, "client-key.pem"),
		"service_ca_file":           filepath.Join(evaluation, "ca.pem"),
		"data_plane_endpoints_file": filepath.Join(evaluation, evaluationDataPlaneEndpointsFile),
	}
	if err := write("installs.json", map[string]any{
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
		"SIMULATOR_EVALUATION_INSTALLS_FILE":      filepath.Join(state, "installs.json"),
		"SIMULATOR_DATA_PLANE_IMAGE":              image,
		"ZPR_SIMULATOR_OPERATOR_CERT_FILE":        filepath.Join(evaluation, "server-chain.pem"),
		"ZPR_SIMULATOR_OPERATOR_KEY_FILE":         filepath.Join(evaluation, "server-key.pem"),
		"ZPR_SIMULATOR_OPERATOR_ORIGIN":           inventory.Config.browserOrigin(inventory.Config.SimulatorPort),
		"ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE": filepath.Join(evaluation, "simulator-oidc.json"),
		"ZPR_SIMULATOR_OPERATOR_OIDC_SECRET_FILE": filepath.Join(evaluation, "oidc-simulator-client.secret"),
		"ZPR_SIMULATOR_OPERATOR_OIDC_CA_FILE":     filepath.Join(evaluation, "ca.pem"),
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
