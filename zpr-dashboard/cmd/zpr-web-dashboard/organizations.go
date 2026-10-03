package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultSimulatorOrganizationID = "northstar"

type simulatorOrganization struct {
	ID                 string                         `json:"id"`
	Name               string                         `json:"name"`
	Description        string                         `json:"description"`
	Directory          simulatorOrganizationDirectory `json:"directory"`
	MachineOwners      map[string][]string            `json:"machine_owners,omitempty"`
	PolicyConfig       string                         `json:"policy_config"`
	PolicyCatalog      string                         `json:"policy_catalog"`
	RuntimePolicy      string                         `json:"runtime_policy,omitempty"`
	Policies           []simulatorOrganizationPolicy  `json:"policies"`
	Services           []simulatorOrganizationService `json:"services"`
	PolicyTestServices []simulatorPolicyTestService   `json:"policy_test_services,omitempty"`
}

type simulatorOrganizationDirectory struct {
	BaseDN               string                        `json:"base_dn"`
	SeedMode             string                        `json:"seed_mode"`
	LDIFFiles            []string                      `json:"ldif_files"`
	HealthUID            string                        `json:"health_uid"`
	IdentityMappings     map[string]string             `json:"identity_mappings"`
	Attributes           []string                      `json:"attributes"`
	UserIdentityMappings map[string]string             `json:"user_identity_mappings,omitempty"`
	UserAttributes       []string                      `json:"user_attributes,omitempty"`
	Departments          []simulatorOrganizationUnit   `json:"departments"`
	People               []simulatorOrganizationPerson `json:"people"`
	Groups               []simulatorOrganizationGroup  `json:"groups"`
}

type simulatorOrganizationUnit struct {
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
}

type simulatorOrganizationPerson struct {
	UID        string `json:"uid"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Department string `json:"department"`
	Title      string `json:"title"`
}

type simulatorOrganizationGroup struct {
	Name    string   `json:"name"`
	Members []string `json:"members"`
}

type simulatorOrganizationPolicy struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type simulatorOrganizationService struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Endpoint    string `json:"endpoint"`
	Description string `json:"description"`
	PolicyID    string `json:"policy_id,omitempty"`
	ActorCN     string `json:"actor_cn,omitempty"`
}

type simulatorPolicyTestService struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ActorCN  string `json:"actor_cn"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port,omitempty"`
	ICMPType int    `json:"icmp_type,omitempty"`
	ICMPCode int    `json:"icmp_code,omitempty"`
}

func simulatorOrganizationsDirectory() string {
	if path := strings.TrimSpace(os.Getenv("SIMULATION_ORGANIZATIONS_DIR")); path != "" {
		return path
	}
	return "examples/organizations"
}

func activeSimulatorOrganizationID(manifest simulatorManifest) string {
	if id := strings.TrimSpace(manifest.OrganizationID); id != "" {
		return id
	}
	return defaultSimulatorOrganizationID
}

func simulatorActiveOrganizationPath() string {
	if path := strings.TrimSpace(os.Getenv("SIMULATION_ACTIVE_ORGANIZATION_FILE")); path != "" {
		return path
	}
	return filepath.Join(filepath.Dir(simulatorManifestPath()), "active-organization.txt")
}

