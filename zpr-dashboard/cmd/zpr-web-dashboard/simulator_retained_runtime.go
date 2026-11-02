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
	// ControlDNSName optionally overrides the production-published DNS name
	// controllers resolve to reach the machine-control callback. It must be
	// a bounded name in the production svc.zpr zone and must match the
	// organization policy's SimulatorControlService provider alias.
	ControlDNSName string `json:"control_dns_name,omitempty"`
	// ControlAsset and Machines, when present, declare this organization's
	// exact retained inventory: the simulator-control callback identity and
	// each retained machine with its controller identity and single
	// workload. A binding without an explicit inventory keeps the exact
	// reviewed retained Velocity contract, so existing deployments are
	// unchanged.
	ControlAsset string                     `json:"control_asset,omitempty"`
	Machines     []simulatorRetainedMachine `json:"machines,omitempty"`
}

// simulatorRetainedMachine is one operator-declared retained machine: its
// manifest machine ID, the registry identities for its controller and
// workload, and the workload it runs.
type simulatorRetainedMachine struct {
	MachineID       string `json:"machine_id"`
	ControllerAsset string `json:"controller_asset"`
	Workload        string `json:"workload"`
	WorkloadAsset   string `json:"workload_asset"`
}

const maxSimulatorRetainedMachines = 8

// retainedInventory returns the effective control identity and machine
// inventory for this runtime binding.
func (runtime simulatorRetainedMachineRuntime) retainedInventory() (string, []simulatorRetainedMachine) {
	if len(runtime.Machines) == 0 {
		return "velocity-simulator-control", []simulatorRetainedMachine{
			{MachineID: "machine-03", ControllerAsset: "velocity-machine-03-controller",
				Workload: "finance-client", WorkloadAsset: "velocity-machine-03-finance-client"},
			{MachineID: "machine-05", ControllerAsset: "velocity-machine-05-controller",
				Workload: "echo-service", WorkloadAsset: "velocity-machine-05-echo-service"},
		}
	}
	return runtime.ControlAsset, runtime.Machines
}

// requiredAssets returns the exact registry identities this runtime binding
// requires; the retained lifecycle and the enrollment readiness comparison
// both use this inventory and never infer additional assets.
func (runtime simulatorRetainedMachineRuntime) requiredAssets() []string {
	control, machines := runtime.retainedInventory()
	assets := make([]string, 0, 2*len(machines)+1)
	for _, machine := range machines {
		assets = append(assets, machine.ControllerAsset, machine.WorkloadAsset)
	}
	return append(assets, control)
}

func (runtime simulatorRetainedMachineRuntime) retainedMachine(machineID string) (simulatorRetainedMachine, bool) {
	_, machines := runtime.retainedInventory()
	for _, machine := range machines {
		if machine.MachineID == machineID {
			return machine, true
		}
	}
	return simulatorRetainedMachine{}, false
}

func (runtime simulatorRetainedMachineRuntime) retainedMachineIDs() []string {
	_, machines := runtime.retainedInventory()
	ids := make([]string, 0, len(machines))
	for _, machine := range machines {
		ids = append(ids, machine.MachineID)
	}
	return ids
}

// validateSimulatorRetainedInventory enforces the explicit inventory
// contract: all-or-nothing declaration, complete bounded identities, unique
// machines and assets, and only workloads with a maintained machine runtime
// configuration.
func validateSimulatorRetainedInventory(runtime simulatorRetainedMachineRuntime) error {
	if len(runtime.Machines) == 0 {
		if runtime.ControlAsset != "" {
			return errors.New("a retained control asset requires an explicit machine inventory")
		}
		return nil
	}
	if runtime.ControlAsset == "" {
		return errors.New("an explicit retained machine inventory requires its control asset")
	}
	if len(runtime.Machines) > maxSimulatorRetainedMachines {
		return fmt.Errorf("at most %d retained machines are supported", maxSimulatorRetainedMachines)
	}
	assets := map[string]struct{}{runtime.ControlAsset: {}}
	machines := map[string]struct{}{}
	for _, machine := range runtime.Machines {
		for _, value := range []string{machine.MachineID, machine.ControllerAsset, machine.Workload, machine.WorkloadAsset} {
			if !validSimulatorDeclaredEndpointText(value) {
				return fmt.Errorf("retained machine %q requires complete bounded identities", machine.MachineID)
			}
		}
		if _, exists := machines[machine.MachineID]; exists {
			return fmt.Errorf("retained machine %q is listed more than once", machine.MachineID)
		}
		machines[machine.MachineID] = struct{}{}
		for _, asset := range []string{machine.ControllerAsset, machine.WorkloadAsset} {
			if _, exists := assets[asset]; exists {
				return fmt.Errorf("retained asset %q is bound more than once", asset)
			}
			assets[asset] = struct{}{}
		}
		if _, configured := machineWorkload(machine.Workload); !configured {
			return fmt.Errorf("retained machine %q workload %q has no maintained runtime configuration", machine.MachineID, machine.Workload)
		}
	}
	return nil
}

