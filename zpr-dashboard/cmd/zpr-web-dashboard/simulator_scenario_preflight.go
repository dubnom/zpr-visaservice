package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
)

const simulatorScenarioPlanRequestLimit = 8 << 10

type simulatorScenarioPlanRequest struct {
	OrganizationID string `json:"organization_id"`
	ScenarioID     string `json:"scenario_id"`
}

type simulatorScenarioPlanApprovalRequest struct {
	OrganizationID string `json:"organization_id"`
	ScenarioID     string `json:"scenario_id"`
	PlanDigest     string `json:"plan_digest"`
	Approved       bool   `json:"approved"`
}

type simulatorScenarioPlanResponse struct {
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
	PlanDigest               string                         `json:"plan_digest"`
	AuthorizationScopeDigest string                         `json:"authorization_scope_digest"`
}

type simulatorScenarioBudgetEvidenceUnavailableResponse struct {
	OrganizationID           string   `json:"organization_id"`
	ScenarioID               string   `json:"scenario_id"`
	AuthorizationScopeDigest string   `json:"authorization_scope_digest"`
	Blockers                 []string `json:"blockers"`
}

type simulatorScenarioPlanApprovalResponse struct {
	Approved                 bool     `json:"approved"`
	ApprovalID               string   `json:"approval_id"`
	OrganizationID           string   `json:"organization_id"`
	ScenarioID               string   `json:"scenario_id"`
	PlanDigest               string   `json:"plan_digest"`
	AuthorizationScopeDigest string   `json:"authorization_scope_digest"`
	MachineIDs               []string `json:"machine_ids"`
}

type simulatorScenarioPlanBuilder func(organizationID, scenarioID string) (simulatorRuntimePlan, error)
type simulatorScenarioPlanOrganizationResolver func(*http.Request) (string, error)
type simulatorScenarioCurrentPolicyResolver func(context.Context, string, string) (simulatorRuntimePolicyEvidence, error)

type simulatorScenarioPlanHandler struct {
	buildPlan            simulatorScenarioPlanBuilder
	resolveOrganization  simulatorScenarioPlanOrganizationResolver
	resolveCurrentPolicy simulatorScenarioCurrentPolicyResolver
	approvals            *simulatorScenarioPlanApprovalStore
	manager              *simulatorScenarioManager
}

type simulatorScenarioPlanApproval struct {
	ID         string
	Plan       simulatorRuntimePlan
	PlanDigest string
}

type simulatorScenarioPlanApprovalStore struct {
	mu             sync.Mutex
	byID           map[string]simulatorScenarioPlanApproval
	pendingByScope map[string]string
}

var simulatorScenarioPlanApprovals = newSimulatorScenarioPlanApprovalStore()

func newSimulatorScenarioPlanHandler() *simulatorScenarioPlanHandler {
	return &simulatorScenarioPlanHandler{
		buildPlan:           buildCurrentSimulatorScenarioPlan,
		resolveOrganization: currentSimulatorOrganization,
		approvals:           simulatorScenarioPlanApprovals,
		manager:             activeSimulatorScenario,
	}
}

func newSimulatorScenarioPlanApprovalStore() *simulatorScenarioPlanApprovalStore {
	return &simulatorScenarioPlanApprovalStore{
		byID:           make(map[string]simulatorScenarioPlanApproval),
		pendingByScope: make(map[string]string),
	}
}

func (h *simulatorScenarioPlanHandler) Preflight(w http.ResponseWriter, r *http.Request) {
	var request simulatorScenarioPlanRequest
	if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
		return
	}
	if !validScenarioID(request.OrganizationID) || !validScenarioID(request.ScenarioID) {
		http.Error(w, "valid organization_id and scenario_id are required", http.StatusBadRequest)
		return
	}
	if !h.requestOrganizationMatches(w, r, request.OrganizationID) {
		return
	}
	plan, err := h.buildPlan(request.OrganizationID, request.ScenarioID)
	if err != nil {
		writeSimulatorScenarioPlanBuildError(w, err)
		return
	}
	if !simulatorScenarioPlanIdentityMatches(plan, request.OrganizationID, request.ScenarioID) {
		http.Error(w, "preflight returned mismatched organization or scenario identity", http.StatusConflict)
		return
	}
	h.resolveNorthstarPolicyReadback(r.Context(), &plan)
	if err := h.resolvePlanBudgetEvidence(r.Context(), &plan); err != nil {
		writeSimulatorScenarioBudgetEvidenceUnavailable(w, plan)
		return
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		http.Error(w, "scenario plan could not be digested", http.StatusInternalServerError)
		return
	}
	writeSimulatorJSON(w, simulatorScenarioPlanResponseFor(plan, digest))
}

