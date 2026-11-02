//go:build !production

package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRetainedScenarioDNSReadsProductionPublicationWithoutLegacyUpdates(t *testing.T) {
	binding := *retainedRuntimeTestOrganization(t).RetainedTopologyBinding
	for _, response := range []string{"fd5a:5052::123\n", "fd5a:5052::456\n", ""} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		calls := 0
		err := waitForRetainedScenarioDNSRecord(ctx, binding, "fd5a:5052::53", "echo-web.svc.zpr.", "fd5a:5052::123",
			func(_ context.Context, executable string, args ...string) (string, error) {
				calls++
				if executable != "docker" || strings.Join(args, " ") != "exec "+binding.NodeContainer+
					" ip netns exec zpr-dns dig +tcp +time=1 +tries=1 +short AAAA @fd5a:5052::53 echo-web.svc.zpr." {
					t.Fatalf("unexpected DNS mutation or resolver: %s %v", executable, args)
				}
				return response, nil
			})
		cancel()
		if (err == nil) != (response == "fd5a:5052::123\n") || calls == 0 {
			t.Fatalf("incorrect publication readiness: response=%q error=%v calls=%d", response, err, calls)
		}
	}
	if err := waitForRetainedScenarioDNSRecord(context.Background(), binding, "fd5a:5052::53", "echo-web.svc.zpr.", "fd5a:5052::123",
		func(context.Context, string, ...string) (string, error) {
			return "", fmt.Errorf("resolver unavailable")
		}); err == nil || !strings.Contains(err.Error(), "resolver unavailable") {
		t.Fatal("resolver failure must be explicit", err)
	}
}

func retainedRuntimeTestOrganization(t *testing.T) simulatorOrganization {
	t.Helper()
	organization := retainedTopologyTestOrganization(t)
	root := t.TempDir()
	organization.RetainedMachineRuntime = &simulatorRetainedMachineRuntime{
		Version: 1, ImageID: "sha256:" + strings.Repeat("b", 64),
		ControllerFile: filepath.Join(root, "controller"), ControllerSHA256: strings.Repeat("c", 64),
		EndpointRoot: filepath.Join(root, "endpoints"), DataPlaneCAFile: filepath.Join(root, "ca.crt"),
		ControlTLSDir: filepath.Join(root, "tls"), CallbackUpstream: "172.26.0.1:24995",
	}
	return organization
}

