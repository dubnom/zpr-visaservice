//go:build !production

package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"neboagency.com/zpr-dashborad/internal/gateway"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	evaluationruntime "neboagency.com/zpr-dashborad/internal/simulator/evaluation"
	"neboagency.com/zpr-dashborad/internal/simulator/setupmode"
)

const defaultSimulatorOrganizationID = "northstar"

type simulatorOrganization struct {
	ID                      string                            `json:"id"`
	Name                    string                            `json:"name"`
	Description             string                            `json:"description"`
	Runtime                 simulatorOrganizationRuntime      `json:"runtime"`
	Directory               simulatorOrganizationDirectory    `json:"directory"`
	MachineOwners           map[string][]string               `json:"machine_owners,omitempty"`
	PolicyConfig            string                            `json:"policy_config"`
	PolicyCatalog           string                            `json:"policy_catalog"`
	RuntimePolicy           string                            `json:"runtime_policy,omitempty"`
	AssertionsSource        string                            `json:"assertions_source,omitempty"`
	Policies                []simulatorOrganizationPolicy     `json:"policies"`
	Services                []simulatorOrganizationService    `json:"services"`
	PolicyTestServices      []simulatorPolicyTestService      `json:"policy_test_services,omitempty"`
	WorkloadServices        map[string][]string               `json:"workload_services,omitempty"`
	WebGateway              *simulatorWebGatewayProfile       `json:"web_gateway,omitempty"`
	LoadTest                *simulatorLoadTestProfile         `json:"load_test,omitempty"`
	BusinessSimulation      bool                              `json:"business_simulation,omitempty"`
	SyntheticLabOnly        bool                              `json:"synthetic_lab_only,omitempty"`
	SyntheticWorkload       *simulatorSyntheticWorkload       `json:"synthetic_workload,omitempty"`
	ActivePolicyTargetID    string                            `json:"active_policy_target_id,omitempty"`
	ActivePolicyDigest      string                            `json:"active_policy_digest,omitempty"`
	PolicyBinding           *simulatorPolicyBinding           `json:"policy_binding,omitempty"`
	PackagedProvisioning    *simulatorPackagedProvisioning    `json:"packaged_provisioning,omitempty"`
	EvaluationRuntime       *evaluationruntime.Contract       `json:"evaluation_runtime,omitempty"`
	RetainedTopologyBinding *simulatorRetainedTopologyBinding `json:"retained_topology_binding,omitempty"`
	RetainedMachineRuntime  *simulatorRetainedMachineRuntime  `json:"retained_machine_runtime,omitempty"`
	DeclaredEndpoints       []simulatorDeclaredEndpoint       `json:"declared_endpoints,omitempty"`
}

