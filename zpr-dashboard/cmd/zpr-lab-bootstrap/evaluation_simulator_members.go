package main

// One-button activation composition: one Simulator fronts the home install
// plus explicitly declared member installs. Every member is an existing,
// independently created and paired catalog-organization install — nothing is
// created, migrated or rebound here — and each member registry entry carries
// its own paired operator target and per-install credentials so the Simulator
// resolves every organization's signer, budget authority, data-plane image
// and machine-control listener from the registry alone.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type evaluationSimulatorMember struct {
	EvaluationState            string `json:"evaluation_state"`
	RetainedMachineRuntimeFile string `json:"retained_machine_runtime_file,omitempty"`
	DataPlaneImage             string `json:"data_plane_image,omitempty"`
}

type evaluationSimulatorMembersManifest struct {
	Version int                         `json:"version"`
	Kind    string                      `json:"kind"`
	Members []evaluationSimulatorMember `json:"members"`
}

func loadEvaluationSimulatorMembers(path string) ([]evaluationSimulatorMember, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("evaluation-installs-manifest requires a canonical absolute path")
	}
	data, err := servicehttp.ReadOperatorFile(path, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read protected member install manifest: %w", err)
	}
	var manifest evaluationSimulatorMembersManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("member install manifest must be one strict JSON document")
	}
	if manifest.Version != 1 || manifest.Kind != "zpr-simulator-evaluation-installs-manifest" || len(manifest.Members) == 0 {
		return nil, errors.New("member install manifest must be a version 1 zpr-simulator-evaluation-installs-manifest with members")
	}
	return manifest.Members, nil
}

// prepareEvaluationSimulatorMembers composes the member registry entries and
// reports the member organizations in manifest order.
func prepareEvaluationSimulatorMembers(membersFile, homeEvaluation, homeOrganization, homeImage, catalog string) ([]any, []string, error) {
	members, err := loadEvaluationSimulatorMembers(membersFile)
	if err != nil {
		return nil, nil, err
	}
	if catalog == "" {
		return nil, nil, errors.New("member installs require the packaged catalog so their organization profiles stay authoritative")
	}
	targets := make([]any, 0, len(members))
	organizations := []string{}
	seen := map[string]string{homeOrganization: homeEvaluation}
	for _, member := range members {
		target, organization, err := evaluationSimulatorMemberTarget(member, homeImage, catalog)
		if err != nil {
			return nil, nil, err
		}
		if member.EvaluationState == homeEvaluation || seen[organization] != "" {
			return nil, nil, fmt.Errorf("organization %q has more than one composed install", organization)
		}
		seen[organization] = member.EvaluationState
		organizations = append(organizations, organization)
		targets = append(targets, target)
	}
	return targets, organizations, nil
}