func TestRetainedMachineRuntimeValidationAndExactMounts(t *testing.T) {
	organization := retainedRuntimeTestOrganization(t)
	if err := validateSimulatorRetainedMachineRuntime(organization); err != nil {
		t.Fatal(err)
	}
	scope := machineControlScope{OrganizationID: "velocity", Generation: "generation-one"}
	for _, machine := range []string{"machine-03", "machine-05", "callback"} {
		args := retainedContainerArguments(organization, machine, scope, "fd5a:5052::53")
		joined := strings.Join(args, "\n")
		if strings.Contains(joined, "--privileged") || strings.Contains(joined, "bootstrap") ||
			strings.Contains(joined, "source="+organization.RetainedMachineRuntime.EndpointRoot+",") {
			t.Fatalf("broad privilege or credential mount in %s: %v", machine, args)
		}
		if !strings.Contains(joined, "--read-only") || !strings.Contains(joined, "org.zpr.runtime-digest=") {
			t.Fatal("missing isolation or runtime pin")
		}
		if machine != "callback" && (!strings.Contains(joined, `cp "$1" "/opt/zpr-workloads/authentication/$2.toml"`) ||
			!strings.Contains(joined, `chmod 600 "/opt/zpr-workloads/authentication/$2.toml"`) ||
			strings.Contains(joined, `ln -s "$1"`)) {
			t.Fatal("workload authentication must be a protected regular file, not a symlink")
		}
		if machine == "callback" {
			if strings.Contains(joined, "-proxy-listen") || strings.Contains(joined, "fffe") {
				t.Fatalf("callback must bind its granted dynamic address, not a configured listener: %v", args)
			}
		} else {
			if !strings.Contains(joined, "-control-url\nhttps://simulator-control.svc.zpr:8792") ||
				!strings.Contains(joined, "-control-dns\nfd5a:5052::53") {
				t.Fatalf("controller must resolve the production-published control name through the protected DNS: %v", args)
			}
		}
		expectedAsset := "velocity-" + machine + "-controller"
		if machine == "callback" {
			expectedAsset = "velocity-simulator-control"
		}
		if !strings.Contains(joined, retainedEndpointFiles(*organization.RetainedMachineRuntime, expectedAsset)[0]) {
			t.Fatalf("missing exact identity for %s", machine)
		}
		if machine == "machine-03" && strings.Contains(joined, "machine-05") ||
			machine == "machine-05" && strings.Contains(joined, "machine-03") ||
			machine == "callback" && strings.Contains(joined, "client.key") {
			t.Fatal("one component received another component's private identity")
		}
	}
	for _, failure := range []string{"no binding", "version", "image", "binary", "path", "upstream", "control name", "control zone"} {
		t.Run(failure, func(t *testing.T) {
			organization := retainedRuntimeTestOrganization(t)
			switch failure {
			case "no binding":
				organization.RetainedTopologyBinding = nil
			case "version":
				organization.RetainedMachineRuntime.Version++
			case "image":
				organization.RetainedMachineRuntime.ImageID = "latest"
			case "binary":
				organization.RetainedMachineRuntime.ControllerSHA256 = "unknown"
			case "path":
				organization.RetainedMachineRuntime.EndpointRoot = "/etc/../private"
			case "upstream":
				organization.RetainedMachineRuntime.CallbackUpstream = "127.0.0.1:8791"
			case "control name":
				organization.RetainedMachineRuntime.ControlDNSName = "bad name.svc.zpr"
			case "control zone":
				organization.RetainedMachineRuntime.ControlDNSName = "control.example.com"
			}
			if err := validateSimulatorRetainedMachineRuntime(organization); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}

func TestRetainedMachineLifecycleOnlyMutatesOwnedContainerIDs(t *testing.T) {
	for _, failure := range []string{"", "foreign", "generation", "image", "configuration"} {
		t.Run(failure, func(t *testing.T) {
			organization := retainedRuntimeTestOrganization(t)
			scope := machineControlScope{OrganizationID: "velocity", Generation: "startup"}
			var container *retainedContainerState
			var mutations []string
			run := func(_ context.Context, executable string, args ...string) (string, error) {
				if executable != "docker" {
					t.Fatal("unexpected executable", executable)
				}
				switch args[0] {
				case "container":
					if container == nil {
						return "", nil
					}
					return "abc123", nil
				case "inspect":
					data, err := json.Marshal([]retainedContainerState{*container})
					return string(data), err
				case "run":
					container = &retainedContainerState{ID: "abc123", Image: organization.RetainedMachineRuntime.ImageID}
					container.State.Running = true
					container.Config.Labels = map[string]string{
						"org.zpr.evaluation-install": organization.RetainedTopologyBinding.InstallID,
						"org.zpr.component":          "simulator-retained-machine-03", "zpr.organization.id": "velocity",
						"zpr.runtime.generation": scope.Generation, "org.zpr.runtime-digest": retainedRuntimeDigest(organization, scope),
					}
					return "abc123", nil
				case "stop", "rm":
					if args[len(args)-1] != "abc123" {
						t.Fatalf("not using inspected exact ID: %v", args)
					}
					mutations = append(mutations, args[0])
					return "", nil
				default:
					t.Fatalf("unexpected Docker operation: %v", args)
					return "", nil
				}
			}
			if _, err := retainedMachineAction(context.Background(), organization, scope, "machine-03", "start", "fd5a:5052::53", run); err != nil {
				t.Fatal(err)
			}
			switch failure {
			case "foreign":
				container.Config.Labels["org.zpr.evaluation-install"] = "foreign"
			case "generation":
				container.Config.Labels["zpr.runtime.generation"] = "another"
			case "image":
				container.Image = "foreign"
			case "configuration":
				container.Config.Labels["org.zpr.runtime-digest"] = "changed"
			}
			_, err := retainedMachineAction(context.Background(), organization, scope, "machine-03", "stop", "fd5a:5052::53", run)
			if (err == nil) != (failure == "") {
				t.Fatalf("wrong ownership result: %v", err)
			}
			if failure != "" && len(mutations) != 0 || failure == "" && strings.Join(mutations, ",") != "stop,rm" {
				t.Fatalf("unexpected mutation: %v", mutations)
			}
		})
	}
}

func prepareRetainedRuntimeTestFiles(t *testing.T, organization simulatorOrganization) {
	t.Helper()
	runtime := organization.RetainedMachineRuntime
	write := func(path string, data []byte, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			t.Fatal(err)
		}
	}
	executable := []byte("fixture executable")
	write(runtime.ControllerFile, executable, 0700)
	digest := sha256.Sum256(executable)
	runtime.ControllerSHA256 = hex.EncodeToString(digest[:])
	ca, key, err := createTestPolicyCA()
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})
	write(runtime.DataPlaneCAFile, caPEM, 0600)
	write(filepath.Join(runtime.ControlTLSDir, "control-ca.crt"), caPEM, 0600)
	for _, machine := range []string{"machine-03", "machine-05"} {
		certificate, key, err := createTestPolicyLeaf(ca, key, machine, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, false)
		if err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(runtime.ControlTLSDir, machine)
		if err := os.MkdirAll(directory, 0700); err != nil {
			t.Fatal(err)
		}
		if err := writeTLSCertificate(filepath.Join(directory, "client.crt"), certificate); err != nil {
			t.Fatal(err)
		}
		if err := writeTLSPrivateKey(filepath.Join(directory, "client.key"), key); err != nil {
			t.Fatal(err)
		}
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range []string{"velocity-machine-03-controller", "velocity-machine-05-controller",
		"velocity-machine-03-finance-client", "velocity-machine-05-echo-service", "velocity-simulator-control"} {
		files := retainedEndpointFiles(*runtime, asset)
		write(files[1], pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsaKey)}), 0600)
		write(files[2], caPEM, 0600)
		write(files[0], []byte(fmt.Sprintf("[global]\n[adapter]\n[authentication]\nbas_key = %q\ntls_ca_file = %q\nbas_client_id = %q\n", files[1], files[2], asset)), 0600)
	}
}