func writeSimulatorScenarioBudgetEvidenceUnavailable(
	w http.ResponseWriter,
	plan simulatorRuntimePlan,
) {
	if !validSimulatorScenarioAuthorizationScopeDigest(plan.AuthorizationScopeDigest) {
		http.Error(w, "current approved budget evidence is unavailable or invalid", http.StatusServiceUnavailable)
		return
	}
	const blocker = "current approved budget evidence is unavailable or invalid"
	blockers := append([]string(nil), plan.Blockers...)
	found := false
	for _, existing := range blockers {
		if existing == blocker {
			found = true
			break
		}
	}
	if !found {
		blockers = append(blockers, blocker)
	}
	response := simulatorScenarioBudgetEvidenceUnavailableResponse{
		OrganizationID:           plan.OrganizationID,
		ScenarioID:               plan.ScenarioID,
		AuthorizationScopeDigest: plan.AuthorizationScopeDigest,
		Blockers:                 blockers,
	}
	content, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "budget evidence failure response could not be encoded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	if _, err := w.Write(append(content, '\n')); err != nil {
		log.Printf("write Simulator budget evidence failure response: %v", err)
	}
}

func (h *simulatorScenarioPlanHandler) Approve(w http.ResponseWriter, r *http.Request) {
	var request simulatorScenarioPlanApprovalRequest
	if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
		return
	}
	if !validScenarioID(request.OrganizationID) || !validScenarioID(request.ScenarioID) ||
		request.PlanDigest == "" {
		http.Error(w, "organization_id, scenario_id, and plan_digest are required", http.StatusBadRequest)
		return
	}
	if !request.Approved {
		http.Error(w, "approved must be true to approve a scenario plan", http.StatusBadRequest)
		return
	}
	if !h.requestOrganizationMatches(w, r, request.OrganizationID) {
		return
	}
	plan, err := h.buildPlan(request.OrganizationID, request.ScenarioID)
	if err != nil {
		writeSimulatorScenarioPlanBuildError(w, err)
		return
	}
	if !simulatorScenarioPlanIdentityMatches(plan, request.OrganizationID, request.ScenarioID) {
		http.Error(w, "preflight returned mismatched organization or scenario identity", http.StatusConflict)
		return
	}
	h.resolveNorthstarPolicyReadback(r.Context(), &plan)
	if err := h.resolvePlanBudgetEvidence(r.Context(), &plan); err != nil {
		http.Error(w, "current approved budget evidence is unavailable or invalid", http.StatusServiceUnavailable)
		return
	}
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		http.Error(w, "scenario plan could not be digested", http.StatusInternalServerError)
		return
	}
	if request.PlanDigest != digest {
		http.Error(w, "scenario plan changed; run preflight again before approval", http.StatusConflict)
		return
	}
	if plan.Status != simulatorRuntimePlanOnly ||
		plan.BudgetStatus != simulatorRuntimeBudgetApproved || len(plan.Blockers) != 0 {
		http.Error(w, "blocked scenario plan or unapproved budget cannot be approved", http.StatusConflict)
		return
	}
	if h.approvals == nil {
		http.Error(w, "scenario plan approval store is unavailable", http.StatusServiceUnavailable)
		return
	}
	approvalID, err := h.approvals.issue(plan)
	if err != nil {
		if errors.Is(err, errSimulatorScenarioPlanApprovalPending) {
			http.Error(w, "an unconsumed approval already exists for this plan", http.StatusConflict)
			return
		}
		http.Error(w, "scenario plan approval could not be issued", http.StatusInternalServerError)
		return
	}
	writeSimulatorJSON(w, simulatorScenarioPlanApprovalResponse{
		Approved: true, ApprovalID: approvalID,
		OrganizationID: plan.OrganizationID, ScenarioID: plan.ScenarioID,
		PlanDigest: digest, AuthorizationScopeDigest: plan.AuthorizationScopeDigest,
		MachineIDs: append([]string(nil), plan.MachineIDs...),
	})
}

