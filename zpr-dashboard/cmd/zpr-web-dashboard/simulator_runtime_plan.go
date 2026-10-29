//go:build !production

package main

import (
	"fmt"
	"net"
	"sort"
	"strings"
)

type simulatorRuntimePlanStatus string

const (
	simulatorRuntimePlanBlocked    simulatorRuntimePlanStatus   = "blocked"
	simulatorRuntimePlanOnly       simulatorRuntimePlanStatus   = "plan-only"
	simulatorRuntimeBudgetAbsent   simulatorRuntimeBudgetStatus = "absent"
	simulatorRuntimeBudgetUnknown  simulatorRuntimeBudgetStatus = "unverified"
	simulatorRuntimeBudgetApproved simulatorRuntimeBudgetStatus = "approved"
	simulatorRuntimeBudgetRejected simulatorRuntimeBudgetStatus = "unapproved"
)

type simulatorRuntimeBudgetStatus string

type simulatorRuntimeBudgetEvidence struct {
	Status         simulatorRuntimeBudgetStatus
	OrganizationID string
	ScenarioID     string
	ApprovalID     string
}

type simulatorRuntimePolicyEvidence struct {
	Resolved       bool
	OrganizationID string
	TargetID       string
	Digest         string
}

type simulatorRuntimeNorthstarAssetEvidence struct {
	Resolved bool
	Missing  []string
}

type simulatorRuntimeBudgetInput struct {
	Phase        string `json:"phase"`
	Step         int    `json:"step"`
	Action       string `json:"action"`
	FanoutCount  *int   `json:"fanout_count,omitempty"`
	ServiceCount *int   `json:"service_count,omitempty"`
}

type simulatorRuntimePlanInput struct {
	OrganizationID           string
	Organization             simulatorOrganization
	Scenario                 simulatorScenario
	Manifest                 simulatorManifest
	AuthorizationScopeDigest string
	Budget                   *simulatorRuntimeBudgetEvidence
	TopologyResolved         bool
	Policy                   simulatorRuntimePolicyEvidence
	NorthstarAssets          simulatorRuntimeNorthstarAssetEvidence
}

type simulatorRuntimePlan struct {
	Status                   simulatorRuntimePlanStatus     `json:"status"`
	OrganizationID           string                         `json:"organization_id"`
	ScenarioID               string                         `json:"scenario_id"`
	ManifestName             string                         `json:"manifest_name"`
	MachineIDs               []string                       `json:"machine_ids"`
	Topology                 *simulatorScenarioTopology     `json:"topology,omitempty"`
	Workloads                []simulatorRuntimePlanWorkload `json:"workloads"`
	BudgetInputs             []simulatorRuntimeBudgetInput  `json:"budget_inputs"`
	BudgetStatus             simulatorRuntimeBudgetStatus   `json:"budget_status"`
	TopologyStatus           string                         `json:"topology_status"`
	PolicyStatus             string                         `json:"policy_status"`
	NorthstarAssetsStatus    string                         `json:"northstar_assets_status"`
	PackageContractStatus    string                         `json:"package_contract_status"`
	Blockers                 []string                       `json:"blockers"`
	AuthorizationScopeDigest string                         `json:"authorization_scope_digest"`
}

type simulatorRuntimePlanWorkload struct {
	MachineID   string `json:"machine_id"`
	ComponentID string `json:"component_id"`
	Agent       string `json:"agent"`
}

