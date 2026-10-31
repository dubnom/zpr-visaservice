//go:build !production

package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

type simulatorRetainedMachineRuntime struct {
	Version          int    `json:"version"`
	ImageID          string `json:"image_id"`
	ControllerFile   string `json:"controller_file"`
	ControllerSHA256 string `json:"controller_sha256"`
	EndpointRoot     string `json:"endpoint_root"`
	DataPlaneCAFile  string `json:"data_plane_ca_file"`
	ControlTLSDir    string `json:"control_tls_dir"`
	CallbackUpstream string `json:"callback_upstream"`
}

const retainedCallbackAddress = "fd5a:5052:adda:1:ffff:ffff:ffff:fffe"

var retainedMachineLifecycleMu sync.Mutex

func validateSimulatorRetainedMachineRuntime(organization simulatorOrganization) error {
	runtime := organization.RetainedMachineRuntime
	if runtime == nil {
		return nil
	}
	if organization.RetainedTopologyBinding == nil || runtime.Version != 1 ||
		!validSimulatorScenarioAuthorizationScopeDigest(runtime.ImageID) ||
		!validOrganizationPolicyDigest(runtime.ControllerSHA256) {
		return errors.New("retained machine runtime requires version 1, an explicit topology binding and exact image/controller digests")
	}
	if err := validateSimulatorRetainedTopology(organization); err != nil {
		return err
	}
	for _, path := range []string{runtime.ControllerFile, runtime.EndpointRoot, runtime.DataPlaneCAFile, runtime.ControlTLSDir} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",:\x00\r\n") {
			return errors.New("retained machine runtime requires canonical absolute bind paths without commas, colons or line breaks")
		}
	}
	host, port, err := net.SplitHostPort(runtime.CallbackUpstream)
	ip, parseErr := netip.ParseAddr(host)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || parseErr != nil || !ip.Is4() || !ip.IsPrivate() || portErr != nil || portNumber < 1024 || portNumber > 65535 {
		return errors.New("retained callback upstream must be an explicit private IPv4 TLS listener and unprivileged port")
	}
	return nil
}

func retainedEndpointFiles(runtime simulatorRetainedMachineRuntime, asset string) []string {
	directory := filepath.Join(runtime.EndpointRoot, asset, "identity")
	return []string{
		filepath.Join(directory, "runtime-auth.toml"),
		filepath.Join(directory, "runtime-auth.key"),
		filepath.Join(directory, "runtime-auth-ca.pem"),
	}
}