func (h *simulatorScenarioPlanHandler) resolvePlanBudgetEvidence(
	ctx context.Context,
	plan *simulatorRuntimePlan,
) error {
	if plan == nil || plan.AuthorizationScopeDigest == "" {
		return errors.New("scenario authorization scope digest is unavailable")
	}
	if h == nil || h.manager == nil {
		return budgetauthority.ErrUnavailable
	}
	scope := budgetauthority.Scope{
		OrganizationID: plan.OrganizationID,
		ScenarioID:     plan.ScenarioID,
		PlanDigest:     plan.AuthorizationScopeDigest,
	}
	if _, err := h.manager.resolveCurrentBudgetEvidence(ctx, scope); err != nil {
		return err
	}
	plan.BudgetStatus = simulatorRuntimeBudgetApproved
	plan.Blockers = withoutSimulatorRuntimeBudgetEvidenceBlockers(plan.Blockers)
	if len(plan.Blockers) == 0 {
		plan.Status = simulatorRuntimePlanOnly
	} else {
		plan.Status = simulatorRuntimePlanBlocked
	}
	return nil
}

func (h *simulatorScenarioPlanHandler) resolveNorthstarPolicyReadback(ctx context.Context, plan *simulatorRuntimePlan) {
	if plan == nil || plan.OrganizationID != "northstar" {
		return
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), plan.OrganizationID)
	if err != nil || !organization.SyntheticLabOnly {
		return
	}
	var resolver simulatorScenarioCurrentPolicyResolver
	if h != nil {
		resolver = h.resolveCurrentPolicy
	}
	resolveSimulatorNorthstarPolicyReadback(ctx, plan, organization, resolver)
}

func resolveSimulatorNorthstarPolicyReadback(
	ctx context.Context,
	plan *simulatorRuntimePlan,
	organization simulatorOrganization,
	resolver simulatorScenarioCurrentPolicyResolver,
) {
	if plan == nil || !organization.SyntheticLabOnly || organization.ID != "northstar" {
		return
	}
	if err := validateSimulatorOrganization(organization); err != nil {
		setSimulatorRuntimePolicyReadback(plan, "mismatched", "Northstar synthetic lab policy identity is invalid")
		return
	}
	if resolver == nil {
		setSimulatorRuntimePolicyReadback(plan, "unresolved", "active Northstar policy readback is unavailable")
		return
	}
	evidence, err := resolver(ctx, organization.ID, organization.ActivePolicyTargetID)
	if err != nil {
		setSimulatorRuntimePolicyReadback(plan, "unresolved", "active Northstar policy readback is unavailable")
		return
	}
	if !simulatorNorthstarPolicyReadbackMatches(organization, evidence) {
		setSimulatorRuntimePolicyReadback(plan, "mismatched", "active policy readback does not match the Northstar lab identity")
		return
	}
	setSimulatorRuntimePolicyReadback(plan, "resolved", "")
}

func simulatorNorthstarPolicyReadbackMatches(
	organization simulatorOrganization,
	evidence simulatorRuntimePolicyEvidence,
) bool {
	return organization.SyntheticLabOnly && evidence.Resolved &&
		evidence.OrganizationID == organization.ID &&
		evidence.TargetID == organization.ActivePolicyTargetID &&
		evidence.Digest == organization.ActivePolicyDigest
}

func verifySimulatorNorthstarPolicyReadback(ctx context.Context, organization simulatorOrganization) error {
	if !organization.SyntheticLabOnly {
		return nil
	}
	if organization.ID != "northstar" || validateSimulatorOrganization(organization) != nil {
		return errors.New("synthetic Northstar policy identity is invalid")
	}
	client, err := newSimulatorPolicyStatusClientFromEnv()
	if err != nil {
		return err
	}
	if client == nil {
		return errors.New("Simulator policy current-state client is unavailable")
	}
	defer client.client.CloseIdleConnections()
	current, err := client.current(ctx, organization.ID, organization.ActivePolicyTargetID)
	if err != nil {
		return err
	}
	evidence := simulatorRuntimePolicyEvidence{
		Resolved: true, OrganizationID: current.Organization,
		TargetID: current.TargetID, Digest: current.CurrentDigest,
	}
	if !simulatorNorthstarPolicyReadbackMatches(organization, evidence) {
		return errors.New("active policy readback does not match the Northstar lab identity")
	}
	return nil
}