// buildSimulatorRuntimePlan describes a plan only; it does not authorize or
// execute scenario actions. Evidence fields must be supplied by their owners.
func buildSimulatorRuntimePlan(input simulatorRuntimePlanInput) simulatorRuntimePlan {
	plan := simulatorRuntimePlan{
		Status:                   simulatorRuntimePlanBlocked,
		OrganizationID:           input.OrganizationID,
		ScenarioID:               input.Scenario.ID,
		ManifestName:             input.Manifest.Name,
		MachineIDs:               []string{},
		Workloads:                []simulatorRuntimePlanWorkload{},
		BudgetInputs:             []simulatorRuntimeBudgetInput{},
		BudgetStatus:             simulatorRuntimeBudgetAbsent,
		TopologyStatus:           "missing",
		PolicyStatus:             "unresolved",
		NorthstarAssetsStatus:    "not-required",
		PackageContractStatus:    "missing",
		Blockers:                 []string{},
		AuthorizationScopeDigest: input.AuthorizationScopeDigest,
	}
	blockerSet := make(map[string]struct{})
	addBlocker := func(message string) {
		if _, exists := blockerSet[message]; exists {
			return
		}
		blockerSet[message] = struct{}{}
		plan.Blockers = append(plan.Blockers, message)
	}

	if !validScenarioID(input.OrganizationID) {
		addBlocker("selected organization ID is invalid")
	}
	if input.Organization.ID != input.OrganizationID {
		addBlocker("organization identity does not match the selected organization")
	}
	if input.Scenario.OrganizationID != input.OrganizationID {
		addBlocker("scenario must be explicitly bound to the selected organization")
	}
	if input.Manifest.OrganizationID != input.OrganizationID {
		addBlocker("manifest organization does not match the selected organization")
	}
	if !validScenarioID(input.Scenario.ID) ||
		strings.TrimSpace(input.Scenario.Name) == "" ||
		strings.TrimSpace(input.Scenario.Description) == "" ||
		len(input.Scenario.Steps) == 0 {
		addBlocker("scenario identity and at least one step are required")
	}
	if strings.TrimSpace(input.Manifest.Name) == "" {
		addBlocker("manifest identity is required")
	}
	if err := validateSimulatorOrganization(input.Organization); err != nil {
		addBlocker("organization profile is invalid: " + err.Error())
	}
	if err := validateSimulatorManifest(input.Manifest); err != nil {
		addBlocker("manifest is invalid: " + err.Error())
	}
	validateSimulatorRuntimePlanActions(input, addBlocker)

	if input.Scenario.Topology == nil {
		plan.TopologyStatus = "missing"
		addBlocker("scenario topology is missing")
	} else if err := validateSimulatorScenarioTopology(*input.Scenario.Topology); err != nil {
		plan.TopologyStatus = "invalid"
		addBlocker("scenario topology is invalid: " + err.Error())
	} else if !input.TopologyResolved {
		plan.TopologyStatus = "unresolved"
		addBlocker("scenario topology has not been resolved")
	} else {
		plan.TopologyStatus = "resolved"
		plan.Topology = cloneSimulatorRuntimePlanTopology(input.Scenario.Topology)
	}

	if input.Policy.Resolved && input.Policy.OrganizationID == input.OrganizationID {
		plan.PolicyStatus = "resolved"
	} else if input.Policy.Resolved {
		plan.PolicyStatus = "mismatched"
		addBlocker("resolved policy does not match the selected organization")
	} else {
		addBlocker("policy resolution is required")
	}

	if input.Budget == nil {
		plan.BudgetStatus = simulatorRuntimeBudgetAbsent
		addBlocker("approved budget evidence is absent")
	} else {
		switch input.Budget.Status {
		case simulatorRuntimeBudgetApproved:
			plan.BudgetStatus = simulatorRuntimeBudgetApproved
			if input.Budget.ApprovalID == "" {
				addBlocker("budget approval reference is required")
			}
			if input.Budget.OrganizationID != input.OrganizationID ||
				input.Budget.ScenarioID != input.Scenario.ID {
				addBlocker("budget approval does not match the selected organization and scenario")
			}
		case simulatorRuntimeBudgetRejected:
			plan.BudgetStatus = simulatorRuntimeBudgetRejected
			addBlocker("budget is not approved")
		default:
			plan.BudgetStatus = simulatorRuntimeBudgetUnknown
			addBlocker("budget approval status is absent or unrecognized")
		}
	}

	if input.OrganizationID == "northstar" {
		switch {
		case len(input.NorthstarAssets.Missing) > 0:
			plan.NorthstarAssetsStatus = "missing"
		case input.NorthstarAssets.Resolved:
			plan.NorthstarAssetsStatus = "resolved"
		default:
			plan.NorthstarAssetsStatus = "unresolved"
			addBlocker("Northstar asset availability has not been resolved")
		}
		for _, asset := range sortedUniqueStrings(input.NorthstarAssets.Missing) {
			addBlocker("Northstar asset is missing: " + asset)
		}
		if input.NorthstarAssets.Resolved && len(input.NorthstarAssets.Missing) > 0 {
			addBlocker("Northstar asset resolution reports missing assets")
		}
	}

	if input.Organization.EvaluationRuntime != nil {
		if err := validateEvaluationScenario(input.Scenario, input.Organization); err != nil {
			plan.PackageContractStatus = "unsupported"
			addBlocker("evaluation runtime contract: " + err.Error())
		} else {
			plan.PackageContractStatus = "evaluation-endpoints-v1"
		}
	} else if input.Organization.PackagedProvisioning == nil {
		plan.PackageContractStatus = "missing"
		addBlocker("version 2 shared-approved-bootstrap OciWeb package contract is required")
	} else if err := validatePackagedProvisioning(input.Organization.PackagedProvisioning); err != nil {
		plan.PackageContractStatus = "unsupported"
		addBlocker("packaged provisioning contract is unsupported: " + err.Error())
	} else {
		plan.PackageContractStatus = "supported"
	}

	machineSet := make(map[string]struct{}, len(input.Manifest.Machines))
	for _, machine := range input.Manifest.Machines {
		machineSet[machine.ID] = struct{}{}
	}
	referencedMachines := simulatorRuntimePlanReferencedMachines(input)
	plan.MachineIDs = referencedMachines
	if len(referencedMachines) > maxScenarioMachines {
		addBlocker(fmt.Sprintf("scenario references %d machines; the limit is %d", len(referencedMachines), maxScenarioMachines))
	}
	for _, machineID := range referencedMachines {
		if !validScenarioID(machineID) {
			addBlocker(fmt.Sprintf("scenario references invalid machine ID %q", machineID))
		}
		if _, exists := machineSet[machineID]; !exists {
			addBlocker(fmt.Sprintf("scenario references machine %q absent from the manifest", machineID))
		}
		if packageContainsMachine(input.Organization.PackagedProvisioning, machineID) == false &&
			input.Organization.PackagedProvisioning != nil {
			addBlocker(fmt.Sprintf("packaged provisioning does not declare scenario machine %q", machineID))
		}
	}

	plan.Workloads = simulatorRuntimePlanWorkloads(input, addBlocker)
	plan.BudgetInputs = simulatorRuntimePlanBudgetInputs(input)
	sort.Strings(plan.Blockers)
	if len(plan.Blockers) == 0 {
		plan.Status = simulatorRuntimePlanOnly
	}
	return plan
}