func handleSimulatorOrganizationActivate(w http.ResponseWriter, r *http.Request) {
	organizationID := r.PathValue("organization")
	if _, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID); err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return
	}
	activeSimulatorScenario.mu.Lock()
	defer activeSimulatorScenario.mu.Unlock()
	if activeSimulatorScenario.run.State == "running" || activeSimulatorScenario.run.State == "cleaning" {
		writeWorkspaceError(w, http.StatusConflict, "Finish or cancel the running scenario before activating another organization.")
		return
	}
	simulatorSessions.RLock()
	defer simulatorSessions.RUnlock()
	for _, session := range simulatorSessions.byMachine {
		if session.Authenticated {
			writeWorkspaceError(w, http.StatusConflict, "Log out all machine users before activating another organization.")
			return
		}
	}
	if !activeOrganizationActivation.start(organizationID, simulatorActiveOrganizationPath()) {
		writeWorkspaceError(w, http.StatusConflict, "An organization activation is already in progress.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeSimulatorJSON(w, map[string]any{"activation": activeOrganizationActivation.snapshot()})
}

func loadSimulatorOrganizations(directory string) ([]simulatorOrganization, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read organization directory: %w", err)
	}
	organizations := make([]simulatorOrganization, 0, len(entries))
	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxScenarioFileSize {
			return nil, fmt.Errorf("organization file %q is too large or unreadable", entry.Name())
		}
		filePath := filepath.Join(directory, entry.Name())
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read organization %q: %w", entry.Name(), err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(content)))
		decoder.DisallowUnknownFields()
		var organization simulatorOrganization
		if err := decoder.Decode(&organization); err != nil {
			return nil, fmt.Errorf("decode organization %q: %w", entry.Name(), err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("organization %q contains trailing data", entry.Name())
		}
		fileID := strings.TrimSuffix(entry.Name(), ".json")
		if organization.ID != fileID {
			return nil, fmt.Errorf("organization %q id must match its filename", entry.Name())
		}
		if _, exists := ids[organization.ID]; exists {
			return nil, fmt.Errorf("duplicate organization id %q", organization.ID)
		}
		ids[organization.ID] = struct{}{}
		if err := validateSimulatorOrganization(organization); err != nil {
			return nil, fmt.Errorf("organization %q: %w", organization.ID, err)
		}
		organizations = append(organizations, organization)
	}
	sort.Slice(organizations, func(i, j int) bool { return organizations[i].Name < organizations[j].Name })
	if len(organizations) == 0 {
		return nil, errors.New("organization catalog is empty")
	}
	return organizations, nil
}