func setSimulatorRuntimePolicyReadback(plan *simulatorRuntimePlan, status, blocker string) {
	if plan == nil {
		return
	}
	plan.PolicyStatus = status
	filtered := make([]string, 0, len(plan.Blockers)+1)
	for _, existing := range plan.Blockers {
		switch existing {
		case "policy resolution is required",
			"active Northstar policy readback is unavailable",
			"active policy readback does not match the Northstar lab identity",
			"Northstar synthetic lab policy identity is invalid":
		default:
			filtered = append(filtered, existing)
		}
	}
	if blocker != "" {
		filtered = append(filtered, blocker)
	}
	sort.Strings(filtered)
	plan.Blockers = filtered
	if len(plan.Blockers) == 0 {
		plan.Status = simulatorRuntimePlanOnly
	} else {
		plan.Status = simulatorRuntimePlanBlocked
	}
}

func withoutSimulatorRuntimeBudgetEvidenceBlockers(blockers []string) []string {
	const (
		absent        = "approved budget evidence is absent"
		unapproved    = "budget is not approved"
		missingID     = "budget approval reference is required"
		scopeMismatch = "budget approval does not match the selected organization and scenario"
		unknown       = "budget approval status is absent or unrecognized"
	)
	filtered := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		switch blocker {
		case absent, unapproved, missingID, scopeMismatch, unknown:
		default:
			filtered = append(filtered, blocker)
		}
	}
	return filtered
}

func (h *simulatorScenarioPlanHandler) requestOrganizationMatches(w http.ResponseWriter, r *http.Request, requested string) bool {
	if authorized, ok := simulatorAuthorizedOrganization(r.Context()); ok {
		if authorized != requested {
			http.Error(w, "requested organization is outside the authorized scope", http.StatusForbidden)
			return false
		}
		return true
	}
	if h.resolveOrganization == nil {
		http.Error(w, "Simulator organization context is unavailable", http.StatusServiceUnavailable)
		return false
	}
	organizationID, err := h.resolveOrganization(r)
	if err != nil || organizationID == "" {
		http.Error(w, "Simulator organization context is unavailable", http.StatusServiceUnavailable)
		return false
	}
	if organizationID != requested {
		http.Error(w, "requested organization is outside the active scope", http.StatusForbidden)
		return false
	}
	return true
}

func decodeSimulatorScenarioPlanRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.URL.RawQuery != "" {
		http.Error(w, "query parameters are not accepted", http.StatusBadRequest)
		return false
	}
	if r.Body == nil {
		http.Error(w, "request body is required", http.StatusBadRequest)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, simulatorScenarioPlanRequestLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		http.Error(w, "invalid scenario plan request", http.StatusBadRequest)
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		http.Error(w, "scenario plan request contains trailing data", http.StatusBadRequest)
		return false
	}
	return true
}

func simulatorScenarioPlanIdentityMatches(plan simulatorRuntimePlan, organizationID, scenarioID string) bool {
	return plan.OrganizationID == organizationID && plan.ScenarioID == scenarioID
}

func simulatorScenarioPlanResponseFor(plan simulatorRuntimePlan, digest string) simulatorScenarioPlanResponse {
	plan = cloneSimulatorRuntimePlan(plan)
	return simulatorScenarioPlanResponse{
		Status: plan.Status, OrganizationID: plan.OrganizationID, ScenarioID: plan.ScenarioID,
		ManifestName: plan.ManifestName, MachineIDs: plan.MachineIDs,
		Topology:     plan.Topology,
		Workloads:    plan.Workloads,
		BudgetInputs: plan.BudgetInputs,
		BudgetStatus: plan.BudgetStatus, TopologyStatus: plan.TopologyStatus,
		PolicyStatus: plan.PolicyStatus, NorthstarAssetsStatus: plan.NorthstarAssetsStatus,
		PackageContractStatus: plan.PackageContractStatus,
		Blockers:              plan.Blockers, PlanDigest: digest,
		AuthorizationScopeDigest: plan.AuthorizationScopeDigest,
	}
}