// simulatorDeclaredEndpoint is one profile-declared required endpoint. Every
// field feeds the Enrollment Service invitation contract verbatim, so each is
// required and bounded exactly like the service's own asset validation. The
// declaration states a requirement only: enrollment, approval and issuance
// stay with the Enrollment Service through the explicit onboarding chain.
type simulatorDeclaredEndpoint struct {
	AssetID   string `json:"asset_id"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Type      string `json:"type"`
	Profile   string `json:"profile"`
	Recipient string `json:"recipient"`
}

const maxSimulatorDeclaredEndpoints = 32

func validSimulatorDeclaredEndpointText(value string) bool {
	return strings.TrimSpace(value) == value && value != "" && len(value) <= 256 &&
		!strings.ContainsAny(value, "\x00\r\n")
}

// validateSimulatorDeclaredEndpoints enforces the profile contract for
// declared endpoints: complete bounded fields, unique asset identifiers, and
// no overlap with the reviewed retained inventory (retained assets are a
// separate pinned contract and are never onboarded from a declaration).
func validateSimulatorDeclaredEndpoints(endpoints []simulatorDeclaredEndpoint) error {
	if len(endpoints) > maxSimulatorDeclaredEndpoints {
		return fmt.Errorf("at most %d declared endpoints are supported", maxSimulatorDeclaredEndpoints)
	}
	seen := make(map[string]struct{}, len(endpoints))
	for _, endpoint := range endpoints {
		for _, value := range []string{endpoint.AssetID, endpoint.Name, endpoint.Owner, endpoint.Type, endpoint.Profile, endpoint.Recipient} {
			if !validSimulatorDeclaredEndpointText(value) {
				return fmt.Errorf("declared endpoint %q requires complete bounded asset fields", endpoint.AssetID)
			}
		}
		if _, exists := seen[endpoint.AssetID]; exists {
			return fmt.Errorf("declared endpoint %q is listed more than once", endpoint.AssetID)
		}
		if slices.Contains(simulatorRetainedRequiredAssets, endpoint.AssetID) {
			return fmt.Errorf("declared endpoint %q collides with the reviewed retained inventory", endpoint.AssetID)
		}
		seen[endpoint.AssetID] = struct{}{}
	}
	return nil
}

type simulatorSyntheticWorkload struct {
	Version   int    `json:"version"`
	MachineID string `json:"machine_id"`
	Name      string `json:"name"`
	Interface string `json:"interface"`
}

type simulatorWebGatewayProfile struct {
	AllowedHosts []string `json:"allowed_hosts"`
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
	ID               string   `json:"id"`
	Location         string   `json:"location"`
	SubstrateAddress string   `json:"substrate_address,omitempty"`
	ZPRAddress       string   `json:"zpr_address,omitempty"`
	Latitude         *float64 `json:"latitude,omitempty"`
	Longitude        *float64 `json:"longitude,omitempty"`
	CoordinateNote   string   `json:"coordinate_note,omitempty"`
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

func handleSimulatorOrganizationActivate(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleSimulatorOrganizationOperation(w, r, false, auth, signer)
	}
}

func handleSimulatorOrganizationRestoreBase(w http.ResponseWriter, r *http.Request) {
	handleSimulatorOrganizationOperation(w, r, true, nil, nil)
}

// handleSimulatorOrganizationDeactivate is the approved deactivation-time
// offboarding contract (teardown means explicit organization deactivation,
// not per-run teardown): every profile-declared endpoint with an approved
// enrollment record is offboarded through the maintained single-call
// offboarding workflow — registry revocation first, Simulator-owned workspace
// bundle removal only after it succeeds — and the active-organization
// selection is cleared when it names this organization. Reviewed retained
// inventory is never offboarded here: grandfathered retained identities keep
// their separate contracts, and legacy-launcher mode is refused entirely.
// A partial failure keeps the per-asset operation records and stops without
// clearing the selection.
func handleSimulatorOrganizationDeactivate(auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode, err := setupmode.Parse(os.Getenv(setupmode.Environment))
		if err != nil || mode != setupmode.OrdinaryClient {
			writeWorkspaceError(w, http.StatusServiceUnavailable,
				"Explicit deactivation offboarding is an ordinary-client on-the-fly contract; legacy launcher organizations keep their reviewed retained identities. No offboarding was started.")
			return
		}
		var input struct {
			Reason string `json:"reason"`
		}
		organizationID, principal, ok := simulatorEnrollmentMutationContext(w, r, auth, signer, &input)
		if !ok {
			return
		}
		if strings.TrimSpace(input.Reason) == "" {
			http.Error(w, "deactivation requires an explicit reason", http.StatusBadRequest)
			return
		}
		profile, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID)
		if err != nil {
			writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
			return
		}
		activeSimulatorScenario.mu.Lock()
		defer activeSimulatorScenario.mu.Unlock()
		if activeSimulatorScenario.run.State == "running" || activeSimulatorScenario.run.State == "cleaning" {
			writeWorkspaceError(w, http.StatusConflict, "Finish or cancel the running scenario before deactivating the organization.")
			return
		}
		simulatorSessions.RLock()
		defer simulatorSessions.RUnlock()
		for _, session := range simulatorSessions.byMachine {
			if session.Authenticated {
				writeWorkspaceError(w, http.StatusConflict, "Log out all machine users before deactivating the organization.")
				return
			}
		}
		// Resolve the current enrollment state once through the authorized
		// relay; only profile-declared endpoints with an approved record are
		// offboarded. Reviewed retained inventory is never in this set.
		approvedAssets := map[string]bool{}
		if len(profile.DeclaredEndpoints) > 0 {
			recorder := newSimulatorEnrollmentRecorder()
			relaySimulatorEnrollmentInvitations(recorder, r, auth, signer, principal, organizationID)
			if recorder.code != http.StatusOK {
				writeWorkspaceError(w, http.StatusBadGateway, fmt.Sprintf(
					"deactivation stopped at the enrollment inventory read (HTTP %d from the paired operator service); no offboarding was started", recorder.code))
				return
			}
			var inventory struct {
				Invitations []struct {
					Asset struct {
						AssetID string `json:"asset_id"`
					} `json:"asset"`
					State string `json:"state"`
				} `json:"invitations"`
			}
			if err := json.Unmarshal(recorder.body.Bytes(), &inventory); err != nil {
				writeWorkspaceError(w, http.StatusBadGateway,
					"enrollment inventory response is invalid; no offboarding was started")
				return
			}
			for _, invitation := range inventory.Invitations {
				if invitation.State == "approved" {
					approvedAssets[invitation.Asset.AssetID] = true
				}
			}
		}
		offboard := newSimulatorEnrollmentOffboardingHandler().serve(auth, signer)
		offboarded := make([]json.RawMessage, 0, len(profile.DeclaredEndpoints))
		alreadyClean := []string{}
		for _, endpoint := range profile.DeclaredEndpoints {
			if !approvedAssets[endpoint.AssetID] {
				alreadyClean = append(alreadyClean, endpoint.AssetID)
				continue
			}
			body, err := json.Marshal(struct {
				AssetID string `json:"asset_id"`
				Reason  string `json:"reason"`
			}{endpoint.AssetID, input.Reason})
			if err != nil {
				writeWorkspaceError(w, http.StatusInternalServerError,
					"cannot encode the declared endpoint offboarding request; deactivation stopped")
				return
			}
			request := r.Clone(r.Context())
			request.Method = http.MethodPost
			request.URL.RawQuery = ""
			request.Header = r.Header.Clone()
			request.Header.Set("Content-Type", "application/json")
			request.Body = io.NopCloser(bytes.NewReader(body))
			request.ContentLength = int64(len(body))
			request.SetPathValue("organization", organizationID)
			recorder := newSimulatorEnrollmentRecorder()
			offboard(recorder, request)
			if recorder.code != http.StatusOK {
				writeWorkspaceError(w, http.StatusBadGateway, fmt.Sprintf(
					"deactivation offboarding for declared endpoint %s stopped (HTTP %d): %s. %d endpoint(s) were already offboarded with persisted operation records; the organization selection is unchanged. Reconcile through the explicit enrollment review path.",
					endpoint.AssetID, recorder.code, strings.TrimSpace(recorder.body.String()), len(offboarded)))
				return
			}
			offboarded = append(offboarded, json.RawMessage(append([]byte(nil), recorder.body.Bytes()...)))
		}
		selection := simulatorActiveOrganizationPath()
		selectionCleared := false
		if selected, err := os.ReadFile(selection); err == nil && strings.TrimSpace(string(selected)) == organizationID {
			if err := os.Remove(selection); err != nil {
				writeWorkspaceError(w, http.StatusInternalServerError, fmt.Sprintf(
					"every declared endpoint is offboarded with persisted operation records, but the organization selection could not be cleared: %v", err))
				return
			}
			selectionCleared = true
		}
		writeSimulatorJSON(w, map[string]any{
			"deactivation": map[string]any{
				"organization_id": organizationID, "setup_mode": "ordinary-client",
				"selection_cleared": selectionCleared, "reason": input.Reason,
			},
			"offboarded":    offboarded,
			"already_clean": alreadyClean,
		})
	}
}

func handleSimulatorOrganizationOperation(w http.ResponseWriter, r *http.Request, restoreBase bool, auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer) {
	mode, err := setupmode.Parse(os.Getenv(setupmode.Environment))
	if err != nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable,
			"Invalid SIMULATION_SETUP_MODE; expected legacy-launcher or ordinary-client. No setup was started.")
		return
	}
	ordinary := false
	switch mode {
	case setupmode.LegacyLauncher:
	case setupmode.OrdinaryClient:
		if restoreBase {
			writeWorkspaceError(w, http.StatusServiceUnavailable,
				"Restore is blocked in ordinary-client mode until an independent restore contract is available (U01-ACTIVATION). No setup was started.")
			return
		}
		ordinary = true
	default:
		writeWorkspaceError(w, http.StatusServiceUnavailable,
			"Simulator setup mode is unsupported. No setup was started.")
		return
	}
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
	if ordinary {
		handleOrdinaryClientActivation(w, r, auth, signer, organizationID)
		return
	}
	var started bool
	if restoreBase {
		started = activeOrganizationActivation.restoreBase(organizationID, simulatorActiveOrganizationPath())
	} else {
		started = activeOrganizationActivation.start(organizationID, simulatorActiveOrganizationPath())
	}
	if !started {
		writeWorkspaceError(w, http.StatusConflict, "An organization activation is already in progress.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeSimulatorJSON(w, map[string]any{"activation": activeOrganizationActivation.snapshot()})
}

// handleOrdinaryClientActivation performs the ordinary-client authorized
// setup for explicit organization activation: the exact required endpoint
// inventory is compared with retained enrollment state through the ordinary
// authorized contracts, activation proceeds only when every requirement is
// retained and approved, and every missing prerequisite is reported
// explicitly without starting any setup. The legacy launcher-owned
// activation, resets and restores are never invoked in this mode, and
// automatic onboarding of genuinely new profile-declared endpoints remains a
// separate explicit operation.
func handleOrdinaryClientActivation(w http.ResponseWriter, r *http.Request, auth operatorEnrollmentAuthorizer, signer *operatordelegation.Signer, organizationID string) {
	signer = simulatorOperatorSignerFor(organizationID, signer)
	if operatorAuthorizationDisabled(auth) || signer == nil {
		writeWorkspaceError(w, http.StatusServiceUnavailable,
			"Named enrollment delegation is not configured; explicit activation starts no setup (U01-ACTIVATION).")
		return
	}
	principal, err := simulatorScenarioPrincipalFromContext(r.Context())
	if err != nil || principal.mode != simulatorAuthorizationNamed {
		writeWorkspaceError(w, http.StatusForbidden,
			"Explicit activation requires the current named operator session. No setup was started.")
		return
	}
	readiness := r.Clone(r.Context())
	readiness.Method = http.MethodGet
	readiness.URL.RawQuery = ""
	readiness.Body, readiness.ContentLength = http.NoBody, 0
	readReadiness := func() (simulatorEnrollmentReadiness, bool) {
		recorder := newSimulatorEnrollmentRecorder()
		newSimulatorEnrollmentReadinessHandler().serve(auth, signer)(recorder, readiness)
		if recorder.code != http.StatusOK {
			writeWorkspaceError(w, http.StatusBadGateway, fmt.Sprintf(
				"activation readiness is unavailable (HTTP %d); no setup was started", recorder.code))
			return simulatorEnrollmentReadiness{}, false
		}
		var report simulatorEnrollmentReadiness
		if err := json.Unmarshal(recorder.body.Bytes(), &report); err != nil {
			writeWorkspaceError(w, http.StatusBadGateway, "activation readiness response is invalid; no setup was started")
			return simulatorEnrollmentReadiness{}, false
		}
		return report, true
	}
	report, ok := readReadiness()
	if !ok {
		return
	}
	var onboarded []json.RawMessage
	if !report.Ready {
		operations, performed, ok := onboardDeclaredEndpointsForActivation(w, r, auth, signer, organizationID, report)
		if !ok {
			return
		}
		if performed {
			// Re-verify through the same authorized comparison: onboarding
			// evidence alone never substitutes for the readiness readback.
			onboarded = operations
			if report, ok = readReadiness(); !ok {
				return
			}
		}
	}
	if !report.Ready {
		w.WriteHeader(http.StatusConflict)
		writeSimulatorJSON(w, map[string]any{
			"error":     "the organization's required endpoints are not all retained and approved; resolve the exact missing prerequisites first. No setup was started.",
			"readiness": report,
		})
		return
	}
	selection := simulatorActiveOrganizationPath()
	temporary := selection + ".tmp"
	if err := os.WriteFile(temporary, []byte(organizationID+"\n"), 0o600); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "cannot record the activated organization; no setup was started")
		return
	}
	if err := os.Rename(temporary, selection); err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "cannot record the activated organization; no setup was started")
		return
	}
	response := map[string]any{
		"activation": map[string]any{
			"organization_id": organizationID, "state": "ready-verified", "setup_mode": "ordinary-client",
		},
		"readiness": report,
	}
	if len(onboarded) > 0 {
		response["onboarded"] = onboarded
	}
	writeSimulatorJSON(w, response)
}

// ordinaryActivationOnboardingHandler constructs the explicit onboarding
// workflow used for genuinely new profile-declared endpoints at activation;
// tests substitute a synthetic claim.
var ordinaryActivationOnboardingHandler = newSimulatorEnrollmentOnboardingHandler

// onboardDeclaredEndpointsForActivation onboards every genuinely new
// profile-declared endpoint — exactly those the authorized readiness
// comparison reports as missing with no invitation at all — through the
// maintained explicit onboarding chain. Any required endpoint in any other
// unready state (not approved, expired, or missing without a declaration)
// fails closed to the ordinary prerequisite report: unapproved or revoked
// endpoints never join, and interrupted issuance is reconciled through the
// explicit review path, never retried here. It returns the recorded
// operations, whether any onboarding was performed, and false when a
// response was already written.
func onboardDeclaredEndpointsForActivation(
	w http.ResponseWriter,
	r *http.Request,
	auth operatorEnrollmentAuthorizer,
	signer *operatordelegation.Signer,
	organizationID string,
	report simulatorEnrollmentReadiness,
) ([]json.RawMessage, bool, bool) {
	profile, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization")
		return nil, false, false
	}
	declared := make(map[string]simulatorDeclaredEndpoint, len(profile.DeclaredEndpoints))
	for _, endpoint := range profile.DeclaredEndpoints {
		declared[endpoint.AssetID] = endpoint
	}
	var missing []simulatorDeclaredEndpoint
	for _, required := range report.Required {
		switch required.Status {
		case "approved-retained":
		case "missing":
			endpoint, isDeclared := declared[required.AssetID]
			if !isDeclared {
				return nil, false, true
			}
			missing = append(missing, endpoint)
		default:
			return nil, false, true
		}
	}
	if len(missing) == 0 {
		return nil, false, true
	}
	onboard := ordinaryActivationOnboardingHandler().serve(auth, signer)
	operations := make([]json.RawMessage, 0, len(missing))
	for _, endpoint := range missing {
		body, err := json.Marshal(endpoint)
		if err != nil {
			writeWorkspaceError(w, http.StatusInternalServerError, "cannot encode the declared endpoint onboarding request; no setup was started")
			return nil, false, false
		}
		request := r.Clone(r.Context())
		request.Method = http.MethodPost
		request.URL.RawQuery = ""
		request.Header = r.Header.Clone()
		request.Header.Set("Content-Type", "application/json")
		request.Body = io.NopCloser(bytes.NewReader(body))
		request.ContentLength = int64(len(body))
		request.SetPathValue("organization", organizationID)
		recorder := newSimulatorEnrollmentRecorder()
		onboard(recorder, request)
		if recorder.code != http.StatusCreated {
			writeWorkspaceError(w, http.StatusBadGateway, fmt.Sprintf(
				"activation onboarding for declared endpoint %s stopped (HTTP %d): %s. No setup was started; reconcile through the explicit enrollment review path.",
				endpoint.AssetID, recorder.code, strings.TrimSpace(recorder.body.String())))
			return nil, false, false
		}
		operations = append(operations, json.RawMessage(append([]byte(nil), recorder.body.Bytes()...)))
	}
	return operations, true, true
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
		if err := validateSimulatorDeclaredEndpoints(organization.DeclaredEndpoints); err != nil {
			return nil, fmt.Errorf("organization %q declared endpoints: %w", entry.Name(), err)
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
	return validateOrganizationDefinition(organization, false)
}

func validateOrganizationDefinition(organization simulatorOrganization, incomplete bool) error {
	if err := validateSimulatorRetainedTopology(organization); err != nil {
		return err
	}
	if err := validateSimulatorRetainedMachineRuntime(organization); err != nil {
		return err
	}
	if binding := organization.PolicyBinding; binding != nil {
		if binding.Version != 1 || !validScenarioID(binding.TargetID) || !validOrganizationPolicyDigest(binding.Digest) ||
			organization.SyntheticLabOnly || organization.EvaluationRuntime != nil {
			return errors.New("policy binding requires version 1, an explicit target and SHA-256 digest, and no conflicting runtime policy contract")
		}
	}
	if organization.EvaluationRuntime != nil {
		contract := organization.EvaluationRuntime
		if contract.Version != 1 || organization.ID != "evaluation-"+contract.InstallID ||
			len(contract.InstallID) != 24 || !validSimulatorScenarioAuthorizationScopeDigest(contract.ImageID) ||
			!validSimulatorScenarioAuthorizationScopeDigest(contract.HandoffDigest) ||
			!validSimulatorScenarioAuthorizationScopeDigest(contract.PolicyDigest) ||
			contract.EndpointCount < 2 || contract.EndpointCount > 16 || contract.ListenerCount < 1 || contract.ListenerCount > 8 ||
			organization.PackagedProvisioning != nil || organization.SyntheticLabOnly {
			return errors.New("invalid or conflicting evaluation endpoint runtime contract")
		}
	}
	if err := validatePackagedProvisioning(organization.PackagedProvisioning); err != nil {
		return err
	}
	if organization.SyntheticLabOnly && organization.ID != "northstar" {
		return errors.New("synthetic lab-only runtime is restricted to the northstar organization")
	}
	if organization.SyntheticLabOnly && organization.ActivePolicyTargetID == "" {
		return errors.New("synthetic Northstar runtime requires an active policy target and digest")
	}
	if !organization.SyntheticLabOnly &&
		(organization.ActivePolicyTargetID != "" || organization.ActivePolicyDigest != "") {
		return errors.New("active policy identity fields require a synthetic lab-only profile")
	}
	if (organization.ActivePolicyTargetID == "") != (organization.ActivePolicyDigest == "") ||
		organization.ActivePolicyTargetID != "" && (!validScenarioID(organization.ActivePolicyTargetID) || !validOrganizationPolicyDigest(organization.ActivePolicyDigest)) {
		return errors.New("active policy readback requires a valid target ID and SHA-256 digest")
	}
	if organization.SyntheticLabOnly {
		provisioning := organization.PackagedProvisioning
		if provisioning == nil || provisioning.PrimaryMachineID != "machine-01" ||
			len(provisioning.Machines) != 1 || provisioning.Machines[0] != "machine-01" {
			return errors.New("synthetic Northstar runtime requires the single-machine machine-01 package contract")
		}
		workload := organization.SyntheticWorkload
		if workload == nil || workload.Version != 1 || workload.MachineID != "machine-01" ||
			workload.Name != "finance-client" || workload.Interface != "tun0" {
			return errors.New("synthetic Northstar runtime requires the version 1 finance-client tun0 workload")
		}
	} else if organization.SyntheticWorkload != nil {
		return errors.New("synthetic workload configuration requires a synthetic lab-only profile")
	}
	if err := validateBusinessSimulationOrganization(organization); err != nil {
		return err
	}
	if !validScenarioID(organization.ID) || strings.TrimSpace(organization.Name) == "" || strings.TrimSpace(organization.Description) == "" {
		return errors.New("id, name, and description are required")
	}
	if organization.Runtime.Driver != "linux-one-node" && organization.Runtime.Driver != "docker-multinode" && !(incomplete && organization.Runtime.Driver == "") {
		return errors.New("runtime driver must be linux-one-node or docker-multinode")
	}
	if organization.Runtime.Topology != "single-node" && organization.Runtime.Topology != "multi-node" && !(incomplete && organization.Runtime.Topology == "") {
		return errors.New("runtime topology must be single-node or multi-node")
	}
	if organization.Runtime.Driver == "linux-one-node" && organization.Runtime.Topology != "single-node" && !(incomplete && organization.Runtime.Topology == "") {
		return errors.New("linux-one-node runtime requires single-node topology")
	}
	if (!incomplete || len(organization.Runtime.Nodes) > 0) && (organization.Runtime.Topology == "single-node" && len(organization.Runtime.Nodes) != 1 || organization.Runtime.Topology == "multi-node" && len(organization.Runtime.Nodes) < 2) {
		return fmt.Errorf("runtime topology %q has an invalid node count", organization.Runtime.Topology)
	}
	seenNodes := make(map[string]bool, len(organization.Runtime.Nodes))
	seenSubstrateAddresses := make(map[string]bool, len(organization.Runtime.Nodes))
	seenZPRAddresses := make(map[string]bool, len(organization.Runtime.Nodes))
	for _, node := range organization.Runtime.Nodes {
		if !validScenarioID(node.ID) || strings.TrimSpace(node.Location) == "" || seenNodes[node.ID] {
			return errors.New("runtime nodes require unique valid ids and locations")
		}
		if (node.Latitude == nil) != (node.Longitude == nil) {
			return fmt.Errorf("runtime node %q requires both latitude and longitude or neither", node.ID)
		}
		if node.Latitude != nil && (math.IsNaN(*node.Latitude) || math.IsInf(*node.Latitude, 0) ||
			math.IsNaN(*node.Longitude) || math.IsInf(*node.Longitude, 0) ||
			math.Abs(*node.Latitude) > 90 || math.Abs(*node.Longitude) > 180) {
			return fmt.Errorf("runtime node %q coordinates must be finite and within latitude [-90,90] and longitude [-180,180]", node.ID)
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
	if !strings.Contains(organization.Directory.BaseDN, "=") && !(incomplete && organization.Directory.BaseDN == "") {
		return errors.New("directory base_dn is required")
	}
	if organization.Directory.SeedMode != "pregen" && organization.Directory.SeedMode != "ldif" && !(incomplete && organization.Directory.SeedMode == "") {
		return fmt.Errorf("unsupported directory seed_mode %q", organization.Directory.SeedMode)
	}
	if len(organization.Directory.LDIFFiles) == 0 && !incomplete {
		return errors.New("directory requires at least one LDIF seed file")
	}
	if !incomplete && (organization.Directory.HealthUID == "" || len(organization.Directory.IdentityMappings) == 0 || len(organization.Directory.Attributes) == 0) {
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
	if organization.WebGateway != nil {
		if _, err := gateway.NewWebGateway(organization.WebGateway.AllowedHosts); err != nil {
			return fmt.Errorf("web gateway profile: %w", err)
		}
	}
	if len(organization.WorkloadServices) > 32 {
		return errors.New("organization has too many workload service registrations")
	}
	for agent, serviceNames := range organization.WorkloadServices {
		if _, ok := machineWorkload(agent); !ok || len(serviceNames) == 0 || len(serviceNames) > 512 {
			return fmt.Errorf("workload %q has invalid service registrations", agent)
		}
		for _, serviceName := range serviceNames {
			if !policyTestValueSafe(serviceName) || strings.TrimSpace(serviceName) == "" || len(serviceName) > 200 || strings.Contains(serviceName, ",") {
				return fmt.Errorf("workload %q has invalid service registration", agent)
			}
		}
	}
	if profile := organization.LoadTest; profile != nil {
		if !validScenarioID(profile.ClientMachine) || !validScenarioID(profile.ServiceMachine) || profile.ClientMachine == profile.ServiceMachine {
			return errors.New("load test requires distinct valid client and service machines")
		}
		if !testClientWorkloads[profile.ClientWorkload] || testServicePorts[profile.ServiceWorkload] == "" {
			return errors.New("load test requires a supported client and service workload")
		}
		if profile.ClientCount < 50 || profile.ClientCount > 512 || profile.ServiceCount < 100 || profile.ServiceCount > 512 {
			return errors.New("load test client count must be between 50 and 512 and service count between 100 and 512")
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

func validOrganizationPolicyDigest(digest string) bool {
	if len(digest) != 64 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == digest
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

func simulatorWorkloadServicesForAgent(directory, organizationID, agent string) ([]string, error) {
	organization, err := loadSimulatorOrganization(directory, organizationID)
	if err != nil {
		return nil, err
	}
	if services, ok := organization.WorkloadServices[agent]; ok {
		return append([]string(nil), services...), nil
	}
	if organization.LoadTest == nil || organization.LoadTest.ServiceWorkload != agent {
		return nil, nil
	}
	return simulatorLoadTestServiceNames(*organization.LoadTest), nil
}

func simulatorWebGatewayForManifest(manifest simulatorManifest) (*simulatorWebGatewayProfile, error) {
	organization, err := loadSimulatorOrganization(simulatorOrganizationsDirectory(), activeSimulatorOrganizationID(manifest))
	if err != nil {
		return nil, err
	}
	return organization.WebGateway, nil
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
			if registry := simulatorEvaluationInstallRegistry; registry != nil {
				if binding := registry.installs[id].RetainedTopologyBinding; binding != nil {
					copy := *binding
					organization.RetainedTopologyBinding = &copy
					if runtime := registry.installs[id].RetainedMachineRuntime; runtime != nil {
						copy := *runtime
						organization.RetainedMachineRuntime = &copy
					}
					if err := validateSimulatorRetainedTopology(organization); err != nil {
						return simulatorOrganization{}, err
					}
					if err := validateSimulatorRetainedMachineRuntime(organization); err != nil {
						return simulatorOrganization{}, err
					}
					// Declared endpoints state onboarding requirements only
					// and never alias the reviewed retained inventory.
					if runtime := organization.RetainedMachineRuntime; runtime != nil {
						retained := runtime.requiredAssets()
						for _, declared := range organization.DeclaredEndpoints {
							if slices.Contains(retained, declared.AssetID) {
								return simulatorOrganization{}, fmt.Errorf(
									"organization %q declared endpoints: %q collides with the retained machine inventory", id, declared.AssetID)
							}
						}
					}
				}
			}
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

func handleSimulatorOrganizations(w http.ResponseWriter, r *http.Request) {
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
	activeID := manifest.OrganizationID
	var activation any = activeOrganizationActivation.snapshot()
	if identity, restricted := simulatorOperatorIdentity(r.Context()); restricted {
		organizations, activeID, activation = filterSimulatorOrganizationCatalog(organizations, activeID, activation, identity.Organizations)
	}
	runtime := "unknown"
	if activeID != "" {
		runtime = observedSimulatorRuntime(r.Context(), manifest)
	}
	writeSimulatorJSON(w, map[string]any{
		"active_id":      activeID,
		"active_runtime": runtime,
		"organizations":  organizations,
		"activation":     activation,
	})
}

// observedSimulatorRuntime reports whether any of the selected organization's
// components appear in the connected operator snapshot. Selection alone does
// not mean the organization is deployed into that environment.
func observedSimulatorRuntime(ctx context.Context, manifest simulatorManifest) string {
	current, err := readControlRoomSnapshotWithContext(ctx)
	if err != nil || current.APIStatus == "disconnected" {
		return "unknown"
	}
	for _, state := range simulatorComponentStates(manifest, current.Actors) {
		if state == "running" {
			return "running"
		}
	}
	return "not_running"
}

func filterSimulatorOrganizationCatalog(organizations []simulatorOrganization, activeID string, activation any, allowedIDs []string) ([]simulatorOrganization, string, any) {
	allowed := make(map[string]bool, len(allowedIDs))
	for _, organizationID := range allowedIDs {
		allowed[organizationID] = true
	}
	allowAll := allowed["*"]
	filtered := make([]simulatorOrganization, 0, len(organizations))
	for _, organization := range organizations {
		if allowAll || allowed[organization.ID] {
			filtered = append(filtered, organization)
		}
	}
	if !allowAll && !allowed[activeID] {
		return filtered, "", nil
	}
	return filtered, activeID, activation
}
