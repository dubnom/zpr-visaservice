package systembundle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const MaxManifestBytes = 1 << 20

const qualificationStatus = "engineering-unqualified"

type Manifest struct {
	SchemaVersion         int         `json:"schema_version"`
	BundleKind            string      `json:"bundle_kind"`
	ReleaseID             string      `json:"release_id"`
	PairedSystemReleaseID string      `json:"paired_system_release_id,omitempty"`
	QualificationStatus   string      `json:"qualification_status"`
	Target                Target      `json:"target"`
	Components            []Component `json:"components"`
}

type Target struct {
	OS           string `json:"os"`
	Distribution string `json:"distribution"`
	Release      string `json:"release"`
	Architecture string `json:"architecture"`
}

type Component struct {
	ID       string   `json:"id"`
	Scope    string   `json:"scope"`
	Install  string   `json:"install"`
	Provides []string `json:"provides"`
	Platform string   `json:"platform"`
	Artifact Artifact `json:"artifact"`
	Source   *Source  `json:"source,omitempty"`
}

type Artifact struct {
	Kind           string `json:"kind"`
	Path           string `json:"path"`
	SHA256         string `json:"sha256"`
	ImageReference string `json:"image_reference,omitempty"`
}

type Source struct {
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
}

var (
	releaseIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	namePattern      = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	sha256Pattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	revisionPattern  = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	platformPattern  = regexp.MustCompile(`^[a-z0-9_]+/[a-z0-9_]+$`)
	imagePattern     = regexp.MustCompile(`^[a-zA-Z0-9._:/-]+@sha256:[0-9a-f]{64}$`)
)

var (
	requiredSystemCapabilities = []string{
		"visa-service",
		"state-backend",
		"node-runtime",
		"bootstrap-config",
		"endpoint-adapter",
		"policy-service",
		"initial-policy",
		"control-service",
		"control-room",
		"enrollment-service",
		"policy-compiler",
		"policy-evaluator",
		"protected-dns",
		"dns-publisher",
		"gateway",
	}
	bundledSystemCapabilities = []string{
		"operator-identity",
		"actor-authentication",
		"trusted-attributes",
		"logging",
		"ldap-bootstrap",
	}
	requiredCompanionCapabilities = []string{
		"simulator",
		"lab-ldap",
		"lab-oidc",
	}
	knownCapabilities = capabilitySet(
		requiredSystemCapabilities,
		bundledSystemCapabilities,
		requiredCompanionCapabilities,
		[]string{"directory", "bootstrap-dns"},
	)
)

func capabilitySet(groups ...[]string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, group := range groups {
		for _, capability := range group {
			result[capability] = struct{}{}
		}
	}
	return result
}