func cloneSimulatorRuntimePlan(plan simulatorRuntimePlan) simulatorRuntimePlan {
	cloned := plan
	if plan.MachineIDs != nil {
		cloned.MachineIDs = make([]string, len(plan.MachineIDs))
		copy(cloned.MachineIDs, plan.MachineIDs)
	}
	cloned.Topology = cloneSimulatorScenarioPlanTopology(plan.Topology)
	if plan.Workloads != nil {
		cloned.Workloads = make([]simulatorRuntimePlanWorkload, len(plan.Workloads))
		copy(cloned.Workloads, plan.Workloads)
	}
	if plan.BudgetInputs != nil {
		cloned.BudgetInputs = make([]simulatorRuntimeBudgetInput, len(plan.BudgetInputs))
		copy(cloned.BudgetInputs, plan.BudgetInputs)
	}
	for index := range cloned.BudgetInputs {
		if fanout := plan.BudgetInputs[index].FanoutCount; fanout != nil {
			value := *fanout
			cloned.BudgetInputs[index].FanoutCount = &value
		}
		if services := plan.BudgetInputs[index].ServiceCount; services != nil {
			value := *services
			cloned.BudgetInputs[index].ServiceCount = &value
		}
	}
	if plan.Blockers != nil {
		cloned.Blockers = make([]string, len(plan.Blockers))
		copy(cloned.Blockers, plan.Blockers)
	}
	return cloned
}

func cloneSimulatorScenarioPlanTopology(topology *simulatorScenarioTopology) *simulatorScenarioTopology {
	if topology == nil {
		return nil
	}
	cloned := &simulatorScenarioTopology{}
	if topology.Nodes != nil {
		cloned.Nodes = make([]simulatorTopologyNode, len(topology.Nodes))
		copy(cloned.Nodes, topology.Nodes)
	}
	if topology.Links != nil {
		cloned.Links = make([]simulatorTopologyLink, len(topology.Links))
		copy(cloned.Links, topology.Links)
	}
	if topology.Components != nil {
		cloned.Components = make([]simulatorComponent, len(topology.Components))
		copy(cloned.Components, topology.Components)
		for index := range topology.Components {
			if topology.Components[index].Identities != nil {
				cloned.Components[index].Identities = make([]simulatorIdentity, len(topology.Components[index].Identities))
				copy(cloned.Components[index].Identities, topology.Components[index].Identities)
			}
		}
	}
	return cloned
}

