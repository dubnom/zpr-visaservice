package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

// Display-only edits do not change the identity represented by a source ID.
func sourceIdentityDigest(source assertionSourceConfig) string {
	source.DisplayName, source.Version = "", 0
	data, _ := json.Marshal(source)
	return sourceConfigRevision(data)
}

func policySourceRegistry() (assertionSourcesConfig, error) {
	var config assertionSourcesConfig
	path := strings.TrimSpace(os.Getenv("ZPR_ASSERTION_SOURCES_FILE"))
	if path == "" {
		return config, nil
	}
	data, err := readOperatorFile(path, 64<<10, true)
	if err != nil {
		return config, errors.New("Trusted source registry is unavailable; reload or restage after recovery.")
	}
	if json.Unmarshal(data, &config) != nil {
		return config, errors.New("Trusted source registry is invalid.")
	}
	if len(config.Sources) > 16 {
		return config, errors.New("At most 16 trusted source identities are allowed.")
	}
	names := make(map[string]assertionSourceConfig)
	for _, source := range config.Sources {
		if !assertionSourceName.MatchString(source.Name) || strings.Contains(source.Name, sourceAttributeSeparator) {
			return config, errors.New("Trusted source registry contains an invalid source ID.")
		}
		if _, duplicate := names[source.Name]; duplicate {
			return config, errors.New("Trusted source IDs must be unique.")
		}
		names[source.Name] = source
	}
	for organization, name := range config.PolicyDefaults {
		source, exists := names[name]
		if _, scoped := source.Organizations[organization]; organization == "" || !exists || !scoped {
			return config, errors.New("Policy default must name exactly one source scoped to its organization.")
		}
	}
	return config, nil
}

func configuredPolicyDefaultSource() string {
	config, err := policySourceRegistry()
	if err != nil {
		return ""
	}
	return config.PolicyDefaults[os.Getenv("ZPR_POLICY_ORGANIZATION_ID")]
}

func appendPolicyDefaultArgs(args []string) []string {
	return policyCompilerSourceArgs(args, configuredPolicyDefaultSource())
}

func policyCompilerSourceArgs(args []string, defaultSource string) []string {
	result := []string{args[0], "--require-source-qualification"}
	if defaultSource != "" {
		result = append(result, "--default-source", defaultSource)
	}
	return append(result, args[1:]...)
}

// Bind conservatively to every configured source, not just positive conditions:
// negative conditions, class expansion and built-in admission can also depend on one.
func policySourceBindings(configPath string) (map[string]string, string, error) {
	var compiler struct {
		TrustedServices map[string]any `toml:"trusted_services"`
	}
	if _, err := toml.DecodeFile(configPath, &compiler); err != nil {
		if os.Getenv("ZPR_ASSERTION_SOURCES_FILE") == "" {
			data, readErr := os.ReadFile(configPath)
			if readErr != nil {
				return nil, "", readErr
			}
			return map[string]string{"$config": sourceConfigRevision(data)}, "", nil
		}
		return nil, "", err
	}
	registry, err := policySourceRegistry()
	if err != nil {
		return nil, "", err
	}
	organization := os.Getenv("ZPR_POLICY_ORGANIZATION_ID")
	defaultSource := registry.PolicyDefaults[organization]
	if defaultSource != "" {
		if _, exists := compiler.TrustedServices[defaultSource]; !exists {
			return nil, "", errors.New("Policy default is absent from the compiler source configuration.")
		}
	}
	bindings := make(map[string]string)
	for id, definition := range compiler.TrustedServices {
		data, _ := json.Marshal(definition)
		bindings[id] = sourceConfigRevision(data)
		for _, source := range registry.Sources {
			if source.Name != id {
				continue
			}
			if _, scoped := source.Organizations[organization]; !scoped {
				return nil, "", fmt.Errorf("Trusted source %q is not scoped to this organization.", id)
			}
			bindings[id] += fmt.Sprintf(":%d:%s", source.Version, sourceIdentityDigest(source))
		}
	}
	return bindings, defaultSource, nil
}

func validatePolicySourceBindings(stage stagedPolicyCandidate, configPath string) error {
	bindings, defaultSource, err := policySourceBindings(configPath)
	if err != nil {
		return err
	}
	// Old artifacts can only be used when no mutable registry is configured.
	if stage.SourceVersions == nil && os.Getenv("ZPR_ASSERTION_SOURCES_FILE") == "" {
		return nil
	}
	if os.Getenv("ZPR_ASSERTION_SOURCES_FILE") != "" && stage.SourceBindingSHA256 != policySourceBindingDigest(bindings, defaultSource) {
		return errors.New("Source version binding changed; restage and obtain new approval.")
	}
	if !reflect.DeepEqual(stage.SourceVersions, bindings) || stage.DefaultSource != defaultSource {
		return errors.New("Trusted source identity, version or policy default changed; restage and obtain new approval.")
	}
	return nil
}

func policySourceBindingDigest(versions map[string]string, defaultSource string) string {
	data, _ := json.Marshal(struct {
		Organization string            `json:"organization"`
		Versions     map[string]string `json:"versions"`
		Default      string            `json:"default"`
	}{os.Getenv("ZPR_POLICY_ORGANIZATION_ID"), versions, defaultSource})
	return sourceConfigRevision(data)
}