func Read(reader io.Reader) (Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxManifestBytes+1))
	if err != nil {
		return Manifest{}, errors.New("unable to read bundle manifest")
	}
	if len(data) > MaxManifestBytes {
		return Manifest{}, errors.New("bundle manifest exceeds 1 MiB")
	}
	if err := validateJSONShape(data); err != nil {
		return Manifest{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, errors.New("bundle manifest has unknown fields or invalid field types")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func validateJSONShape(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSONValue(decoder, 0); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func checkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("bundle manifest nesting exceeds 16 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return errors.New("bundle manifest is not valid JSON")
	}
	if token == nil {
		return errors.New("bundle manifest must not contain null values")
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("bundle manifest is not valid JSON")
			}
			name, ok := key.(string)
			if !ok || name != strings.ToLower(name) {
				return errors.New("bundle manifest field names must be lowercase")
			}
			if _, duplicate := seen[name]; duplicate {
				return errors.New("bundle manifest has duplicate fields")
			}
			seen[name] = struct{}{}
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("bundle manifest is not valid JSON")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("bundle manifest is not valid JSON")
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("bundle manifest must contain exactly one JSON document")
	}
	return nil
}

func (manifest Manifest) Validate() error {
	if manifest.SchemaVersion != 1 {
		return errors.New("bundle manifest requires schema_version 1")
	}
	if manifest.BundleKind != "system" && manifest.BundleKind != "companion" {
		return errors.New("bundle_kind must be system or companion")
	}
	if !releaseIDPattern.MatchString(manifest.ReleaseID) {
		return errors.New("release_id must be a lowercase identifier of at most 64 characters")
	}
	if manifest.QualificationStatus != qualificationStatus {
		return errors.New("this validator accepts engineering-unqualified manifests only")
	}
	if manifest.Target != (Target{OS: "linux", Distribution: "ubuntu", Release: "24.04", Architecture: "amd64"}) {
		return errors.New("target must be linux/ubuntu/24.04/amd64")
	}
	if manifest.BundleKind == "system" {
		if manifest.PairedSystemReleaseID != "" {
			return errors.New("system manifest must not set paired_system_release_id")
		}
	} else if !releaseIDPattern.MatchString(manifest.PairedSystemReleaseID) {
		return errors.New("companion manifest requires a valid paired_system_release_id")
	}
	if len(manifest.Components) == 0 {
		return errors.New("bundle manifest must declare components")
	}

	components := make(map[string]Component, len(manifest.Components))
	provided := make(map[string]Component)
	for _, component := range manifest.Components {
		if !namePattern.MatchString(component.ID) {
			return errors.New("component id must be lowercase kebab-case")
		}
		if _, duplicate := components[component.ID]; duplicate {
			return fmt.Errorf("duplicate component id %q", component.ID)
		}
		if err := component.Validate(manifest.BundleKind); err != nil {
			return fmt.Errorf("component %q: %w", component.ID, err)
		}
		if err := component.ValidateTarget(manifest.Target); err != nil {
			return fmt.Errorf("component %q: %w", component.ID, err)
		}
		components[component.ID] = component
		for _, capability := range component.Provides {
			if _, duplicate := provided[capability]; duplicate {
				return fmt.Errorf("capability %q is provided by multiple components", capability)
			}
			provided[capability] = component
		}
	}

	if manifest.BundleKind == "system" {
		return validateSystemCapabilities(provided)
	}
	return validateCompanionCapabilities(provided)
}

func (component Component) Validate(bundleKind string) error {
	if component.Scope != "zpr" && component.Scope != "production-support" && component.Scope != "companion" {
		return errors.New("scope must be zpr, production-support or companion")
	}
	if component.Install != "required" && component.Install != "bundled-default" && component.Install != "selectable" {
		return errors.New("install must be required, bundled-default or selectable")
	}
	if len(component.Provides) == 0 {
		return errors.New("provides must not be empty")
	}
	seenCapabilities := make(map[string]struct{}, len(component.Provides))
	for _, capability := range component.Provides {
		if !namePattern.MatchString(capability) {
			return errors.New("provided capabilities must be lowercase kebab-case")
		}
		if _, ok := knownCapabilities[capability]; !ok {
			return fmt.Errorf("unknown capability %q", capability)
		}
		if _, duplicate := seenCapabilities[capability]; duplicate {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seenCapabilities[capability] = struct{}{}
	}
	if !platformPattern.MatchString(component.Platform) {
		return errors.New("platform must use os/architecture form")
	}
	if err := component.Artifact.Validate(); err != nil {
		return err
	}
	if component.Source != nil {
		if strings.TrimSpace(component.Source.Repository) == "" ||
			len(component.Source.Repository) > 2048 ||
			!revisionPattern.MatchString(component.Source.Revision) {
			return errors.New("source requires a repository and full lowercase Git revision")
		}
	}
	if component.Artifact.Kind == "binary" && component.Source == nil {
		return errors.New("binary component requires source repository and revision")
	}

	switch bundleKind {
	case "system":
		if component.Scope == "companion" {
			return errors.New("system bundle must not contain companion-only components")
		}
		if component.Scope == "zpr" && component.Install != "required" {
			return errors.New("ZPR production components must be required")
		}
		for _, capability := range component.Provides {
			if capability == "simulator" || strings.HasPrefix(capability, "lab-") || capability == "bootstrap-dns" {
				return errors.New("system bundle must not contain Simulator, lab-only or bootstrap-DNS capabilities")
			}
			if component.Scope == "zpr" && isProductionSupportCapability(capability) {
				return errors.New("ZPR component must not provide production-support capabilities")
			}
			if component.Scope == "production-support" && isZPRCapability(capability) {
				return errors.New("production-support component must not provide ZPR service capabilities")
			}
		}
	case "companion":
		if component.Scope == "production-support" {
			return errors.New("companion bundle must use isolated companion services, not production-support components")
		}
		for _, capability := range component.Provides {
			if isProductionSupportCapability(capability) {
				return errors.New("companion component must not provide production-support capabilities")
			}
			if component.Scope == "zpr" && !isZPRCapability(capability) {
				return errors.New("ZPR companion component must provide only ZPR capabilities")
			}
			if component.Scope == "companion" && isZPRCapability(capability) {
				return errors.New("companion-only component must not provide ZPR capabilities")
			}
			if capability == "bootstrap-dns" && component.Install != "selectable" {
				return errors.New("companion bootstrap DNS must be selectable")
			}
		}
	}
	return nil
}

func (component Component) ValidateTarget(target Target) error {
	isEndpointAdapter := false
	for _, capability := range component.Provides {
		if capability == "endpoint-adapter" {
			isEndpointAdapter = true
			break
		}
	}
	if isEndpointAdapter {
		if len(component.Provides) != 1 || component.Scope != "zpr" {
			return errors.New("endpoint-adapter must be a standalone ZPR component")
		}
		return nil
	}
	hostPlatform := target.OS + "/" + target.Architecture
	if component.Platform != hostPlatform {
		return fmt.Errorf("platform must match the bundle host platform %s", hostPlatform)
	}
	return nil
}

func (artifact Artifact) Validate() error {
	if artifact.Kind != "oci-image" && artifact.Kind != "binary" && artifact.Kind != "archive" {
		return errors.New("artifact kind must be oci-image, binary or archive")
	}
	if !sha256Pattern.MatchString(artifact.SHA256) {
		return errors.New("artifact sha256 must be 64 lowercase hexadecimal characters")
	}
	if artifact.Path == "" || strings.Contains(artifact.Path, "\\") ||
		strings.HasPrefix(artifact.Path, "/") || strings.TrimSpace(artifact.Path) != artifact.Path ||
		!isCleanRelativePath(artifact.Path) {
		return errors.New("artifact path must be a clean relative path")
	}
	if artifact.Kind == "oci-image" {
		if !imagePattern.MatchString(artifact.ImageReference) {
			return errors.New("OCI image_reference must include an immutable sha256 digest")
		}
	} else if artifact.ImageReference != "" {
		return errors.New("non-image artifact must not set image_reference")
	}
	return nil
}

func isCleanRelativePath(value string) bool {
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validateSystemCapabilities(provided map[string]Component) error {
	for _, capability := range requiredSystemCapabilities {
		component, ok := provided[capability]
		if !ok {
			return fmt.Errorf("system bundle is missing required capability %q", capability)
		}
		if component.Scope != "zpr" || component.Install != "required" {
			return fmt.Errorf("system capability %q must be a required ZPR component", capability)
		}
	}
	for _, capability := range bundledSystemCapabilities {
		component, ok := provided[capability]
		if !ok {
			return fmt.Errorf("system bundle is missing bundled capability %q", capability)
		}
		if component.Scope != "production-support" || component.Install != "bundled-default" {
			return fmt.Errorf("system capability %q must have a bundled-default production-support component", capability)
		}
	}
	return nil
}

func validateCompanionCapabilities(provided map[string]Component) error {
	for _, capability := range requiredCompanionCapabilities {
		component, ok := provided[capability]
		if !ok {
			return fmt.Errorf("companion bundle is missing required capability %q", capability)
		}
		if component.Scope != "companion" || component.Install != "required" {
			return fmt.Errorf("companion capability %q must be a required companion component", capability)
		}
	}
	return nil
}

func isProductionSupportCapability(capability string) bool {
	for _, candidate := range bundledSystemCapabilities {
		if capability == candidate {
			return true
		}
	}
	return capability == "directory"
}

func isZPRCapability(capability string) bool {
	for _, candidate := range requiredSystemCapabilities {
		if capability == candidate {
			return true
		}
	}
	return false
}