func simulatorScenarioPlanDigest(plan simulatorRuntimePlan) (string, error) {
	content, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

const simulatorScenarioAuthorizationScopeDomain = "zpr:simulator:scenario-authorization-scope:v1\x00"

type simulatorScenarioAuthorizationScopeSnapshot struct {
	OrganizationID string                `json:"organization_id"`
	Organization   simulatorOrganization `json:"organization"`
	Scenario       simulatorScenario     `json:"scenario"`
	Manifest       simulatorManifest     `json:"manifest"`
}

func simulatorScenarioAuthorizationScopeDigest(input simulatorRuntimePlanInput) (string, error) {
	snapshot := simulatorScenarioAuthorizationScopeSnapshot{
		OrganizationID: input.OrganizationID,
		Organization:   input.Organization,
		Scenario:       input.Scenario,
		Manifest:       input.Manifest,
	}
	content, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	digestInput := append([]byte(simulatorScenarioAuthorizationScopeDomain), content...)
	digest := sha256.Sum256(digestInput)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func validSimulatorScenarioAuthorizationScopeDigest(digest string) bool {
	const prefix = "sha256:"
	if len(digest) != len(prefix)+sha256.Size*2 ||
		!strings.HasPrefix(digest, prefix) ||
		strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(digest, prefix))
	return err == nil && len(decoded) == sha256.Size
}

var errSimulatorScenarioPlanApprovalPending = errors.New("scenario plan approval is already pending")

func (store *simulatorScenarioPlanApprovalStore) issue(plan simulatorRuntimePlan) (string, error) {
	plan = cloneSimulatorRuntimePlan(plan)
	digest, err := simulatorScenarioPlanDigest(plan)
	if err != nil {
		return "", err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.byID == nil {
		store.byID = make(map[string]simulatorScenarioPlanApproval)
	}
	if store.pendingByScope == nil {
		store.pendingByScope = make(map[string]string)
	}
	scope := simulatorScenarioPlanApprovalScope(plan.OrganizationID, plan.ScenarioID, digest)
	if _, exists := store.pendingByScope[scope]; exists {
		return "", errSimulatorScenarioPlanApprovalPending
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	approvalID := base64.RawURLEncoding.EncodeToString(token)
	if _, exists := store.byID[approvalID]; exists {
		return "", errors.New("scenario plan approval identifier collision")
	}
	approval := simulatorScenarioPlanApproval{
		ID: approvalID, Plan: plan, PlanDigest: digest,
	}
	store.byID[approvalID] = approval
	store.pendingByScope[scope] = approvalID
	return approvalID, nil
}

var (
	errSimulatorScenarioApprovalUnknown       = errors.New("scenario plan approval is unknown or already consumed")
	errSimulatorScenarioApprovalScopeMismatch = errors.New("scenario plan approval scope does not match")
)

func (store *simulatorScenarioPlanApprovalStore) consume(approvalID, organizationID, scenarioID, planDigest string) (simulatorRuntimePlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	approval, err := store.matchingApprovalLocked(approvalID, organizationID, scenarioID, planDigest)
	if err != nil {
		return simulatorRuntimePlan{}, err
	}
	delete(store.byID, approvalID)
	scope := simulatorScenarioPlanApprovalScope(approval.Plan.OrganizationID, approval.Plan.ScenarioID, approval.PlanDigest)
	if store.pendingByScope[scope] == approvalID {
		delete(store.pendingByScope, scope)
	}
	return cloneSimulatorRuntimePlan(approval.Plan), nil
}

func (store *simulatorScenarioPlanApprovalStore) lookup(approvalID, organizationID, scenarioID, planDigest string) (simulatorRuntimePlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	approval, err := store.matchingApprovalLocked(approvalID, organizationID, scenarioID, planDigest)
	if err != nil {
		return simulatorRuntimePlan{}, err
	}
	return cloneSimulatorRuntimePlan(approval.Plan), nil
}

func (store *simulatorScenarioPlanApprovalStore) matchingApprovalLocked(
	approvalID, organizationID, scenarioID, planDigest string,
) (simulatorScenarioPlanApproval, error) {
	approval, exists := store.byID[approvalID]
	if !exists {
		return simulatorScenarioPlanApproval{}, errSimulatorScenarioApprovalUnknown
	}
	if approval.ID != approvalID ||
		approval.Plan.OrganizationID != organizationID ||
		approval.Plan.ScenarioID != scenarioID ||
		approval.PlanDigest != planDigest {
		return simulatorScenarioPlanApproval{}, errSimulatorScenarioApprovalScopeMismatch
	}
	actualDigest, err := simulatorScenarioPlanDigest(approval.Plan)
	if err != nil {
		return simulatorScenarioPlanApproval{}, err
	}
	if actualDigest != approval.PlanDigest {
		return simulatorScenarioPlanApproval{}, errSimulatorScenarioApprovalScopeMismatch
	}
	return approval, nil
}

func consumeSimulatorScenarioApproval(approvalID, organizationID, scenarioID, planDigest string) (simulatorRuntimePlan, error) {
	return simulatorScenarioPlanApprovals.consume(approvalID, organizationID, scenarioID, planDigest)
}

func lookupSimulatorScenarioApproval(approvalID, organizationID, scenarioID, planDigest string) (simulatorRuntimePlan, error) {
	return simulatorScenarioPlanApprovals.lookup(approvalID, organizationID, scenarioID, planDigest)
}

func simulatorScenarioPlanApprovalScope(organizationID, scenarioID, planDigest string) string {
	return organizationID + "\x00" + scenarioID + "\x00" + planDigest
}

type simulatorScenarioPlanBuildFailure struct {
	status  int
	message string
}

func (failure *simulatorScenarioPlanBuildFailure) Error() string { return failure.message }

func writeSimulatorScenarioPlanBuildError(w http.ResponseWriter, err error) {
	var failure *simulatorScenarioPlanBuildFailure
	if errors.As(err, &failure) {
		http.Error(w, failure.message, failure.status)
		return
	}
	http.Error(w, "scenario plan input is unavailable", http.StatusServiceUnavailable)
}

func buildCurrentSimulatorScenarioPlan(organizationID, scenarioID string) (simulatorRuntimePlan, error) {
	manifest, err := readSimulatorManifest()
	if err != nil {
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusServiceUnavailable, message: "simulation manifest is unavailable",
		}
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID)
	if err != nil {
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusServiceUnavailable, message: "organization profile is unavailable",
		}
	}
	manifest.OrganizationID = organizationID
	scenario, err := readExplicitSimulatorScenario(scenarioID)
	if err != nil {
		var failure *simulatorScenarioPlanBuildFailure
		if errors.As(err, &failure) {
			return simulatorRuntimePlan{}, failure
		}
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusUnprocessableEntity, message: "scenario input is invalid",
		}
	}

	topologyResolved := scenario.Topology != nil &&
		validateSimulatorScenarioTopology(*scenario.Topology) == nil
	input := simulatorRuntimePlanInput{
		OrganizationID: organizationID, Organization: organization,
		Scenario: scenario, Manifest: manifest, TopologyResolved: topologyResolved,
		Policy:          simulatorRuntimePolicyEvidence{OrganizationID: organizationID},
		NorthstarAssets: simulatorRuntimeNorthstarAssets(organization, simulatorOrganizationsDirectory()),
	}
	scopeDigest, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusInternalServerError, message: "scenario authorization scope could not be digested",
		}
	}
	input.AuthorizationScopeDigest = scopeDigest
	plan := buildSimulatorRuntimePlan(input)
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		addSimulatorRuntimePlanBlocker(&plan, "scenario validation failed: "+err.Error())
	}
	return plan, nil
}