func TestRetainedMachineFilesFailClosedWithoutBootstrap(t *testing.T) {
	organization := retainedRuntimeTestOrganization(t)
	prepareRetainedRuntimeTestFiles(t, organization)
	if err := verifySimulatorRetainedMachineFiles(organization); err != nil {
		t.Fatal(err)
	}
	organization.RetainedMachineRuntime.ControllerSHA256 = strings.Repeat("f", 64)
	if err := verifySimulatorRetainedMachineFiles(organization); err == nil {
		t.Fatal("changed executable accepted")
	}
}

func retainedCustomInventoryOrganization(t *testing.T) simulatorOrganization {
	t.Helper()
	organization := retainedRuntimeTestOrganization(t)
	organization.RetainedMachineRuntime.ControlAsset = "northstar-simulator-control"
	organization.RetainedMachineRuntime.Machines = []simulatorRetainedMachine{
		{MachineID: "machine-01", ControllerAsset: "northstar-machine-01-controller",
			Workload: "operations-client", WorkloadAsset: "northstar-machine-01-operations-client"},
		{MachineID: "machine-02", ControllerAsset: "northstar-machine-02-controller",
			Workload: "metrics-service", WorkloadAsset: "northstar-machine-02-metrics-service"},
	}
	return organization
}

func TestRetainedInventoryDefaultsToReviewedVelocityContract(t *testing.T) {
	organization := retainedRuntimeTestOrganization(t)
	runtime := *organization.RetainedMachineRuntime
	control, machines := runtime.retainedInventory()
	if control != "velocity-simulator-control" || len(machines) != 2 ||
		machines[0].MachineID != "machine-03" || machines[1].MachineID != "machine-05" {
		t.Fatalf("default inventory is not the reviewed Velocity contract: %q %v", control, machines)
	}
	required := runtime.requiredAssets()
	for _, asset := range simulatorRetainedRequiredAssets {
		if !slices.Contains(required, asset) {
			t.Fatalf("default required assets lack %q: %v", asset, required)
		}
	}
	if len(required) != len(simulatorRetainedRequiredAssets) {
		t.Fatalf("default required assets differ from the reviewed inventory: %v", required)
	}
	if !isSimulatorRetainedMachine(&runtime, "machine-03") || isSimulatorRetainedMachine(&runtime, "machine-01") ||
		isSimulatorRetainedMachine(nil, "machine-03") {
		t.Fatal("default retained machine recognition is wrong")
	}
}

