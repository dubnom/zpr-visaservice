//go:build !production

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	evaluationruntime "neboagency.com/zpr-dashborad/internal/simulator/evaluation"
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
	Status                   simulatorRuntimePlanStatus         `json:"status"`
	OrganizationID           string                             `json:"organization_id"`
	ScenarioID               string                             `json:"scenario_id"`
	ManifestName             string                             `json:"manifest_name"`
	MachineIDs               []string                           `json:"machine_ids"`
	Topology                 *simulatorScenarioTopology         `json:"topology,omitempty"`
	RetainedTopologyBinding  *simulatorRetainedTopologyBinding  `json:"retained_topology_binding,omitempty"`
	Workloads                []simulatorRuntimePlanWorkload     `json:"workloads"`
	MachineRuntimeIdentities []simulatorRuntimeIdentity         `json:"machine_runtime_identities,omitempty"`
	MachineRuntimeTrust      []simulatorRuntimeTrustRequirement `json:"machine_runtime_trust,omitempty"`
	BudgetInputs             []simulatorRuntimeBudgetInput      `json:"budget_inputs"`
	BudgetStatus             simulatorRuntimeBudgetStatus       `json:"budget_status"`
	TopologyStatus           string                             `json:"topology_status"`
	PolicyStatus             string                             `json:"policy_status"`
	NorthstarAssetsStatus    string                             `json:"northstar_assets_status"`
	PackageContractStatus    string                             `json:"package_contract_status"`
	Blockers                 []string                           `json:"blockers"`
	PlanDigest               string                             `json:"plan_digest"`
	AuthorizationScopeDigest string                             `json:"authorization_scope_digest"`
}

type simulatorScenarioBudgetEvidenceUnavailableResponse struct {
	BudgetApprovalAvailable  bool     `json:"budget_approval_available,omitempty"`
	OrganizationID           string   `json:"organization_id"`
	ScenarioID               string   `json:"scenario_id"`
	AuthorizationScopeDigest string   `json:"authorization_scope_digest"`
	Blockers                 []string `json:"blockers"`
}

type simulatorScenarioPlanApprovalResponse struct {
	Approved                 bool     `json:"approved"`
	ApprovalID               string   `json:"approval_id"`
	ApprovalReference        string   `json:"approval_reference"`
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
	Principal  simulatorScenarioPrincipal
}

type simulatorScenarioPlanApprovalStore struct {
	mu             sync.Mutex
	byID           map[string]simulatorScenarioPlanApproval
	pendingByScope map[string]string
	// journal, when set, durably records issued and consumed approvals so
	// the same principal can resume an unconsumed approval after a restart
	// through the full approval recheck. A nil journal keeps the store
	// memory-only.
	journal simulatorScenarioPermitJournal
}

var simulatorScenarioPlanApprovals = newDurableSimulatorScenarioPlanApprovalStore()

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

func newDurableSimulatorScenarioPlanApprovalStore() *simulatorScenarioPlanApprovalStore {
	store := newSimulatorScenarioPlanApprovalStore()
	store.journal = simulatorScenarioPermitFileJournal{}
	return store
}

func validateSimulatorScenarioPlanIDs(w http.ResponseWriter, organizationID, scenarioID string) bool {
	if !validSimulatorRunNamespaceID(organizationID) || !validSimulatorRunNamespaceID(scenarioID) {
		http.Error(w, fmt.Sprintf("organization_id and scenario_id must be valid lowercase IDs of at most %d characters",
			maxSimulatorRunNamespaceIDLength), http.StatusBadRequest)
		return false
	}
	return true
}

