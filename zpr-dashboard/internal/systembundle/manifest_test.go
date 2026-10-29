package systembundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatesSystemAndPairedCompanionBundles(t *testing.T) {
	systemDirectory := t.TempDir()
	system := testSystemManifest(t, systemDirectory)
	if err := ValidateBundle(system, systemDirectory); err != nil {
		t.Fatalf("ValidateBundle(system) error = %v", err)
	}

	companionDirectory := t.TempDir()
	companion := testCompanionManifest(t, system, companionDirectory)
	if err := ValidateBundle(companion, companionDirectory); err != nil {
		t.Fatalf("ValidateBundle(companion) error = %v", err)
	}
	if err := ValidateCompanionPair(companion, system); err != nil {
		t.Fatalf("ValidateCompanionPair() error = %v", err)
	}
}

func TestRejectsMissingRequiredSystemCapability(t *testing.T) {
	for _, capability := range requiredSystemCapabilities {
		t.Run(capability, func(t *testing.T) {
			manifest := testSystemManifest(t, t.TempDir())
			manifest.Components = removeComponentProviding(manifest.Components, capability)
			if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), `"`+capability+`"`) {
				t.Fatalf("Validate() error = %v, want missing %s", err, capability)
			}
		})
	}
}

func TestRejectsMissingBundledSystemCapability(t *testing.T) {
	for _, capability := range bundledSystemCapabilities {
		t.Run(capability, func(t *testing.T) {
			manifest := testSystemManifest(t, t.TempDir())
			manifest.Components = removeComponentProviding(manifest.Components, capability)
			if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), `"`+capability+`"`) {
				t.Fatalf("Validate() error = %v, want missing %s", err, capability)
			}
		})
	}
}

func TestSystemBundleCapabilitySetsCoverWorkingNetworkBaseline(t *testing.T) {
	required := map[string]bool{}
	for _, capability := range requiredSystemCapabilities {
		required[capability] = true
	}
	bundled := map[string]bool{}
	for _, capability := range bundledSystemCapabilities {
		bundled[capability] = true
	}
	for _, capability := range []string{
		"visa-service", "node-runtime", "protected-dns", "logging",
		"control-room", "operator-identity", "trusted-attributes",
		"ldap-bootstrap", "bootstrap-config", "initial-policy",
	} {
		if !required[capability] && !bundled[capability] {
			t.Errorf("working-network baseline capability %q is not mandatory", capability)
		}
	}
}

func TestRejectsIncorrectSystemCapabilityInstallMode(t *testing.T) {
	for _, capability := range requiredSystemCapabilities {
		t.Run(capability+"/not-required", func(t *testing.T) {
			manifest := testSystemManifest(t, t.TempDir())
			component := systemComponentProviding(&manifest, capability)
			component.Install = "selectable"
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() accepted a selectable required capability")
			}
		})
		t.Run(capability+"/wrong-scope", func(t *testing.T) {
			manifest := testSystemManifest(t, t.TempDir())
			component := systemComponentProviding(&manifest, capability)
			component.Scope = "production-support"
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() accepted a required capability outside the ZPR scope")
			}
		})
	}
	for _, capability := range bundledSystemCapabilities {
		t.Run(capability+"/not-bundled-default", func(t *testing.T) {
			manifest := testSystemManifest(t, t.TempDir())
			component := systemComponentProviding(&manifest, capability)
			component.Install = "selectable"
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() accepted a missing or optional baseline capability")
			}
		})
		t.Run(capability+"/wrong-scope", func(t *testing.T) {
			manifest := testSystemManifest(t, t.TempDir())
			component := systemComponentProviding(&manifest, capability)
			component.Scope = "zpr"
			if err := manifest.Validate(); err == nil {
				t.Fatal("Validate() accepted a bundled capability in the ZPR service scope")
			}
		})
	}
}

func systemComponentProviding(manifest *Manifest, capability string) *Component {
	for index := range manifest.Components {
		component := &manifest.Components[index]
		if len(component.Provides) == 1 && component.Provides[0] == capability {
			return component
		}
	}
	panic("test manifest has no component for " + capability)
}

func TestRejectsHostComponentPlatformMismatch(t *testing.T) {
	manifest := testSystemManifest(t, t.TempDir())
	for index := range manifest.Components {
		if manifest.Components[index].ID == "policy-compiler" {
			manifest.Components[index].Platform = "linux/arm64"
			break
		}
	}
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "bundle host platform linux/amd64") {
		t.Fatalf("Validate() error = %v, want host platform mismatch", err)
	}
}

func TestAllowsExplicitEndpointPlatform(t *testing.T) {
	manifest := testSystemManifest(t, t.TempDir())
	for index := range manifest.Components {
		if manifest.Components[index].ID == "endpoint-adapter" {
			manifest.Components[index].Platform = "darwin/arm64"
			break
		}
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want explicit endpoint platform accepted", err)
	}
}

func TestRejectsCompanionComponentInSystemBundle(t *testing.T) {
	manifest := testSystemManifest(t, t.TempDir())
	manifest.Components = append(manifest.Components, testComponent(t, t.TempDir(), "simulator", "companion", "required", "simulator"))
	if err := manifest.Validate(); err == nil {
		t.Fatal("Validate() accepted Simulator in system bundle")
	}
}

func TestRejectsTagOnlyImageReference(t *testing.T) {
	component := testComponent(t, t.TempDir(), "test-image", "zpr", "required", "visa-service")
	component.Artifact.Kind = "oci-image"
	component.Artifact.ImageReference = "registry.example/test:1.2.3"
	if err := component.Validate("system"); err == nil || !strings.Contains(err.Error(), "immutable sha256 digest") {
		t.Fatalf("Validate() error = %v, want immutable image digest failure", err)
	}
}