func simulatorRuntimePlanReferencedMachines(input simulatorRuntimePlanInput) []string {
	machines := make(map[string]struct{})
	add := func(machineID string) {
		if machineID != "" {
			machines[machineID] = struct{}{}
		}
	}
	for _, step := range append(append([]simulatorScenarioStep(nil), input.Scenario.Steps...), input.Scenario.Cleanup...) {
		add(step.Machine)
	}
	result := make([]string, 0, len(machines))
	for machineID := range machines {
		result = append(result, machineID)
	}
	sort.Strings(result)
	return result
}

func validateSimulatorRuntimePlanActions(input simulatorRuntimePlanInput, addBlocker func(string)) {
	validateSteps := func(steps []simulatorScenarioStep, cleanup bool) {
		for index, step := range steps {
			stepLabel := fmt.Sprintf("scenario step %d", index+1)
			if cleanup {
				stepLabel = fmt.Sprintf("cleanup step %d", index+1)
				switch step.Action {
				case "stop_test_service", "stop_service_fleet", "stop_workload", "logout", "stop_machine":
				default:
					addBlocker(fmt.Sprintf("%s uses unsupported cleanup action %q", stepLabel, step.Action))
				}
			}

			if step.ClientCount != 0 && step.Action != "stress_traffic" {
				addBlocker(fmt.Sprintf("%s client-count override is only valid for stress traffic", stepLabel))
			}
			if step.TimeoutSeconds < 0 || step.TimeoutSeconds > 180 {
				addBlocker(fmt.Sprintf("%s timeout is outside the supported range", stepLabel))
			}
			switch step.Action {
			case evaluationScenarioAction:
				if err := validateEvaluationScenario(input.Scenario, input.Organization); err != nil {
					addBlocker(err.Error())
				}
			case "verify_packaged_demo":
				if cleanup || step.Machine != "" || step.Component != "" || step.Target != "" ||
					step.Expected != "" || step.User != "" || len(step.Workloads) != 0 {
					addBlocker(fmt.Sprintf("%s contains scenario-supplied inputs not permitted for packaged demo verification", stepLabel))
				}
				if step.TimeoutSeconds < 30 {
					addBlocker(fmt.Sprintf("%s packaged demo verification requires a 30-second minimum timeout", stepLabel))
				}
				addBlocker("packaged demo probe script availability has not been resolved")
			case "verify_multinode_runtime":
				if cleanup || input.Organization.Runtime.Driver != "docker-multinode" ||
					input.Organization.Runtime.Topology != "multi-node" ||
					len(input.Organization.Runtime.Nodes) < 2 {
					addBlocker(fmt.Sprintf("%s requires a multi-node docker-multinode organization", stepLabel))
				}
			case "start_machine", "wait_controller", "login", "select_workloads", "logout", "stop_machine",
				"start_workload", "stop_workload", "start_test_service", "stop_test_service",
				"request_business_service", "request_test_service", "benchmark_test_service",
				"request_web_gateway", "resolve_dns":
				if step.Machine == "" {
					addBlocker(fmt.Sprintf("%s action %q requires an explicit machine", stepLabel, step.Action))
				}
			case "traffic":
				component, found := simulatorRuntimePlanComponent(input, step.Component)
				if !found {
					addBlocker(fmt.Sprintf("%s references a missing traffic component", stepLabel))
					break
				}
				if step.Machine == "" {
					if component.Namespace == "" {
						addBlocker(fmt.Sprintf("%s machine-free traffic requires a component namespace", stepLabel))
					}
				}
				target := step.Target
				if target == "" {
					target = component.Target
				}
				address := net.ParseIP(target)
				if address == nil || address.To4() != nil {
					addBlocker(fmt.Sprintf("%s traffic target must be a valid IPv6 address", stepLabel))
				}
				if step.Expected != "allow" && step.Expected != "deny" {
					addBlocker(fmt.Sprintf("%s traffic requires an explicit allow or deny expectation", stepLabel))
				}
			case "delay":
				if step.TimeoutSeconds < 1 || step.TimeoutSeconds > 30 {
					addBlocker(fmt.Sprintf("%s delay timeout must be between 1 and 30 seconds", stepLabel))
				}
			case "start_service_fleet", "stop_service_fleet":
				addBlocker("load-test fanout is not executable from a runtime plan")
				profile := input.Organization.LoadTest
				if profile == nil || step.Machine == "" || step.Machine != profile.ServiceMachine {
					addBlocker(fmt.Sprintf("%s service-fleet action must use the configured service machine", stepLabel))
				}
			case "stress_traffic":
				addBlocker("load-test fanout is not executable from a runtime plan")
				profile := input.Organization.LoadTest
				if profile == nil || step.Machine == "" || step.Machine != profile.ClientMachine ||
					step.Component != profile.ClientWorkload || step.Target != profile.ServiceWorkload {
					addBlocker(fmt.Sprintf("%s stress traffic must match the configured client and service workloads", stepLabel))
				}
				if step.ClientCount != 0 && (step.ClientCount < 50 || step.ClientCount > 512) {
					addBlocker(fmt.Sprintf("%s stress traffic client count is outside the supported range", stepLabel))
				}
				if _, found := simulatorRuntimePlanComponent(input, step.Component); !found {
					addBlocker(fmt.Sprintf("%s stress traffic component is missing", stepLabel))
				}
			default:
				addBlocker(fmt.Sprintf("%s uses unsupported action %q", stepLabel, step.Action))
			}
		}
	}
	validateSteps(input.Scenario.Steps, false)
	validateSteps(input.Scenario.Cleanup, true)
}

