//go:build !production

package main

import (
	"log"
	"net/http"
	"os"

	"neboagency.com/zpr-dashborad/internal/networkmetadata"
)

func handleSimulatorNetworkMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		http.Error(w, "network metadata does not accept query parameters", http.StatusBadRequest)
		return
	}
	target := simulatorOperatorTarget{
		ServiceURL: os.Getenv("SIMULATOR_OPERATOR_SERVICE_URL"),
		CertFile:   os.Getenv("SIMULATOR_OPERATOR_CLIENT_CERT_FILE"),
		KeyFile:    os.Getenv("SIMULATOR_OPERATOR_CLIENT_KEY_FILE"),
		CAFile:     os.Getenv("SIMULATOR_OPERATOR_SERVICE_CA_FILE"),
	}
	if registry := simulatorEvaluationInstallRegistry; registry != nil {
		organizationID, authorized := simulatorAuthorizedOrganization(r.Context())
		if !authorized {
			selected, err := registry.selectOrganization()
			if err != nil {
				log.Printf("Network metadata selection failed: %T", err)
				http.Error(w, "network metadata is unavailable", http.StatusServiceUnavailable)
				return
			}
			organizationID = selected
		}
		var err error
		target, err = registry.target(organizationID)
		if err != nil {
			log.Printf("Network metadata target failed: %T", err)
			http.Error(w, "network metadata is unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	var response struct {
		Name *string `json:"name"`
	}
	if err := readOperatorJSON(r.Context(), target, "/api/network", &response); err != nil {
		log.Printf("Production network metadata read failed: %T", err)
		http.Error(w, "network metadata is unavailable", http.StatusServiceUnavailable)
		return
	}
	if response.Name == nil {
		log.Print("Production network metadata response omitted the name field")
		http.Error(w, "network metadata is unavailable", http.StatusBadGateway)
		return
	}
	metadata := networkmetadata.Metadata{Name: *response.Name}
	if err := metadata.Validate(); err != nil {
		log.Printf("Production network metadata is invalid: %T", err)
		http.Error(w, "network metadata is unavailable", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, metadata)
}