// defaultRetainedControlDNSName is the production-published service name for
// the retained machine-control callback: organization policy provides
// SimulatorControlService at this name and the production Visa Service
// publishes the callback adapter's granted address under it. The callback
// address itself is always dynamic — policy matches the authenticated
// adapter identity, and trust in the control channel comes exclusively from
// its pinned mTLS identities, never from an address.
const defaultRetainedControlDNSName = "simulator-control.svc.zpr"

// retainedControlAddressFile is where the retained callback records the ZPR
// address the Visa Service actually granted to its adapter.
const retainedControlAddressFile = "/run/zpr-workloads/control-address"

// controlDNSName returns the production DNS name for the retained
// machine-control service.
func (runtime simulatorRetainedMachineRuntime) controlDNSName() string {
	if runtime.ControlDNSName == "" {
		return defaultRetainedControlDNSName
	}
	return runtime.ControlDNSName
}

// simulatorRetainedRequiredAssets is the exact reviewed retained Velocity
// machine-runtime endpoint inventory — the default inventory for bindings
// without an explicit machine declaration — and its names stay reserved
// against profile-declared endpoints.
var simulatorRetainedRequiredAssets = []string{
	"velocity-machine-03-controller", "velocity-machine-03-finance-client",
	"velocity-machine-05-controller", "velocity-machine-05-echo-service", "velocity-simulator-control",
}

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
	if name := runtime.ControlDNSName; name != "" && (!validScenarioDNSName(name) || !strings.HasSuffix(name, ".svc.zpr")) {
		return errors.New("retained control DNS name must be a bounded name in the production svc.zpr zone")
	}
	return validateSimulatorRetainedInventory(*runtime)
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
	for _, asset := range runtime.requiredAssets() {
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
	for _, machine := range runtime.retainedMachineIDs() {
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
		// Control pins the machine-control rendezvous contract revision so
		// containers started with the superseded fixed-address arguments are
		// refused instead of silently reused.
		Control string
	}{organization.RetainedTopologyBinding, organization.RetainedMachineRuntime, scope, "resolved-dock-address-v3"})
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

// resolveRetainedDockAddress returns the docking node's current substrate
// address for machine and callback containers. Multi-node bindings carry the
// install's deterministic address (compiled policy embeds it); single-node
// bindings pin identity only, so the dynamic address is resolved live from
// the verified docking container on the binding's own network.
func resolveRetainedDockAddress(
	ctx context.Context,
	organization simulatorOrganization,
	run func(context.Context, string, ...string) (string, error),
) (string, error) {
	binding := *organization.RetainedTopologyBinding
	docking := binding.dockingNode()
	if docking.SubstrateAddress != "" {
		return docking.SubstrateAddress, nil
	}
	output, err := run(ctx, "docker", "inspect", "--type", "container", docking.Container)
	if err != nil {
		return "", fmt.Errorf("resolve retained dock address: %w", err)
	}
	var containers []struct {
		Image           string
		State           struct{ Running bool }
		Config          struct{ Labels map[string]string }
		NetworkSettings struct {
			Networks map[string]struct{ IPAddress string }
		}
	}
	if json.Unmarshal([]byte(output), &containers) != nil || len(containers) != 1 {
		return "", errors.New("resolve retained dock address: invalid docking container inspection")
	}
	container := containers[0]
	address := container.NetworkSettings.Networks[binding.Network].IPAddress
	if !container.State.Running || container.Image != binding.ImageID ||
		container.Config.Labels["org.zpr.evaluation-install"] != binding.InstallID || address == "" {
		return "", errors.New("docking container ownership, image or network attachment does not match the binding; refusing to resolve its dock address")
	}
	return address, nil
}

func retainedContainerArguments(organization simulatorOrganization, machine string, scope machineControlScope, dnsServer, dockAddress string) []string {
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
	controlAsset, _ := runtime.retainedInventory()
	assets := []string{controlAsset}
	workload := ""
	if machine != "callback" {
		retained, _ := runtime.retainedMachine(machine)
		workload = retained.Workload
		assets = []string{retained.ControllerAsset, retained.WorkloadAsset}
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
		args = append(args, "--env", "SIMULATION_RETAINED_CALLBACK_NODE="+dockAddress+":5000")
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
			"-mode", "machine-control-proxy", "-proxy-upstream", runtime.CallbackUpstream)
	} else {
		script += `cp "$1" "/opt/zpr-workloads/authentication/$2.toml"
chmod 600 "/opt/zpr-workloads/authentication/$2.toml"
shift 2
exec /usr/local/bin/zpr-machine-controller "$@"`
		args = append(args, "-c", script, "retained-machine",
			retainedEndpointFiles(runtime, assets[1])[0], workload,
			"-mode", "machine-controller", "-machine-id", machine,
			"-control-url", "https://"+runtime.controlDNSName()+":8792",
			"-control-dns", dnsServer,
			"-control-ca", "/run/zpr-machine/control-ca.crt", "-client-cert", "/run/zpr-machine/client.crt",
			"-client-key", "/run/zpr-machine/client.key", "-zpr-ph", "/opt/zpr-workloads/ph",
			"-zpr-node-addr", dockAddress+":5000")
	}
	return args
}