func TestRejectsProductionQualificationClaim(t *testing.T) {
	manifest := testSystemManifest(t, t.TempDir())
	manifest.QualificationStatus = "production"
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "engineering-unqualified") {
		t.Fatalf("Validate() error = %v, want explicit engineering-only failure", err)
	}
}

func TestReadRejectsDuplicateUnknownNullAndTrailingJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "duplicate keys", data: `{"schema_version":1,"schema_version":1}`},
		{name: "unknown field", data: `{"unknown":true}`},
		{name: "null", data: `{"schema_version":null}`},
		{name: "trailing document", data: `{} {}`},
		{name: "uppercase field", data: `{"Schema_version":1}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Read(strings.NewReader(test.data)); err == nil {
				t.Fatal("Read() accepted invalid manifest JSON")
			}
		})
	}
}

func TestValidateBundleRejectsChecksumMismatchAndSymlink(t *testing.T) {
	t.Run("checksum mismatch", func(t *testing.T) {
		directory := t.TempDir()
		manifest := testSystemManifest(t, directory)
		component := manifest.Components[0]
		filename := filepath.Join(directory, filepath.FromSlash(component.Artifact.Path))
		if err := os.WriteFile(filename, []byte("tampered"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBundle(manifest, directory); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatalf("ValidateBundle() error = %v, want checksum failure", err)
		}
	})

	t.Run("symlink", func(t *testing.T) {
		directory := t.TempDir()
		manifest := testSystemManifest(t, directory)
		component := manifest.Components[0]
		filename := filepath.Join(directory, filepath.FromSlash(component.Artifact.Path))
		outside := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(outside, []byte("payload"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filename); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filename); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBundle(manifest, directory); err == nil {
			t.Fatal("ValidateBundle() accepted an artifact symlink")
		}
	})
}

func TestValidateCompanionPairRejectsChangedZPRArtifact(t *testing.T) {
	system := testSystemManifest(t, t.TempDir())
	companion := testCompanionManifest(t, system, t.TempDir())
	companion.Components[0].Artifact.SHA256 = strings.Repeat("a", 64)
	if err := ValidateCompanionPair(companion, system); err == nil {
		t.Fatal("ValidateCompanionPair() accepted changed ZPR artifact")
	}
}

func testSystemManifest(t *testing.T, directory string) Manifest {
	t.Helper()
	manifest := Manifest{
		SchemaVersion:       1,
		BundleKind:          "system",
		ReleaseID:           "system-candidate-1",
		QualificationStatus: qualificationStatus,
		Target:              Target{OS: "linux", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64"},
	}
	for _, capability := range requiredSystemCapabilities {
		manifest.Components = append(manifest.Components, testComponent(t, directory, capability, "zpr", "required", capability))
	}
	for _, capability := range bundledSystemCapabilities {
		manifest.Components = append(manifest.Components, testComponent(t, directory, capability, "production-support", "bundled-default", capability))
	}
	return manifest
}

func testCompanionManifest(t *testing.T, system Manifest, directory string) Manifest {
	t.Helper()
	manifest := Manifest{
		SchemaVersion:         1,
		BundleKind:            "companion",
		ReleaseID:             "companion-candidate-1",
		PairedSystemReleaseID: system.ReleaseID,
		QualificationStatus:   qualificationStatus,
		Target:                system.Target,
	}
	for _, component := range system.Components {
		if component.Scope == "zpr" {
			manifest.Components = append(manifest.Components, testComponent(t, directory, component.ID, "zpr", "required", component.Provides...))
		}
	}
	for _, capability := range requiredCompanionCapabilities {
		manifest.Components = append(manifest.Components, testComponent(t, directory, capability, "companion", "required", capability))
	}
	manifest.Components = append(manifest.Components, testComponent(t, directory, "bootstrap-dns", "companion", "selectable", "bootstrap-dns"))
	return manifest
}

func testComponent(t *testing.T, directory, id, scope, install string, capabilities ...string) Component {
	t.Helper()
	content := []byte("payload:" + id)
	relative := filepath.ToSlash(filepath.Join("artifacts", id+".tar"))
	filename := filepath.Join(directory, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, content, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	return Component{
		ID:       id,
		Scope:    scope,
		Install:  install,
		Provides: capabilities,
		Platform: "linux/amd64",
		Artifact: Artifact{
			Kind:   "archive",
			Path:   relative,
			SHA256: hex.EncodeToString(sum[:]),
		},
	}
}

func removeComponentProviding(components []Component, capability string) []Component {
	result := make([]Component, 0, len(components)-1)
	for _, component := range components {
		if len(component.Provides) == 1 && component.Provides[0] == capability {
			continue
		}
		result = append(result, component)
	}
	return result
}

func TestReadRoundTripsManifest(t *testing.T) {
	manifest := testSystemManifest(t, t.TempDir())
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(strings.NewReader(string(data)))
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.ReleaseID != manifest.ReleaseID || got.BundleKind != manifest.BundleKind {
		t.Fatalf("Read() = %#v, want release %q", got, manifest.ReleaseID)
	}
}

func TestReadBundleManifestRequiresContainedRegularFile(t *testing.T) {
	directory := t.TempDir()
	manifest := testSystemManifest(t, directory)
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBundleManifest(directory, "manifest.json"); err != nil {
		t.Fatalf("ReadBundleManifest() error = %v", err)
	}
	if _, err := ReadBundleManifest(directory, "../manifest.json"); err == nil {
		t.Fatal("ReadBundleManifest() accepted a path escaping the bundle")
	}
}