func TestRetainedInventoryDeclaresPerOrganizationMachines(t *testing.T) {
	organization := retainedCustomInventoryOrganization(t)
	if err := validateSimulatorRetainedMachineRuntime(organization); err != nil {
		t.Fatal(err)
	}
	runtime := *organization.RetainedMachineRuntime
	required := runtime.requiredAssets()
	for _, asset := range []string{"northstar-simulator-control", "northstar-machine-01-controller",
		"northstar-machine-01-operations-client", "northstar-machine-02-controller", "northstar-machine-02-metrics-service"} {
		if !slices.Contains(required, asset) {
			t.Fatalf("declared inventory lacks %q: %v", asset, required)
		}
	}
	if slices.Contains(required, "velocity-simulator-control") {
		t.Fatal("declared inventory still carries the Velocity default")
	}
	if !isSimulatorRetainedMachine(&runtime, "machine-01") || isSimulatorRetainedMachine(&runtime, "machine-03") {
		t.Fatal("declared machine recognition is wrong")
	}
	scope := machineControlScope{OrganizationID: organization.ID, Generation: "generation-one"}
	args := strings.Join(retainedContainerArguments(organization, "machine-01", scope, "fd5a:5052::53"), "\n")
	if !strings.Contains(args, "northstar-machine-01-controller") ||
		!strings.Contains(args, "northstar-machine-01-operations-client") ||
		!strings.Contains(args, "operations-client") || strings.Contains(args, "velocity-") {
		t.Fatalf("container arguments do not use the declared inventory: %s", args)
	}
	callback := strings.Join(retainedContainerArguments(organization, "callback", scope, "fd5a:5052::53"), "\n")
	if !strings.Contains(callback, "northstar-simulator-control") || strings.Contains(callback, "velocity-") {
		t.Fatalf("callback arguments do not use the declared control identity: %s", callback)
	}
}

func TestRetainedInventoryValidationFailsClosed(t *testing.T) {
	for name, mutate := range map[string]func(*simulatorRetainedMachineRuntime){
		"control asset without machines": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.Machines = nil
		},
		"machines without control asset": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.ControlAsset = ""
		},
		"incomplete machine": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.Machines[0].WorkloadAsset = ""
		},
		"duplicate machine": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.Machines[1].MachineID = runtime.Machines[0].MachineID
		},
		"duplicate asset": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.Machines[1].ControllerAsset = runtime.Machines[0].ControllerAsset
		},
		"asset aliasing the control identity": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.Machines[0].ControllerAsset = runtime.ControlAsset
		},
		"unknown workload": func(runtime *simulatorRetainedMachineRuntime) {
			runtime.Machines[0].Workload = "unsupported-workload"
		},
	} {
		t.Run(name, func(t *testing.T) {
			organization := retainedCustomInventoryOrganization(t)
			mutate(organization.RetainedMachineRuntime)
			if err := validateSimulatorRetainedMachineRuntime(organization); err == nil {
				t.Fatal("invalid retained inventory was not refused")
			}
		})
	}
}