func simulatorRuntimeNorthstarAssets(organization simulatorOrganization, organizationsDirectory string) simulatorRuntimeNorthstarAssetEvidence {
	if organization.ID != "northstar" {
		return simulatorRuntimeNorthstarAssetEvidence{}
	}

	missing := make([]string, 0)
	directoryRoot := filepath.Join(organizationsDirectory, organization.ID)
	for _, filename := range organization.Directory.LDIFFiles {
		if _, err := readOrganizationDefinition(directoryRoot, filename); err != nil {
			missing = append(missing, "directory/"+filename)
		}
	}

	assetRoots := []string{organizationsDirectory, filepath.Dir(organizationsDirectory)}
	for _, asset := range []struct {
		name string
		path string
	}{
		{name: "policy-config", path: organization.PolicyConfig},
		{name: "policy-catalog", path: organization.PolicyCatalog},
	} {
		found := false
		for _, root := range assetRoots {
			if _, err := readOrganizationDefinition(root, asset.path); err == nil {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, asset.name)
		}
	}
	return simulatorRuntimeNorthstarAssetEvidence{
		Resolved: len(missing) == 0,
		Missing:  missing,
	}
}

func readExplicitSimulatorScenario(scenarioID string) (simulatorScenario, error) {
	var scenario simulatorScenario
	if !validScenarioID(scenarioID) {
		return scenario, &simulatorScenarioPlanBuildFailure{
			status: http.StatusBadRequest, message: "valid scenario_id is required",
		}
	}
	filename := filepath.Join(simulatorScenarioDirectory(), scenarioID+".json")
	info, err := os.Stat(filename)
	if errors.Is(err, os.ErrNotExist) {
		return scenario, &simulatorScenarioPlanBuildFailure{
			status: http.StatusNotFound, message: "scenario was not found",
		}
	}
	if err != nil || info.Size() > maxScenarioFileSize {
		return scenario, &simulatorScenarioPlanBuildFailure{
			status: http.StatusServiceUnavailable, message: "scenario file is unavailable",
		}
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		return scenario, &simulatorScenarioPlanBuildFailure{
			status: http.StatusServiceUnavailable, message: "scenario file is unavailable",
		}
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scenario); err != nil {
		return scenario, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return scenario, errors.New("scenario file contains trailing data")
	}
	if scenario.ID != scenarioID {
		return scenario, &simulatorScenarioPlanBuildFailure{
			status: http.StatusUnprocessableEntity, message: "scenario identity does not match its filename",
		}
	}
	return scenario, nil
}

func addSimulatorRuntimePlanBlocker(plan *simulatorRuntimePlan, message string) {
	for _, existing := range plan.Blockers {
		if existing == message {
			return
		}
	}
	plan.Blockers = append(plan.Blockers, message)
	sort.Strings(plan.Blockers)
	plan.Status = simulatorRuntimePlanBlocked
}
