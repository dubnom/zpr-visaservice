package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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
	Runtime            simulatorOrganizationRuntime   `json:"runtime"`
	Directory          simulatorOrganizationDirectory `json:"directory"`
	MachineOwners      map[string][]string            `json:"machine_owners,omitempty"`
	PolicyConfig       string                         `json:"policy_config"`
	PolicyCatalog      string                         `json:"policy_catalog"`
	RuntimePolicy      string                         `json:"runtime_policy,omitempty"`
	AssertionsSource   string                         `json:"assertions_source,omitempty"`
	Policies           []simulatorOrganizationPolicy  `json:"policies"`
	Services           []simulatorOrganizationService `json:"services"`
	PolicyTestServices []simulatorPolicyTestService   `json:"policy_test_services,omitempty"`
	LoadTest           *simulatorLoadTestProfile      `json:"load_test,omitempty"`
}

type simulatorLoadTestProfile struct {
	ClientMachine               string `json:"client_machine"`
	ServiceMachine              string `json:"service_machine"`
	ClientWorkload              string `json:"client_workload"`
	ServiceWorkload             string `json:"service_workload"`
	ClientCount                 int    `json:"client_count"`
	ServiceCount                int    `json:"service_count"`
	BasePort                    int    `json:"base_port"`
	DurationSeconds             int    `json:"duration_seconds"`
	RequestDelayMinMilliseconds int    `json:"request_delay_min_ms"`
	RequestDelayMaxMilliseconds int    `json:"request_delay_max_ms"`
	RestartIntervalMinSeconds   int    `json:"restart_interval_min_seconds"`
	RestartIntervalMaxSeconds   int    `json:"restart_interval_max_seconds"`
	RestartPauseMinSeconds      int    `json:"restart_pause_min_seconds"`
	RestartPauseMaxSeconds      int    `json:"restart_pause_max_seconds"`
}

type simulatorOrganizationRuntime struct {
	Driver   string                             `json:"driver"`
	Topology string                             `json:"topology"`
	Nodes    []simulatorOrganizationRuntimeNode `json:"nodes"`
}

type simulatorOrganizationRuntimeNode struct {
	ID               string `json:"id"`
	Location         string `json:"location"`
	SubstrateAddress string `json:"substrate_address,omitempty"`
	ZPRAddress       string `json:"zpr_address,omitempty"`
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
	Location   string `json:"location,omitempty"`
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	ActorCN    string `json:"actor_cn"`
	Protocol   string `json:"protocol"`
	Port       int    `json:"port,omitempty"`
	ICMPType   int    `json:"icmp_type,omitempty"`
	ICMPCode   int    `json:"icmp_code,omitempty"`
	ZPRAddress string `json:"zpr_address,omitempty"`
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

func simulatorRuntimeDriverForManifest(manifest simulatorManifest) (string, error) {
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest))
	if err != nil {
		return "", err
	}
	return organization.Runtime.Driver, nil
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
		if organization.LoadTest == nil {
			profile, err := loadSimulatorLoadTestProfile(directory, fileID)
			if err != nil {
				return nil, fmt.Errorf("organization %q load-test profile: %w", fileID, err)
			}
			organization.LoadTest = profile
		}
		materializeSimulatorLoadTestServices(&organization)
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

func loadSimulatorLoadTestProfile(directory, organizationID string) (*simulatorLoadTestProfile, error) {
	path := filepath.Join(directory, organizationID, "load_test.json")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || info.Size() > 16<<10 {
		return nil, errors.New("profile is too large or unreadable")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	var profile simulatorLoadTestProfile
	if err := decoder.Decode(&profile); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("profile contains trailing data")
	}
	return &profile, nil
}