func simulatorRuntimePlanBudgetInputs(input simulatorRuntimePlanInput) []simulatorRuntimeBudgetInput {
	profile := input.Organization.LoadTest
	requests := make([]simulatorRuntimeBudgetInput, 0)
	phases := []struct {
		name  string
		steps []simulatorScenarioStep
	}{
		{name: "step", steps: input.Scenario.Steps},
		{name: "cleanup", steps: input.Scenario.Cleanup},
	}
	for _, phase := range phases {
		for index, step := range phase.steps {
			switch step.Action {
			case evaluationScenarioAction:
				if contract := input.Organization.EvaluationRuntime; contract != nil {
					requests = append(requests, simulatorRuntimeBudgetInput{Phase: phase.name, Step: index + 1,
						Action: step.Action, FanoutCount: positiveSimulatorRuntimePlanCount(contract.EndpointCount),
						ServiceCount: positiveSimulatorRuntimePlanCount(contract.ListenerCount)})
				}
			case "stress_traffic":
				request := simulatorRuntimeBudgetInput{Phase: phase.name, Step: index + 1, Action: step.Action}
				fanoutCount := step.ClientCount
				if fanoutCount == 0 && profile != nil {
					fanoutCount = profile.ClientCount
				}
				request.FanoutCount = positiveSimulatorRuntimePlanCount(fanoutCount)
				if profile != nil {
					request.ServiceCount = positiveSimulatorRuntimePlanCount(profile.ServiceCount)
				}
				requests = append(requests, request)
			case "start_service_fleet", "stop_service_fleet":
				request := simulatorRuntimeBudgetInput{Phase: phase.name, Step: index + 1, Action: step.Action}
				if profile != nil {
					request.ServiceCount = positiveSimulatorRuntimePlanCount(profile.ServiceCount)
				}
				requests = append(requests, request)
			}
		}
	}
	return requests
}

func positiveSimulatorRuntimePlanCount(count int) *int {
	if count <= 0 {
		return nil
	}
	return &count
}