func (h *simulatorScenarioPlanHandler) Preflight(w http.ResponseWriter, r *http.Request) {
	var request simulatorScenarioPlanRequest
	if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
		return
	}
	if !validateSimulatorScenarioPlanIDs(w, request.OrganizationID, request.ScenarioID) {
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
		http.Error(w, "current approved authorization evidence is unavailable or invalid", http.StatusServiceUnavailable)
		return
	}
	const blocker = "current approved authorization evidence is unavailable or invalid"
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
		BudgetApprovalAvailable: simulatorScenarioBudgetContractSupported(plan.PackageContractStatus) &&
			len(withoutSimulatorRuntimeBudgetEvidenceBlockers(plan.Blockers)) == 0,
		OrganizationID:           plan.OrganizationID,
		ScenarioID:               plan.ScenarioID,
		AuthorizationScopeDigest: plan.AuthorizationScopeDigest,
		Blockers:                 blockers,
	}
	content, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "authorization evidence failure response could not be encoded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	if _, err := w.Write(append(content, '\n')); err != nil {
		log.Printf("write Simulator authorization evidence failure response: %v", err)
	}
}

func (h *simulatorScenarioPlanHandler) Approve(w http.ResponseWriter, r *http.Request) {
	var request simulatorScenarioPlanApprovalRequest
	if !decodeSimulatorScenarioPlanRequest(w, r, &request) {
		return
	}
	if !validateSimulatorScenarioPlanIDs(w, request.OrganizationID, request.ScenarioID) {
		return
	}
	if request.PlanDigest == "" {
		http.Error(w, "plan_digest is required", http.StatusBadRequest)
		return
	}
	if !request.Approved {
		http.Error(w, "approved must be true to approve a scenario plan", http.StatusBadRequest)
		return
	}
	if !h.requestOrganizationMatches(w, r, request.OrganizationID) {
		return
	}
	principal, err := simulatorScenarioPrincipalFromContext(r.Context())
	if err != nil {
		writeWorkspaceError(w, http.StatusForbidden, simulatorScenarioOperatorDeniedMessage)
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
		http.Error(w, "current approved authorization evidence is unavailable or invalid", http.StatusServiceUnavailable)
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
		http.Error(w, "blocked scenario plan or unapproved authorization cannot be approved", http.StatusConflict)
		return
	}
	if h.approvals == nil {
		http.Error(w, "scenario plan approval store is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := revalidateSimulatorScenarioPrincipal(r.Context(), principal); err != nil {
		writeWorkspaceError(w, http.StatusForbidden, simulatorScenarioOperatorDeniedMessage)
		return
	}
	approvalID, err := h.approvals.issueOrResumeForPrincipal(plan, principal)
	if err != nil {
		if errors.Is(err, errSimulatorScenarioPlanApprovalPending) {
			writeWorkspaceError(w, http.StatusConflict, "An unconsumed approval for this plan belongs to a different operator or authorization mode.")
			return
		}
		log.Printf("Simulator scenario plan approval could not be issued or resumed: %v", err)
		http.Error(w, "scenario plan approval could not be issued", http.StatusInternalServerError)
		return
	}
	writeSimulatorJSON(w, simulatorScenarioPlanApprovalResponse{
		Approved: true, ApprovalID: approvalID,
		ApprovalReference: simulatorScenarioPlanApprovalReference(approvalID),
		OrganizationID:    plan.OrganizationID, ScenarioID: plan.ScenarioID,
		PlanDigest: digest, AuthorizationScopeDigest: plan.AuthorizationScopeDigest,
		MachineIDs: append([]string{}, plan.MachineIDs...),
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
	if plan == nil {
		return
	}
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), plan.OrganizationID)
	if err != nil {
		return
	}
	var resolver simulatorScenarioCurrentPolicyResolver
	if h != nil {
		resolver = h.resolveCurrentPolicy
	}
	if organization.PolicyBinding != nil {
		if resolver == nil {
			resolver = currentSimulatorPolicyEvidence
		}
		resolveSimulatorPolicyBinding(ctx, plan, organization, resolver)
		return
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
	if organization.PolicyBinding != nil {
		return verifySimulatorPolicyBinding(ctx, organization, currentSimulatorPolicyEvidence)
	}
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
		absent        = "approved authorization evidence is absent"
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
		Topology:                 plan.Topology,
		RetainedTopologyBinding:  plan.RetainedTopologyBinding,
		Workloads:                plan.Workloads,
		MachineRuntimeIdentities: plan.MachineRuntimeIdentities,
		MachineRuntimeTrust:      plan.MachineRuntimeTrust,
		BudgetInputs:             plan.BudgetInputs,
		BudgetStatus:             plan.BudgetStatus, TopologyStatus: plan.TopologyStatus,
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
	if plan.RetainedTopologyBinding != nil {
		copy := *plan.RetainedTopologyBinding
		cloned.RetainedTopologyBinding = &copy
	}
	cloned.MachineRuntimeIdentities = slices.Clone(plan.MachineRuntimeIdentities)
	cloned.MachineRuntimeTrust = slices.Clone(plan.MachineRuntimeTrust)
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

func simulatorScenarioPlanApprovalReference(approvalID string) string {
	digest := sha256.Sum256([]byte("simulator-scenario-approval-reference-v1\x00" + approvalID))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (store *simulatorScenarioPlanApprovalStore) issueForPrincipal(
	plan simulatorRuntimePlan, principal simulatorScenarioPrincipal,
) (string, error) {
	return store.issueOrResume(plan, principal, false)
}

func (store *simulatorScenarioPlanApprovalStore) issueOrResumeForPrincipal(
	plan simulatorRuntimePlan, principal simulatorScenarioPrincipal,
) (string, error) {
	return store.issueOrResume(plan, principal, true)
}

func (store *simulatorScenarioPlanApprovalStore) issueOrResume(
	plan simulatorRuntimePlan, principal simulatorScenarioPrincipal, resume bool,
) (string, error) {
	if !principal.valid() {
		return "", errSimulatorScenarioOperatorDenied
	}
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
	if approvalID, exists := store.pendingByScope[scope]; exists {
		if !resume {
			return "", errSimulatorScenarioPlanApprovalPending
		}
		approval, exists := store.byID[approvalID]
		if !exists {
			return "", errors.New("pending scenario approval index is inconsistent")
		}
		if _, err := store.matchingApprovalLocked(approvalID, plan.OrganizationID, plan.ScenarioID, digest, approval.Principal); err != nil {
			return "", fmt.Errorf("pending scenario approval is inconsistent: %w", err)
		}
		if approval.Principal != principal {
			return "", errSimulatorScenarioPlanApprovalPending
		}
		return approvalID, nil
	}
	// A durable unconsumed approval for this exact scope survives a restart:
	// the same principal resumes it (the caller has already re-passed the full
	// plan, budget and session recheck); any other principal stays blocked. An
	// unavailable or corrupt journal fails closed.
	if store.journal != nil {
		record, err := store.journal.pendingForScope(plan.OrganizationID, plan.ScenarioID, digest)
		if err != nil {
			return "", fmt.Errorf("scenario permit journal is unavailable: %w", err)
		}
		if record != nil {
			recorded, err := record.principal()
			if err != nil {
				return "", fmt.Errorf("scenario permit journal is unavailable: %w", err)
			}
			if !resume || recorded != principal {
				return "", errSimulatorScenarioPlanApprovalPending
			}
			if _, exists := store.byID[record.ApprovalID]; exists {
				return "", errors.New("durable scenario approval conflicts with the in-memory store; inspect the journal")
			}
			store.byID[record.ApprovalID] = simulatorScenarioPlanApproval{
				ID: record.ApprovalID, Plan: plan, PlanDigest: digest, Principal: principal,
			}
			store.pendingByScope[scope] = record.ApprovalID
			return record.ApprovalID, nil
		}
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}
	approvalID := base64.RawURLEncoding.EncodeToString(token)
	if _, exists := store.byID[approvalID]; exists {
		return "", errors.New("scenario plan approval identifier collision")
	}
	if store.journal != nil {
		mode, issuer, subject, err := simulatorScenarioPermitPrincipalFields(principal)
		if err != nil {
			return "", err
		}
		if err := store.journal.appendRecord(simulatorScenarioPermitRecord{
			Event: simulatorScenarioPermitIssued, ApprovalID: approvalID,
			OrganizationID: plan.OrganizationID, ScenarioID: plan.ScenarioID, PlanDigest: digest,
			PrincipalMode: mode, ActorIssuer: issuer, ActorSubject: subject,
		}); err != nil {
			return "", fmt.Errorf("record durable scenario approval: %w", err)
		}
	}
	approval := simulatorScenarioPlanApproval{
		ID: approvalID, Plan: plan, PlanDigest: digest, Principal: principal,
	}
	store.byID[approvalID] = approval
	store.pendingByScope[scope] = approvalID
	return approvalID, nil
}

var (
	errSimulatorScenarioApprovalUnknown       = errors.New("scenario plan approval is unknown or already consumed")
	errSimulatorScenarioApprovalScopeMismatch = errors.New("scenario plan approval scope does not match")
)

func (store *simulatorScenarioPlanApprovalStore) consumeForPrincipal(
	approvalID, organizationID, scenarioID, planDigest string, principal simulatorScenarioPrincipal,
) (simulatorRuntimePlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	approval, err := store.matchingApprovalLocked(approvalID, organizationID, scenarioID, planDigest, principal)
	if err != nil {
		return simulatorRuntimePlan{}, err
	}
	// Durable consumption precedes in-memory consumption so a restart can
	// never resume an approval whose run was already admitted; a journal
	// failure keeps the one-use approval intact and blocks the run.
	if store.journal != nil {
		mode, issuer, subject, err := simulatorScenarioPermitPrincipalFields(principal)
		if err != nil {
			return simulatorRuntimePlan{}, err
		}
		if err := store.journal.appendRecord(simulatorScenarioPermitRecord{
			Event: simulatorScenarioPermitConsumed, ApprovalID: approvalID,
			OrganizationID: approval.Plan.OrganizationID, ScenarioID: approval.Plan.ScenarioID,
			PlanDigest:    approval.PlanDigest,
			PrincipalMode: mode, ActorIssuer: issuer, ActorSubject: subject,
		}); err != nil {
			return simulatorRuntimePlan{}, fmt.Errorf("record durable scenario approval consumption: %w", err)
		}
	}
	delete(store.byID, approvalID)
	scope := simulatorScenarioPlanApprovalScope(approval.Plan.OrganizationID, approval.Plan.ScenarioID, approval.PlanDigest)
	if store.pendingByScope[scope] == approvalID {
		delete(store.pendingByScope, scope)
	}
	return cloneSimulatorRuntimePlan(approval.Plan), nil
}

func (store *simulatorScenarioPlanApprovalStore) lookupForPrincipal(
	approvalID, organizationID, scenarioID, planDigest string, principal simulatorScenarioPrincipal,
) (simulatorRuntimePlan, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	approval, err := store.matchingApprovalLocked(approvalID, organizationID, scenarioID, planDigest, principal)
	if err != nil {
		return simulatorRuntimePlan{}, err
	}
	return cloneSimulatorRuntimePlan(approval.Plan), nil
}

func (store *simulatorScenarioPlanApprovalStore) matchingApprovalLocked(
	approvalID, organizationID, scenarioID, planDigest string, principal simulatorScenarioPrincipal,
) (simulatorScenarioPlanApproval, error) {
	if !principal.valid() {
		return simulatorScenarioPlanApproval{}, errSimulatorScenarioOperatorDenied
	}
	approval, exists := store.byID[approvalID]
	if !exists {
		return simulatorScenarioPlanApproval{}, errSimulatorScenarioApprovalUnknown
	}
	if approval.ID != approvalID ||
		approval.Plan.OrganizationID != organizationID ||
		approval.Plan.ScenarioID != scenarioID ||
		approval.PlanDigest != planDigest ||
		approval.Principal != principal {
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
		writeWorkspaceError(w, failure.status, failure.message)
		return
	}
	writeWorkspaceError(w, http.StatusServiceUnavailable, "scenario plan input is unavailable")
}

// readRetainedSimulatorPolicyDigest reads the authoritative live policy
// digest for a retained paired install through its guarded Visa Service
// admin readback registered in the evaluation install registry.
func readRetainedSimulatorPolicyDigest(organizationID string) (string, error) {
	registry := simulatorEvaluationInstallRegistry
	if registry == nil {
		return "", errors.New("paired operator service is unavailable")
	}
	install, ok := registry.installs[organizationID]
	if !ok || install.VisaAdminURL == "" || install.VisaAdminKeyFile == "" {
		return "", errors.New("guarded policy readback is not configured for this installation")
	}
	transport, err := servicehttp.MutualTLSClientTransport(install.ClientCertFile, install.ClientKeyFile, install.ServiceCAFile, "")
	if err != nil {
		return "", err
	}
	defer transport.CloseIdleConnections()
	key, err := readOperatorFile(install.VisaAdminKeyFile, 4096, true)
	if err != nil {
		return "", errors.New("policy readback credential unavailable")
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return evaluationruntime.ReadPolicyDigest(ctx, client, install.VisaAdminURL, strings.TrimSpace(string(key)))
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
	path, err := workspaceDatabasePath(organizationID)
	if err != nil {
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusBadRequest, message: "valid organization_id is required",
		}
	}
	store, err := openExistingWorkspaceRepository(path)
	if err != nil {
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusServiceUnavailable, message: "published scenario workspace is unavailable; load the scenario catalog first",
		}
	}
	defer store.Close()
	scenario, _, err := readPublishedSimulatorScenario(context.Background(), store, organizationID, scenarioID)
	if err != nil {
		var failure *simulatorPublishedScenarioFailure
		if errors.As(err, &failure) {
			return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{status: failure.status, message: failure.message}
		}
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusServiceUnavailable, message: "published scenario is unavailable",
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
	var evaluationRuntimeErr error
	if organization.EvaluationRuntime != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, evaluationRuntimeErr = evaluationScenarioPlane(ctx, organization)
		cancel()
		if evaluationRuntimeErr == nil {
			input.Policy = simulatorRuntimePolicyEvidence{Resolved: true, OrganizationID: organizationID,
				Digest: organization.EvaluationRuntime.PolicyDigest}
		}
	} else if organization.RetainedTopologyBinding != nil {
		// The retained install resolves policy through the paired install's
		// guarded live readback; an unavailable readback keeps the explicit
		// "policy resolution is required" blocker rather than guessing.
		if digest, err := readRetainedSimulatorPolicyDigest(organizationID); err == nil {
			input.Policy = simulatorRuntimePolicyEvidence{Resolved: true, OrganizationID: organizationID, Digest: digest}
		}
	}
	scopeDigest, err := simulatorScenarioAuthorizationScopeDigest(input)
	if err != nil {
		return simulatorRuntimePlan{}, &simulatorScenarioPlanBuildFailure{
			status: http.StatusInternalServerError, message: "scenario authorization scope could not be digested",
		}
	}
	input.AuthorizationScopeDigest = scopeDigest
	plan := buildSimulatorRuntimePlan(input)
	if err := validateSimulatorScenarioMachineRuntime(organization, manifest, plan.MachineIDs); err != nil {
		addSimulatorRuntimePlanBlocker(&plan, "scenario runtime is unavailable: "+err.Error())
	}
	if organization.RetainedTopologyBinding != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		resolveSimulatorRetainedTopology(ctx, &plan, organization)
		cancel()
		if organization.RetainedMachineRuntime != nil {
			if err := verifySimulatorRetainedMachineFiles(organization); err != nil {
				addSimulatorRuntimePlanBlocker(&plan, "retained machine runtime verification failed: "+err.Error())
			}
		}
	}
	if evaluationRuntimeErr != nil {
		addSimulatorRuntimePlanBlocker(&plan, "evaluation runtime verification failed: "+evaluationRuntimeErr.Error())
	}
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