func validateSimulatorOrganization(organization simulatorOrganization) error {
	if !validScenarioID(organization.ID) || strings.TrimSpace(organization.Name) == "" || strings.TrimSpace(organization.Description) == "" {
		return errors.New("id, name, and description are required")
	}
	if organization.Runtime.Driver != "linux-one-node" && organization.Runtime.Driver != "docker-multinode" {
		return errors.New("runtime driver must be linux-one-node or docker-multinode")
	}
	if organization.Runtime.Topology != "single-node" && organization.Runtime.Topology != "multi-node" {
		return errors.New("runtime topology must be single-node or multi-node")
	}
	if organization.Runtime.Driver == "linux-one-node" && organization.Runtime.Topology != "single-node" || organization.Runtime.Driver == "docker-multinode" && organization.Runtime.Topology != "multi-node" {
		return fmt.Errorf("runtime driver %q is incompatible with topology %q", organization.Runtime.Driver, organization.Runtime.Topology)
	}
	if organization.Runtime.Topology == "single-node" && len(organization.Runtime.Nodes) != 1 || organization.Runtime.Topology == "multi-node" && len(organization.Runtime.Nodes) < 2 {
		return fmt.Errorf("runtime topology %q has an invalid node count", organization.Runtime.Topology)
	}
	seenNodes := make(map[string]bool, len(organization.Runtime.Nodes))
	seenSubstrateAddresses := make(map[string]bool, len(organization.Runtime.Nodes))
	seenZPRAddresses := make(map[string]bool, len(organization.Runtime.Nodes))
	for _, node := range organization.Runtime.Nodes {
		if !validScenarioID(node.ID) || strings.TrimSpace(node.Location) == "" || seenNodes[node.ID] {
			return errors.New("runtime nodes require unique valid ids and locations")
		}
		if organization.Runtime.Driver == "docker-multinode" {
			substrate := net.ParseIP(node.SubstrateAddress)
			zprAddress := net.ParseIP(node.ZPRAddress)
			if substrate == nil || substrate.To4() == nil || zprAddress == nil || zprAddress.To4() != nil {
				return fmt.Errorf("runtime node %q requires an IPv4 substrate address and IPv6 ZPR address", node.ID)
			}
			if seenSubstrateAddresses[node.SubstrateAddress] || seenZPRAddresses[node.ZPRAddress] {
				return errors.New("runtime node substrate and ZPR addresses must be unique")
			}
			seenSubstrateAddresses[node.SubstrateAddress] = true
			seenZPRAddresses[node.ZPRAddress] = true
		}
		seenNodes[node.ID] = true
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
	seenGroups := make(map[string]bool, len(organization.Directory.Groups))
	for _, group := range organization.Directory.Groups {
		if strings.TrimSpace(group.Name) == "" || seenGroups[group.Name] {
			return errors.New("directory groups require unique names")
		}
		seenGroups[group.Name] = true
		members := make(map[string]bool, len(group.Members))
		for _, member := range group.Members {
			if _, exists := seen[member]; !exists || members[member] {
				return fmt.Errorf("directory group %q has an unknown or duplicate member %q", group.Name, member)
			}
			members[member] = true
		}
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
	if profile := organization.LoadTest; profile != nil {
		if !validScenarioID(profile.ClientMachine) || !validScenarioID(profile.ServiceMachine) || profile.ClientMachine == profile.ServiceMachine {
			return errors.New("load test requires distinct valid client and service machines")
		}
		if !testClientWorkloads[profile.ClientWorkload] || testServicePorts[profile.ServiceWorkload] == "" {
			return errors.New("load test requires a supported client and service workload")
		}
		if profile.ClientCount < 100 || profile.ClientCount > 512 || profile.ServiceCount < 100 || profile.ServiceCount > 512 {
			return errors.New("load test client and service counts must be between 100 and 512")
		}
		if profile.BasePort < 1024 || profile.BasePort+profile.ServiceCount-1 > 65535 {
			return errors.New("load test service port range is invalid")
		}
		if profile.DurationSeconds < 10 || profile.DurationSeconds > 180 {
			return errors.New("load test duration must be between 10 and 180 seconds")
		}
		if profile.RequestDelayMinMilliseconds < 100 || profile.RequestDelayMaxMilliseconds < profile.RequestDelayMinMilliseconds || profile.RequestDelayMaxMilliseconds > 60000 {
			return errors.New("load test request delay range is invalid")
		}
		if profile.RestartIntervalMinSeconds < 5 || profile.RestartIntervalMaxSeconds < profile.RestartIntervalMinSeconds || profile.RestartIntervalMaxSeconds > profile.DurationSeconds {
			return errors.New("load test client restart interval is invalid")
		}
		if profile.RestartPauseMinSeconds < 1 || profile.RestartPauseMaxSeconds < profile.RestartPauseMinSeconds || profile.RestartPauseMaxSeconds > 30 {
			return errors.New("load test client restart pause is invalid")
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
		if service.ZPRAddress != "" {
			address := net.ParseIP(service.ZPRAddress)
			if address == nil || address.To4() != nil {
				return fmt.Errorf("policy test service %q has an invalid ZPR IPv6 address", service.ID)
			}
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

func materializeSimulatorLoadTestServices(organization *simulatorOrganization) {
	if organization.LoadTest == nil {
		return
	}
	services := organization.Services[:0]
	for _, service := range organization.Services {
		if service.Name != "LoadServiceFleet" {
			services = append(services, service)
		}
	}
	organization.Services = services
	profile := organization.LoadTest
	for index := 1; index <= profile.ServiceCount; index++ {
		serviceNumber := fmt.Sprintf("%03d", index)
		organization.Services = append(organization.Services, simulatorOrganizationService{
			Name:        "LoadService" + serviceNumber,
			Kind:        "Stress HTTP",
			Endpoint:    fmt.Sprintf("ZPR/TCP/%d", profile.BasePort+index-1),
			Description: "Generated load-test endpoint on " + profile.ServiceMachine,
			PolicyID:    "load-service-" + serviceNumber + ".svc.zpr",
			ActorCN:     profile.ServiceWorkload,
		})
	}
}

func simulatorLoadTestServiceNames(profile simulatorLoadTestProfile) []string {
	services := make([]string, profile.ServiceCount)
	for index := range services {
		services[index] = fmt.Sprintf("LoadService%03d", index+1)
	}
	return services
}

func simulatorLoadTestServicesForAgent(directory, organizationID, agent string) ([]string, error) {
	organization, err := loadSimulatorOrganization(directory, organizationID)
	if err != nil {
		return nil, err
	}
	if organization.LoadTest == nil || organization.LoadTest.ServiceWorkload != agent {
		return nil, nil
	}
	return simulatorLoadTestServiceNames(*organization.LoadTest), nil
}

func simulatorLoadTestProfileForManifest(manifest simulatorManifest) (simulatorLoadTestProfile, bool, error) {
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest))
	if err != nil {
		return simulatorLoadTestProfile{}, false, err
	}
	if organization.LoadTest == nil {
		return simulatorLoadTestProfile{}, false, nil
	}
	return *organization.LoadTest, true, nil
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