func validateSimulatorOrganization(organization simulatorOrganization) error {
	if !validScenarioID(organization.ID) || strings.TrimSpace(organization.Name) == "" || strings.TrimSpace(organization.Description) == "" {
		return errors.New("id, name, and description are required")
	}
	if !strings.Contains(organization.Directory.BaseDN, "=") {
		return errors.New("directory base_dn is required")
	}
	if organization.Directory.SeedMode != "pregen" && organization.Directory.SeedMode != "ldif" {
		return fmt.Errorf("unsupported directory seed_mode %q", organization.Directory.SeedMode)
	}
	if len(organization.Directory.LDIFFiles) == 0 {
		return errors.New("directory requires at least one LDIF seed file")
	}
	if organization.Directory.HealthUID == "" || len(organization.Directory.IdentityMappings) == 0 || len(organization.Directory.Attributes) == 0 {
		return errors.New("directory requires a health uid, identity mappings, and attributes")
	}
	for identity, attribute := range organization.Directory.UserIdentityMappings {
		if !strings.HasPrefix(identity, "user.") || !validLDAPAttributeName(attribute) {
			return errors.New("user directory identity mappings must map user.* identities to LDAP attributes")
		}
	}
	for _, attribute := range organization.Directory.UserAttributes {
		if !validLDAPAttributeName(attribute) {
			return fmt.Errorf("invalid user directory LDAP attribute %q", attribute)
		}
	}
	for _, seedFile := range organization.Directory.LDIFFiles {
		clean := filepath.Clean(seedFile)
		if filepath.IsAbs(seedFile) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("directory seed file %q must be a safe relative path", seedFile)
		}
	}
	seen := make(map[string]struct{}, len(organization.Directory.People))
	for _, person := range organization.Directory.People {
		if person.UID == "" || person.Name == "" || person.Department == "" {
			return errors.New("directory people require uid, name, and department")
		}
		if _, exists := seen[person.UID]; exists {
			return fmt.Errorf("duplicate directory uid %q", person.UID)
		}
		seen[person.UID] = struct{}{}
	}
	for _, policy := range organization.Policies {
		if !validScenarioID(policy.ID) || policy.Name == "" || policy.Description == "" {
			return errors.New("policies require a valid id, name, and description")
		}
	}
	for _, service := range organization.Services {
		if service.Name == "" || service.Kind == "" || service.Endpoint == "" {
			return errors.New("services require name, kind, and endpoint")
		}
		if service.PolicyID != "" && (!policyTestValueSafe(service.PolicyID) || len(service.PolicyID) > 200) {
			return fmt.Errorf("service %q has an invalid policy service id", service.Name)
		}
		if service.ActorCN != "" && (!policyTestValueSafe(service.ActorCN) || len(service.ActorCN) > 200) {
			return fmt.Errorf("service %q has an invalid provider actor identity", service.Name)
		}
	}
	testServiceIDs := make(map[string]bool, len(organization.PolicyTestServices))
	for _, service := range organization.PolicyTestServices {
		if !policyTestValueSafe(service.ID) || strings.TrimSpace(service.ID) == "" || testServiceIDs[service.ID] {
			return errors.New("policy test service ids must be unique and valid")
		}
		if strings.TrimSpace(service.Name) == "" || strings.TrimSpace(service.ActorCN) == "" || !policyTestValueSafe(service.ActorCN) {
			return fmt.Errorf("policy test service %q requires a name and provider identity", service.ID)
		}
		switch strings.ToUpper(service.Protocol) {
		case "TCP", "UDP":
			if service.Port < 1 || service.Port > 65535 {
				return fmt.Errorf("policy test service %q has an invalid port", service.ID)
			}
		case "ICMP6":
			if service.ICMPType < 0 || service.ICMPType > 255 || service.ICMPCode < 0 || service.ICMPCode > 255 {
				return fmt.Errorf("policy test service %q has an invalid ICMPv6 type or code", service.ID)
			}
		default:
			return fmt.Errorf("policy test service %q uses an unsupported protocol", service.ID)
		}
		testServiceIDs[service.ID] = true
	}
	for machineID, owners := range organization.MachineOwners {
		if !validScenarioID(machineID) || len(owners) == 0 {
			return fmt.Errorf("machine ownership for %q requires a valid machine id and owners", machineID)
		}
		for _, owner := range owners {
			if _, exists := seen[owner]; !exists {
				return fmt.Errorf("machine %q references unknown organization person %q", machineID, owner)
			}
		}
	}
	return nil
}

func loadSimulatorOrganization(directory, id string) (simulatorOrganization, error) {
	organizations, err := loadSimulatorOrganizations(directory)
	if err != nil {
		return simulatorOrganization{}, err
	}
	for _, organization := range organizations {
		if organization.ID == id {
			return organization, nil
		}
	}
	return simulatorOrganization{}, fmt.Errorf("unknown organization %q", id)
}

func simulatorOrganizationAllowsUser(manifest simulatorManifest, organization simulatorOrganization, machineID, user string) bool {
	user = strings.TrimSpace(user)
	if user == "" || user == "it-pool" {
		return false
	}
	personExists := false
	for _, person := range organization.Directory.People {
		if person.UID == user {
			personExists = true
			break
		}
	}
	if !personExists {
		return false
	}
	if owners, hasAssignments := organization.MachineOwners[machineID]; hasAssignments {
		for _, owner := range owners {
			if owner == user {
				return true
			}
		}
		return false
	}
	if len(organization.MachineOwners) > 0 {
		return false
	}
	return simulatorMachineAllowsUser(manifest, machineID, user)
}

func validLDAPAttributeName(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func handleSimulatorOrganizations(w http.ResponseWriter, _ *http.Request) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		http.Error(w, "simulation manifest unavailable", http.StatusServiceUnavailable)
		return
	}
	organizations, err := loadSimulatorOrganizations(simulatorOrganizationsDirectory())
	if err != nil {
		http.Error(w, "organization catalog unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeSimulatorJSON(w, map[string]any{
		"active_id":     manifest.OrganizationID,
		"organizations": organizations,
		"activation":    activeOrganizationActivation.snapshot(),
	})
}