func verifySimulatorRetainedMachineFiles(organization simulatorOrganization) error {
	if organization.RetainedMachineRuntime == nil {
		return errors.New("retained machine lifecycle consumer is not configured")
	}
	if err := validateSimulatorRetainedMachineRuntime(organization); err != nil {
		return err
	}
	runtime := *organization.RetainedMachineRuntime
	file, err := os.Open(runtime.ControllerFile)
	if err != nil {
		return fmt.Errorf("open retained controller executable: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return errors.New("retained controller must be a regular executable not writable by group or others")
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash retained controller: %w", err)
	}
	if hex.EncodeToString(hash.Sum(nil)) != runtime.ControllerSHA256 {
		return errors.New("retained controller executable digest does not match the reviewed runtime")
	}
	ca, err := readOperatorFile(runtime.DataPlaneCAFile, 65536, false)
	if err != nil || !x509.NewCertPool().AppendCertsFromPEM(ca) {
		return errors.New("retained data-plane CA is unavailable or invalid")
	}
	for _, asset := range []string{
		"velocity-machine-03-controller", "velocity-machine-03-finance-client",
		"velocity-machine-05-controller", "velocity-machine-05-echo-service", "velocity-simulator-control",
	} {
		files := retainedEndpointFiles(runtime, asset)
		if _, err := machinePHAuthenticationArguments("", files[0]); err != nil {
			return fmt.Errorf("retained asset %s: %w", asset, err)
		}
		data, err := readOperatorFile(files[0], 16384, true)
		if err != nil {
			return err
		}
		var config struct {
			Authentication struct {
				ClientID string `toml:"bas_client_id"`
				Key      string `toml:"bas_key"`
				CA       string `toml:"tls_ca_file"`
			}
		}
		if _, err := toml.Decode(string(data), &config); err != nil {
			return err
		}
		if config.Authentication.ClientID != asset || config.Authentication.Key != files[1] || config.Authentication.CA != files[2] {
			return fmt.Errorf("retained asset %s configuration must reference only its exact registry identity and runtime files", asset)
		}
	}
	controlCA, err := readOperatorFile(filepath.Join(runtime.ControlTLSDir, "control-ca.crt"), 65536, false)
	if err != nil {
		return fmt.Errorf("read retained machine-control CA: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(controlCA) {
		return errors.New("retained machine-control CA is invalid")
	}
	for _, machine := range []string{"machine-03", "machine-05"} {
		certificate, err := tls.LoadX509KeyPair(filepath.Join(runtime.ControlTLSDir, machine, "client.crt"), filepath.Join(runtime.ControlTLSDir, machine, "client.key"))
		if err != nil {
			return fmt.Errorf("load retained %s control identity: %w", machine, err)
		}
		leaf, err := x509.ParseCertificate(certificate.Certificate[0])
		if err != nil {
			return err
		}
		if leaf.Subject.CommonName != machine {
			return fmt.Errorf("retained control certificate identity does not match %s", machine)
		}
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
			return fmt.Errorf("verify retained %s control certificate: %w", machine, err)
		}
	}
	return nil
}

type retainedContainerState struct {
	ID, Image string
	Config    struct {
		Labels map[string]string
	}
	State struct{ Running bool }
}

func retainedRuntimeDigest(organization simulatorOrganization, scope machineControlScope) string {
	data, _ := json.Marshal(struct {
		Binding *simulatorRetainedTopologyBinding
		Runtime *simulatorRetainedMachineRuntime
		Scope   machineControlScope
	}{organization.RetainedTopologyBinding, organization.RetainedMachineRuntime, scope})
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func retainedInspectContainer(ctx context.Context, name string, run func(context.Context, string, ...string) (string, error)) (*retainedContainerState, error) {
	output, err := run(ctx, "docker", "container", "ls", "--all", "--filter", "name=^/"+name+"$", "--format", "{{.ID}}")
	if err != nil {
		return nil, fmt.Errorf("find retained container: %w", err)
	}
	id := strings.TrimSpace(output)
	if id == "" {
		return nil, nil
	}
	if strings.ContainsAny(id, " \t\r\n") {
		return nil, errors.New("retained container lookup is ambiguous")
	}
	output, err = run(ctx, "docker", "inspect", "--type", "container", id)
	if err != nil {
		return nil, err
	}
	var containers []retainedContainerState
	if err := json.Unmarshal([]byte(output), &containers); err != nil || len(containers) != 1 || containers[0].ID == "" {
		return nil, errors.New("invalid retained container inspection")
	}
	return &containers[0], nil
}

func retainedContainerOwned(container *retainedContainerState, organization simulatorOrganization, component string, scope machineControlScope) bool {
	labels := container.Config.Labels
	return labels["org.zpr.evaluation-install"] == organization.RetainedTopologyBinding.InstallID &&
		labels["org.zpr.component"] == "simulator-retained-"+component &&
		labels["zpr.organization.id"] == organization.ID &&
		labels["zpr.runtime.generation"] == scope.Generation &&
		labels["org.zpr.runtime-digest"] == retainedRuntimeDigest(organization, scope)
}

func retainedContainerArguments(organization simulatorOrganization, machine string, scope machineControlScope) []string {
	runtime, binding := *organization.RetainedMachineRuntime, *organization.RetainedTopologyBinding
	component, name := machine, machineContainerName(machine)
	if machine == "callback" {
		name = "zpr-simulator-control-" + binding.InstallID
	}
	args := []string{"run", "--detach", "--init", "--name", name,
		"--network", binding.Network, "--user", "0:0", "--read-only",
		"--cap-drop", "ALL", "--cap-add", "NET_ADMIN", "--cap-add", "NET_RAW", "--cap-add", "DAC_OVERRIDE",
		"--security-opt", "no-new-privileges", "--device", "/dev/net/tun",
		"--tmpfs", "/run:rw,nosuid,nodev,size=32m", "--tmpfs", "/tmp:rw,nosuid,nodev,size=64m",
		"--tmpfs", "/opt/zpr-workloads:rw,nosuid,nodev,size=1m",
		"--tmpfs", runtime.EndpointRoot + ":rw,nosuid,nodev,noexec,size=1m,mode=0700",
		"--label", "org.zpr.evaluation-install=" + binding.InstallID,
		"--label", "org.zpr.component=simulator-retained-" + component,
		"--label", "org.zpr.runtime-digest=" + retainedRuntimeDigest(organization, scope),
		"--label", "zpr.organization.id=" + organization.ID,
		"--label", "zpr.runtime.generation=" + scope.Generation,
		"--env", "SIMULATION_ORGANIZATION_ID=" + organization.ID,
		"--env", "SIMULATION_RUNTIME_GENERATION=" + scope.Generation}
	mount := func(source, target string) {
		args = append(args, "--mount", "type=bind,source="+source+",target="+target+",readonly")
	}
	mount(runtime.ControllerFile, "/usr/local/bin/zpr-machine-controller")
	mount(runtime.DataPlaneCAFile, "/opt/zpr-workloads/ca.crt")
	assets := []string{"velocity-simulator-control"}
	workload := ""
	if machine != "callback" {
		workload = "finance-client"
		if machine == "machine-05" {
			workload = "echo-service"
		}
		assets = []string{"velocity-" + machine + "-controller", "velocity-" + machine + "-" + workload}
		args = append(args, "--label", "zpr.machine.id="+machine)
		mount(filepath.Join(runtime.ControlTLSDir, "control-ca.crt"), "/run/zpr-machine/control-ca.crt")
		mount(filepath.Join(runtime.ControlTLSDir, machine, "client.crt"), "/run/zpr-machine/client.crt")
		mount(filepath.Join(runtime.ControlTLSDir, machine, "client.key"), "/run/zpr-machine/client.key")
	}
	for _, asset := range assets {
		for _, path := range retainedEndpointFiles(runtime, asset) {
			mount(path, filepath.Join("/run/zpr-runtime-input", asset, filepath.Base(path)))
		}
	}
	config := retainedEndpointFiles(runtime, assets[0])[0]
	args = append(args, "--env", "SIMULATION_RETAINED_ENDPOINT_ROOT="+runtime.EndpointRoot,
		"--env", "SIMULATION_RETAINED_ASSETS="+strings.Join(assets, " "))
	if machine == "callback" {
		args = append(args, "--env", "SIMULATION_RETAINED_CALLBACK_NODE="+binding.SubstrateAddress+":5000")
	}
	args = append(args, "--env", "SIMULATION_MACHINE_AUTHENTICATION_CONFIG="+config,
		"--entrypoint", "/bin/sh", runtime.ImageID)
	script := `set -eu
mkdir -p /run/zpr-workloads /opt/zpr-workloads/authentication
for asset in $SIMULATION_RETAINED_ASSETS; do
    directory="$SIMULATION_RETAINED_ENDPOINT_ROOT/$asset/identity"
    mkdir -p "$directory"
    chmod 700 "$directory"
    for file in runtime-auth.toml runtime-auth.key runtime-auth-ca.pem; do
        cp "/run/zpr-runtime-input/$asset/$file" "$directory/$file"
        chmod 600 "$directory/$file"
    done
done
ln -s /usr/local/bin/ph /opt/zpr-workloads/ph
ln -s /usr/local/bin/ph-cli /opt/zpr-workloads/ph-cli
for interface in tun0 tun3 tun5; do
    ip tuntap add name "$interface" mode tun multi_queue
    ip link set "$interface" up
done
ip -6 route replace fd5a:5052::/32 dev tun5
`
	if machine == "callback" {
		args = append(args, "-c", script+`exec /usr/local/bin/zpr-machine-controller "$@"`, "retained-callback",
			"-mode", "machine-control-proxy", "-proxy-listen", "["+retainedCallbackAddress+"]:8792",
			"-proxy-upstream", runtime.CallbackUpstream)
	} else {
		script += `cp "$1" "/opt/zpr-workloads/authentication/$2.toml"
chmod 600 "/opt/zpr-workloads/authentication/$2.toml"
shift 2
exec /usr/local/bin/zpr-machine-controller "$@"`
		args = append(args, "-c", script, "retained-machine",
			retainedEndpointFiles(runtime, assets[1])[0], workload,
			"-mode", "machine-controller", "-machine-id", machine,
			"-control-url", "https://["+retainedCallbackAddress+"]:8792",
			"-control-ca", "/run/zpr-machine/control-ca.crt", "-client-cert", "/run/zpr-machine/client.crt",
			"-client-key", "/run/zpr-machine/client.key", "-zpr-ph", "/opt/zpr-workloads/ph",
			"-zpr-node-addr", binding.SubstrateAddress+":5000")
	}
	return args
}

func waitForRetainedScenarioDNSRecord(ctx context.Context, binding simulatorRetainedTopologyBinding, server, name, address string,
	run func(context.Context, string, ...string) (string, error)) error {
	expected := net.ParseIP(address)
	resolver := net.ParseIP(server)
	if expected == nil || expected.To4() != nil || resolver == nil || resolver.To4() != nil {
		return errors.New("retained service publication requires explicit IPv6 service and DNS addresses")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		output, err := run(ctx, "docker", "exec", binding.NodeContainer, "ip", "netns", "exec", "zpr-dns",
			"dig", "+tcp", "+time=1", "+tries=1", "+short", "AAAA", "@"+server, name)
		if err != nil {
			return fmt.Errorf("read production-owned DNS publication for %s: %w", name, err)
		}
		for _, record := range strings.Fields(output) {
			if observed := net.ParseIP(record); observed != nil && observed.Equal(expected) {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("production-owned DNS has not published %s at %s: %w", name, address, ctx.Err())
		case <-ticker.C:
		}
	}
}

func retainedMachineAction(ctx context.Context, organization simulatorOrganization, scope machineControlScope, machine, action string,
	run func(context.Context, string, ...string) (string, error)) (string, error) {
	if machine != "machine-03" && machine != "machine-05" && machine != "callback" || action != "start" && action != "stop" {
		return "", errors.New("unsupported retained machine lifecycle operation")
	}

	if err := validateSimulatorRetainedMachineRuntime(organization); err != nil || organization.RetainedMachineRuntime == nil {
		return "", errors.New("retained machine lifecycle configuration is invalid")
	}
	if scope.OrganizationID != organization.ID || !validMachineControlGeneration(scope.Generation) {
		return "", errors.New("retained machine lifecycle scope is invalid")
	}
	name := machineContainerName(machine)
	if machine == "callback" {
		name = "zpr-simulator-control-" + organization.RetainedTopologyBinding.InstallID
	}
	container, err := retainedInspectContainer(ctx, name, run)
	if err != nil {
		return "", err
	}
	if container != nil && (!retainedContainerOwned(container, organization, machine, scope) || container.Image != organization.RetainedMachineRuntime.ImageID) {
		return "", errors.New("retained container belongs to another installation, runtime configuration or generation; explicit reconciliation is required")
	}
	if action == "stop" {
		if container == nil {
			return "already stopped", nil
		}
		if container.State.Running {
			if _, err := run(ctx, "docker", "stop", "--time", "8", container.ID); err != nil {
				return "", err
			}
		}
		if _, err := run(ctx, "docker", "rm", container.ID); err != nil {
			return "", err
		}
		return "stopped", nil
	}
	if container != nil {
		if container.State.Running {
			return "already running", nil
		}
		if _, err := run(ctx, "docker", "rm", container.ID); err != nil {
			return "", err
		}
	}
	output, err := run(ctx, "docker", retainedContainerArguments(organization, machine, scope)...)
	if err != nil {
		return output, err
	}
	container, err = retainedInspectContainer(ctx, name, run)
	if err != nil {
		return "", err
	}
	if container == nil || !container.State.Running || !retainedContainerOwned(container, organization, machine, scope) ||
		container.Image != organization.RetainedMachineRuntime.ImageID {
		return "", errors.New("retained container did not start with the expected ownership; inspect its scoped logs")
	}
	return "started", nil
}

func simulatorRetainedMachineAction(ctx context.Context, manifest simulatorManifest, machine, action string) (string, bool, error) {
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), manifest.OrganizationID)
	if err != nil {
		return "", false, err
	}
	if organization.RetainedTopologyBinding == nil {
		return "", false, nil
	}
	retainedMachineLifecycleMu.Lock()
	defer retainedMachineLifecycleMu.Unlock()
	scope, err := machineControlRuntimeScope(simulatorManifestPath())
	if err != nil {
		return "", true, err
	}
	if action == "start" {
		if err := verifySimulatorRetainedTopology(ctx, organization, scenarioCommand); err != nil {
			return "", true, err
		}
		if err := verifySimulatorRetainedMachineFiles(organization); err != nil {
			return "", true, err
		}
		if _, err := retainedMachineAction(ctx, organization, scope, "callback", "start", scenarioCommand); err != nil {
			return "", true, err
		}
	}
	output, err := retainedMachineAction(ctx, organization, scope, machine, action, scenarioCommand)
	if err != nil || action != "stop" {
		return output, true, err
	}
	for _, peer := range []string{"machine-03", "machine-05"} {
		container, err := retainedInspectContainer(ctx, machineContainerName(peer), scenarioCommand)
		if err != nil {
			return output, true, err
		}
		if container != nil {
			return output, true, nil
		}
	}
	_, err = retainedMachineAction(ctx, organization, scope, "callback", "stop", scenarioCommand)
	return output, true, err
}
