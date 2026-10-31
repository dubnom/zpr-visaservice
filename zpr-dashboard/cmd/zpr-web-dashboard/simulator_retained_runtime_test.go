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
		args := retainedContainerArguments(organization, machine, scope)
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
	for _, failure := range []string{"no binding", "version", "image", "binary", "path", "upstream"} {
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
			if _, err := retainedMachineAction(context.Background(), organization, scope, "machine-03", "start", run); err != nil {
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
			_, err := retainedMachineAction(context.Background(), organization, scope, "machine-03", "stop", run)
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