func simulatorRuntimePlanComponent(input simulatorRuntimePlanInput, name string) (simulatorComponent, bool) {
	var result simulatorComponent
	found := false
	for _, component := range simulatorRuntimePlanComponents(input) {
		if component.Name != name {
			continue
		}
		if found {
			return simulatorComponent{}, false
		}
		result, found = component, true
	}
	return result, found
}

func simulatorRuntimePlanComponents(input simulatorRuntimePlanInput) []simulatorComponent {
	components := input.Manifest.Components
	if input.Scenario.Topology != nil && input.Scenario.Topology.Components != nil {
		components = input.Scenario.Topology.Components
	}
	return simulatorRuntimePlanOrganizationComponents(input.Organization, components)
}

func simulatorRuntimePlanOrganizationComponents(
	organization simulatorOrganization,
	components []simulatorComponent,
) []simulatorComponent {
	result := append([]simulatorComponent(nil), components...)
	known := make(map[string]struct{}, len(components))
	for _, component := range components {
		known[component.Name] = struct{}{}
	}

	workloadNames := make([]string, 0, len(organization.WorkloadServices))
	for name := range organization.WorkloadServices {
		workloadNames = append(workloadNames, name)
	}
	sort.Strings(workloadNames)
	for _, name := range workloadNames {
		if _, exists := known[name]; exists {
			continue
		}
		if component, ok := businessComponent(organization, name); ok {
			result = append(result, component)
			known[name] = struct{}{}
		}
	}
	return result
}

func simulatorRuntimePlanWorkloads(input simulatorRuntimePlanInput, addBlocker func(string)) []simulatorRuntimePlanWorkload {
	components := simulatorRuntimePlanComponents(input)
	componentByName := make(map[string]simulatorComponent, len(components))
	duplicateNames := make(map[string]struct{})
	for _, component := range components {
		if _, exists := componentByName[component.Name]; exists {
			duplicateNames[component.Name] = struct{}{}
		}
		componentByName[component.Name] = component
	}
	declared := make(map[string]simulatorRuntimePlanWorkload)
	addWorkload := func(machineID, componentID string) {
		if _, duplicate := duplicateNames[componentID]; duplicate {
			addBlocker(fmt.Sprintf("workload component %q is ambiguous", componentID))
			return
		}
		component, exists := componentByName[componentID]
		if !exists {
			addBlocker(fmt.Sprintf("workload component %q is missing", componentID))
			return
		}
		agent := component.Agent
		if agent == "" {
			agent = component.Name
		}
		if _, supported := machineWorkload(agent); !supported {
			addBlocker(fmt.Sprintf("workload component %q has no fixed workload implementation", componentID))
			return
		}
		key := machineID + "\x00" + componentID
		declared[key] = simulatorRuntimePlanWorkload{
			MachineID: machineID, ComponentID: componentID, Agent: agent,
		}
	}

	for _, steps := range [][]simulatorScenarioStep{input.Scenario.Steps, input.Scenario.Cleanup} {
		for _, step := range steps {
			switch step.Action {
			case "start_workload", "stop_workload":
				addWorkload(step.Machine, step.Component)
			case "select_workloads":
				seen := make(map[string]struct{}, len(step.Workloads))
				for _, workload := range step.Workloads {
					if _, duplicate := seen[workload]; duplicate {
						addBlocker(fmt.Sprintf("scenario selects workload %q more than once", workload))
						continue
					}
					seen[workload] = struct{}{}
					addWorkload(step.Machine, workload)
				}
			}
		}
	}

	result := make([]simulatorRuntimePlanWorkload, 0, len(declared))
	for _, workload := range declared {
		result = append(result, workload)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].MachineID != result[j].MachineID {
			return result[i].MachineID < result[j].MachineID
		}
		return result[i].ComponentID < result[j].ComponentID
	})
	return result
}

func packageContainsMachine(config *simulatorPackagedProvisioning, machineID string) bool {
	if config == nil {
		return false
	}
	for _, configuredMachine := range config.Machines {
		if configuredMachine == machineID {
			return true
		}
	}
	return false
}

func cloneSimulatorRuntimePlanTopology(topology *simulatorScenarioTopology) *simulatorScenarioTopology {
	if topology == nil {
		return nil
	}
	cloned := &simulatorScenarioTopology{
		Nodes:      append([]simulatorTopologyNode(nil), topology.Nodes...),
		Links:      append([]simulatorTopologyLink(nil), topology.Links...),
		Components: append([]simulatorComponent(nil), topology.Components...),
	}
	for index := range cloned.Components {
		cloned.Components[index].Identities = append([]simulatorIdentity(nil), topology.Components[index].Identities...)
	}
	return cloned
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