// waitForRetainedControlPublication waits for the retained callback to
// record the ZPR address the Visa Service granted to its adapter, then for
// the production Visa Service to publish the control service name at exactly
// that address. It fails closed with explicit evidence when the installed
// data plane does not publish the name, instead of letting controller waits
// time out without a cause.
func waitForRetainedControlPublication(ctx context.Context, organization simulatorOrganization, dnsServer string,
	run func(context.Context, string, ...string) (string, error)) error {
	if organization.RetainedMachineRuntime == nil || organization.RetainedTopologyBinding == nil {
		return errors.New("retained control publication requires the runtime and topology bindings")
	}
	runtime, binding := *organization.RetainedMachineRuntime, *organization.RetainedTopologyBinding
	container := "zpr-simulator-control-" + binding.InstallID
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	address := ""
	for address == "" {
		output, err := run(waitCtx, "docker", "exec", container, "cat", retainedControlAddressFile)
		if err == nil {
			granted, parseErr := netip.ParseAddr(strings.TrimSpace(output))
			if parseErr != nil || !netip.MustParsePrefix("fd5a:5052::/32").Contains(granted) {
				return fmt.Errorf("retained callback recorded an invalid granted control address %q", strings.TrimSpace(output))
			}
			address = granted.String()
			break
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("retained callback has not recorded its granted control address: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
	name := runtime.controlDNSName() + "."
	if err := waitForRetainedScenarioDNSRecord(waitCtx, binding, dnsServer, name, address, run); err != nil {
		return fmt.Errorf("the production Visa Service has not published %s at the callback's granted address %s; verify the organization policy provides SimulatorControlService at that name and that the installed data plane publishes service DNS records: %w", name, address, err)
	}
	return nil
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
		output, err := run(ctx, "docker", "exec", binding.dockingNode().Container, "ip", "netns", "exec", "zpr-dns",
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

func isSimulatorRetainedMachine(runtime *simulatorRetainedMachineRuntime, machine string) bool {
	if runtime == nil {
		return false
	}
	_, retained := runtime.retainedMachine(machine)
	return retained
}

func retainedMachineAction(ctx context.Context, organization simulatorOrganization, scope machineControlScope, machine, action, dnsServer string,
	run func(context.Context, string, ...string) (string, error)) (string, error) {
	if !isSimulatorRetainedMachine(organization.RetainedMachineRuntime, machine) && machine != "callback" || action != "start" && action != "stop" {
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
	dockAddress, err := resolveRetainedDockAddress(ctx, organization, run)
	if err != nil {
		return "", err
	}
	output, err := run(ctx, "docker", retainedContainerArguments(organization, machine, scope, dnsServer, dockAddress)...)
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
		dns := net.ParseIP(manifest.DNSServer)
		if dns == nil || dns.To4() != nil {
			return "", true, errors.New("retained machine lifecycle requires the manifest's IPv6 protected DNS server")
		}
		if err := verifySimulatorRetainedTopology(ctx, organization, scenarioCommand); err != nil {
			return "", true, err
		}
		if err := verifySimulatorRetainedMachineFiles(organization); err != nil {
			return "", true, err
		}
		if _, err := retainedMachineAction(ctx, organization, scope, "callback", "start", manifest.DNSServer, scenarioCommand); err != nil {
			return "", true, err
		}
		if err := waitForRetainedControlPublication(ctx, organization, manifest.DNSServer, scenarioCommand); err != nil {
			return "", true, err
		}
	}
	output, err := retainedMachineAction(ctx, organization, scope, machine, action, manifest.DNSServer, scenarioCommand)
	if err != nil || action != "stop" {
		return output, true, err
	}
	for _, peer := range organization.RetainedMachineRuntime.retainedMachineIDs() {
		container, err := retainedInspectContainer(ctx, machineContainerName(peer), scenarioCommand)
		if err != nil {
			return output, true, err
		}
		if container != nil {
			return output, true, nil
		}
	}
	_, err = retainedMachineAction(ctx, organization, scope, "callback", "stop", manifest.DNSServer, scenarioCommand)
	return output, true, err
}
