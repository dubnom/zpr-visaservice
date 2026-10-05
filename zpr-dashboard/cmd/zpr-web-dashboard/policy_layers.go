package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type policyLayer struct {
	Name    string   `json:"name"`
	Sources []string `json:"sources"`
}

type policyLayerBundle struct {
	OrganizationID string        `json:"organization_id"`
	Layers         []policyLayer `json:"layers"`
}

func organizationPolicyLayers(rootDirectory, organizationID string) (policyLayerBundle, error) {
	organization, err := loadSimulatorOrganization(filepath.Join(rootDirectory, "organizations"), organizationID)
	if err != nil {
		return policyLayerBundle{}, err
	}
	if organization.RuntimePolicy == "" {
		organization.RuntimePolicy = "organizations/" + organizationID + "/runtime-policy.zpl"
	}
	return policyLayerBundle{OrganizationID: organizationID, Layers: []policyLayer{
		{Name: "bootstrap", Sources: []string{"policy-layers/bootstrap.zpl"}},
		{Name: "platform", Sources: []string{"policy-layers/platform.zpl"}},
		{Name: "organization", Sources: []string{organization.RuntimePolicy}},
	}}, nil
}

func writeOrganizationPolicy(rootDirectory, organizationID, destination string) error {
	bundle, err := organizationPolicyLayers(rootDirectory, organizationID)
	if err != nil {
		return err
	}
	source, err := composePolicyLayers(rootDirectory, bundle)
	if err != nil {
		return err
	}
	organization, err := loadSimulatorOrganization(filepath.Join(rootDirectory, "organizations"), organizationID)
	if err != nil {
		return err
	}
	if organization.LoadTest != nil {
		source += generateSimulatorLoadTestPolicy(*organization.LoadTest)
		if len(source) > maxPolicySourceBytes {
			return fmt.Errorf("composed policy exceeds source size limit")
		}
	}
	if destination == "" {
		return fmt.Errorf("policy output file is required")
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".composed-policy-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(source)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), destination)
}

func generateSimulatorLoadTestPolicy(profile simulatorLoadTestProfile) string {
	var source strings.Builder
	fmt.Fprintf(&source, "\n# Generated load-test grants: %d services on %s\n", profile.ServiceCount, profile.ServiceMachine)
	for index, serviceName := range simulatorLoadTestServiceNames(profile) {
		port := profile.BasePort + index
		endpoint := fmt.Sprintf("load-service-%03d.svc.zpr", index+1)
		fmt.Fprintf(&source, "define %s as service with device.zpr.adapter.cn:'%s'.\n", serviceName, profile.ServiceWorkload)
		fmt.Fprintf(&source, "provide %s at %s over TCP %d.\n  allow LoadClient.\n", serviceName, endpoint, port)
	}
	return source.String()
}

func composePolicyLayers(rootDirectory string, bundle policyLayerBundle) (string, error) {
	if !validScenarioID(bundle.OrganizationID) {
		return "", fmt.Errorf("valid organization ID is required")
	}
	if len(bundle.Layers) != 3 {
		return "", fmt.Errorf("policy requires bootstrap, platform, and organization layers")
	}
	root, err := filepath.Abs(rootDirectory)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	var result strings.Builder
	seen := make(map[string]bool)
	for index, name := range []string{"bootstrap", "platform", "organization"} {
		layer := bundle.Layers[index]
		if layer.Name != name || len(layer.Sources) == 0 {
			return "", fmt.Errorf("layer %d must be nonempty %s", index+1, name)
		}
		fmt.Fprintf(&result, "\n# Policy layer: %s\n", name)
		for _, source := range layer.Sources {
			clean := filepath.Clean(source)
			if filepath.IsAbs(source) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("policy layer source must be relative to its bundle root")
			}
			resolved, err := filepath.EvalSymlinks(filepath.Join(root, clean))
			if err != nil {
				return "", err
			}
			relative, err := filepath.Rel(root, resolved)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("policy layer source escapes its bundle root")
			}
			if seen[resolved] {
				return "", fmt.Errorf("duplicate policy layer source %s", source)
			}
			seen[resolved] = true
			file, err := os.Open(resolved)
			if err != nil {
				return "", err
			}
			info, statErr := file.Stat()
			if statErr != nil || !info.Mode().IsRegular() {
				file.Close()
				return "", fmt.Errorf("policy layer source must be a regular file")
			}
			content, readErr := io.ReadAll(io.LimitReader(file, maxPolicySourceBytes+1))
			file.Close()
			if readErr != nil {
				return "", readErr
			}
			if len(content) > maxPolicySourceBytes || strings.TrimSpace(string(content)) == "" {
				return "", fmt.Errorf("policy layer source is empty or too large")
			}
			result.Write(content)
			result.WriteByte('\n')
			if result.Len() > maxPolicySourceBytes {
				return "", fmt.Errorf("composed policy exceeds source size limit")
			}
		}
	}
	return result.String(), nil
}
