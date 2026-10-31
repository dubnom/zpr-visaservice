package main

import (
	"net/http"
	"os"

	"neboagency.com/zpr-dashborad/internal/networkmetadata"
)

func configuredNetworkMetadata() (http.Handler, error) {
	metadata := networkmetadata.Metadata{Name: os.Getenv("ZPR_NETWORK_NAME")}
	if err := metadata.Validate(); err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			http.Error(w, "network metadata does not accept query parameters", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, metadata)
	}), nil
}
