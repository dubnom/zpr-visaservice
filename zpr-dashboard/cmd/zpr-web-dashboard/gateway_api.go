package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"neboagency.com/zpr-dashborad/internal/controlservice/gatewaydraft"
)

type gatewayContractsResponse struct {
	OrganizationID string                    `json:"organization_id"`
	Contracts      []gatewayInstanceContract `json:"contracts"`
}

type gatewayConfigsResponse struct {
	OrganizationID string                `json:"organization_id"`
	Configs        []gatewayConfigRecord `json:"configs"`
}

type gatewayConfigCheckRequest struct {
	Config json.RawMessage `json:"config"`
}

type gatewayConfigSaveRequest struct {
	ExpectedRevision *int            `json:"expected_revision"`
	Config           json.RawMessage `json:"config"`
}

type gatewayConfigCheckResponse struct {
	Valid       bool                    `json:"valid"`
	Diagnostics string                  `json:"diagnostics"`
	SourceLine  int                     `json:"source_line,omitempty"`
	Contract    gatewayInstanceContract `json:"contract,omitempty"`
}

func newGatewayAPI(organizationID string, readInventory gatewaydraft.Reader, store *gatewayConfigStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		if organizationID == "" || readInventory == nil {
			writePolicyError(w, http.StatusServiceUnavailable, "Gateway organization context is unavailable.")
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/gateways/contracts":
			data, err := readInventory(r.Context())
			if err != nil {
				writePolicyError(w, http.StatusServiceUnavailable, "Live gateway service inventory is unavailable; contracts cannot be verified.")
				return
			}
			contracts := gatewaydraft.InstalledContracts(organizationID, data)
			writeJSON(w, http.StatusOK, gatewayContractsResponse{OrganizationID: organizationID, Contracts: contracts})
		case r.Method == http.MethodGet && r.URL.Path == "/api/gateways/configs":
			if store == nil {
				writePolicyError(w, http.StatusServiceUnavailable, "Gateway draft storage is not configured.")
				return
			}
			configs, err := store.List(organizationID)
			if err != nil {
				writePolicyError(w, http.StatusInternalServerError, "Unable to read gateway drafts.")
				return
			}
			writeJSON(w, http.StatusOK, gatewayConfigsResponse{OrganizationID: organizationID, Configs: configs})
		case r.Method == http.MethodPost && r.URL.Path == "/api/gateways/config/check":
			var request gatewayConfigCheckRequest
			if !decodePolicyRequest(w, r, maxGatewayConfigBytes, &request) {
				return
			}
			if len(request.Config) == 0 {
				writePolicyError(w, http.StatusBadRequest, "Gateway configuration is required.")
				return
			}
			var identity struct {
				InstanceID string `json:"instance_id"`
			}
			if err := json.Unmarshal(request.Config, &identity); err != nil || identity.InstanceID == "" {
				writePolicyError(w, http.StatusBadRequest, "Gateway configuration must include an instance_id.")
				return
			}
			data, err := readInventory(r.Context())
			if err != nil {
				writePolicyError(w, http.StatusServiceUnavailable, "Live gateway service inventory is unavailable; config was not checked.")
				return
			}
			contract, found := gatewaydraft.FindInstalledContract(organizationID, identity.InstanceID, data)
			if !found {
				writePolicyError(w, http.StatusConflict, "No live Gateway service matches this organization and instance ID.")
				return
			}
			if _, err := parseGatewayInstanceConfig(request.Config, contract); err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, gatewayConfigCheckResponse{Valid: false, Diagnostics: err.Error(), SourceLine: gatewaydraft.DiagnosticLine(request.Config, err), Contract: contract})
				return
			}
			writeJSON(w, http.StatusOK, gatewayConfigCheckResponse{Valid: true, Diagnostics: "Gateway draft matches the live Gateway service identity; runtime configuration is unchanged.", Contract: contract})
		case strings.HasPrefix(r.URL.Path, "/api/gateways/configs/"):
			if store == nil {
				writePolicyError(w, http.StatusServiceUnavailable, "Gateway draft storage is not configured.")
				return
			}
			pathSuffix := strings.TrimPrefix(r.URL.Path, "/api/gateways/configs/")
			if r.Method == http.MethodGet && pathSuffix != "" && !strings.Contains(pathSuffix, "/") {
				record, err := store.Get(organizationID, pathSuffix)
				if errors.Is(err, os.ErrNotExist) {
					writePolicyError(w, http.StatusNotFound, "Gateway draft was not found.")
					return
				}
				if err != nil {
					writePolicyError(w, http.StatusInternalServerError, "Unable to read gateway draft.")
					return
				}
				writeJSON(w, http.StatusOK, record)
				return
			}
			if r.Method == http.MethodPost && strings.HasSuffix(pathSuffix, "/revisions") {
				instanceID := strings.TrimSuffix(pathSuffix, "/revisions")
				if !validGatewayIdentifier(instanceID) {
					writePolicyError(w, http.StatusBadRequest, "Gateway instance ID is invalid.")
					return
				}
				var request gatewayConfigSaveRequest
				if !decodePolicyRequest(w, r, maxGatewayConfigBytes, &request) {
					return
				}
				if request.ExpectedRevision == nil || len(request.Config) == 0 {
					writePolicyError(w, http.StatusBadRequest, "expected_revision and config are required.")
					return
				}
				if *request.ExpectedRevision < 0 {
					writePolicyError(w, http.StatusBadRequest, "expected_revision cannot be negative.")
					return
				}
				data, err := readInventory(r.Context())
				if err != nil {
					writePolicyError(w, http.StatusServiceUnavailable, "Live gateway service inventory is unavailable; draft was not saved.")
					return
				}
				contract, found := gatewaydraft.FindInstalledContract(organizationID, instanceID, data)
				if !found {
					writePolicyError(w, http.StatusConflict, "No live Gateway service matches this organization and instance ID.")
					return
				}
				if _, err := parseGatewayInstanceConfig(request.Config, contract); err != nil {
					writeJSON(w, http.StatusUnprocessableEntity, gatewayConfigCheckResponse{Valid: false, Diagnostics: err.Error(), Contract: contract})
					return
				}
				record, err := store.Save(organizationID, instanceID, *request.ExpectedRevision, request.Config)
				if errors.Is(err, errGatewayConfigRevisionConflict) {
					writePolicyError(w, http.StatusConflict, "Gateway draft revision changed; reload before saving.")
					return
				}
				if err != nil {
					writePolicyError(w, http.StatusInternalServerError, "Unable to save gateway draft.")
					return
				}
				writeJSON(w, http.StatusOK, record)
				return
			}
			writePolicyError(w, http.StatusMethodNotAllowed, "Gateway configuration method or path is not supported.")
		default:
			writePolicyError(w, http.StatusMethodNotAllowed, "Gateway endpoint method or path is not supported.")
		}
	})
}