func evaluationSimulatorMemberTarget(member evaluationSimulatorMember, homeImage, catalog string) (map[string]any, string, error) {
	evaluation := member.EvaluationState
	if !filepath.IsAbs(evaluation) || filepath.Clean(evaluation) != evaluation || evaluation == "/" {
		return nil, "", errors.New("member evaluation states require canonical absolute non-root paths")
	}
	data, err := os.ReadFile(filepath.Join(evaluation, "evaluation.json"))
	var inventory evaluationInventory
	if err != nil || json.Unmarshal(data, &inventory) != nil {
		return nil, "", fmt.Errorf("member %s: evaluation inventory unavailable", evaluation)
	}
	if err := inventory.Config.validate(); err != nil {
		return nil, "", fmt.Errorf("member %s: invalid evaluation inventory: %w", evaluation, err)
	}
	if _, err := readEvaluation(evaluation, inventory.Config); err != nil {
		return nil, "", fmt.Errorf("member %s: %w", evaluation, err)
	}
	if inventory.Config.SimulatorPort == 0 {
		return nil, "", fmt.Errorf("member %s: register its Simulator client first with -evaluation-simulator-pairing", evaluation)
	}
	organization := evaluationOrganization(inventory)
	if !slices.Contains(evaluationSimulatorCatalogIDs, organization) {
		return nil, "", fmt.Errorf("member organization %q is not a packaged catalog organization", organization)
	}
	image := member.DataPlaneImage
	if image == "" {
		image = homeImage
	}
	if !immutableImage.MatchString(image) || len(image) != len("sha256:")+64 || image[:7] != "sha256:" {
		return nil, "", fmt.Errorf("member organization %q requires an exact data-plane image ID", organization)
	}
	target := map[string]any{
		"organization_id":        organization,
		"control_room_url":       inventory.Config.browserOrigin(inventory.Config.RoomPort),
		"operator_service_url":   fmt.Sprintf("https://localhost:%d", inventory.Config.ServicePort),
		"client_cert_file":       filepath.Join(evaluation, "simulator-client-chain.pem"),
		"client_key_file":        filepath.Join(evaluation, "simulator-client-key.pem"),
		"service_ca_file":        filepath.Join(evaluation, "ca.pem"),
		"delegation_signer_file": filepath.Join(evaluation, "simulator-delegation-signer.json"),
		"delegation_key_file":    filepath.Join(evaluation, "simulator-delegation-key.pem"),
		"delegation_audience":    evaluationServiceAudience(inventory.Config),
		"data_plane_image":       image,
	}
	if inventory.Config.DataPlaneEndpoints != 0 {
		target["data_plane_endpoints_file"] = filepath.Join(evaluation, evaluationDataPlaneEndpointsFile)
	}
	if inventory.Config.VisaService && inventory.Config.VSAdminPort != 0 {
		target["visa_admin_url"] = fmt.Sprintf("https://localhost:%d", inventory.Config.VSAdminPort)
		target["visa_admin_key_file"] = filepath.Join(evaluation, "vs-admin-api.key")
	}
	if member.RetainedMachineRuntimeFile != "" {
		catalogNode, err := evaluationSimulatorCatalogNodeID(catalog, organization)
		if err != nil {
			return nil, "", fmt.Errorf("member organization %q: %w", organization, err)
		}
		binding, runtime, control, err := loadEvaluationSimulatorRetainedBindings(member.RetainedMachineRuntimeFile, inventory, catalogNode)
		if err != nil {
			return nil, "", fmt.Errorf("member organization %q: %w", organization, err)
		}
		target["retained_topology_binding"] = binding
		target["retained_machine_runtime"] = runtime
		if control.Listen != "" {
			target["control_listen"] = control.Listen
			target["control_tls_cert_file"] = filepath.Join(control.TLSDir, "server.crt")
			target["control_tls_key_file"] = filepath.Join(control.TLSDir, "server.key")
			target["control_client_ca_file"] = filepath.Join(control.TLSDir, "control-ca.crt")
		}
	}
	return target, organization, nil
}

// renderEvaluationSimulatorMembersLogin derives the one-Simulator session
// configuration spanning every composed organization from the home install's
// frozen verified login: identities, issuer, client, callback and permissions
// are preserved exactly; only the organization scope widens, and each paired
// backend still authorizes every operation against its own delegation trust.
func renderEvaluationSimulatorMembersLogin(login operatorauth.Config, homeOrganization string, memberOrganizations []string) ([]byte, error) {
	organizations := append([]string{homeOrganization}, memberOrganizations...)
	slices.Sort(organizations)
	members := login
	members.Grants = make([]operatorauth.Grant, len(login.Grants))
	for i, grant := range login.Grants {
		grant.Organizations = slices.Clone(organizations)
		grant.Permissions = slices.Clone(grant.Permissions)
		members.Grants[i] = grant
	}
	if err := members.Validate(); err != nil {
		return nil, errors.New("derived members session configuration is invalid")
	}
	document, err := json.MarshalIndent(members, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(document, '\n'), nil
}
