package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
)

var runtimeSourceID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// This is a narrow production Admin API bridge, not a source registration API.
func trustedSourceControlHandler(admin *adminClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		upstream := "/admin/trusted-services"
		var body io.Reader
		if r.URL.Path != "/api/trusted-sources/runtime" {
			source := r.PathValue("source")
			if !runtimeSourceID.MatchString(source) || strings.Contains(source, "__source__") {
				writePolicyError(w, http.StatusBadRequest, "Invalid trusted source ID.")
				return
			}
			upstream += "/" + source + "/control"
			if r.Method == http.MethodPut {
				data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
				enabled, decodeErr := decodeStrictSourceToggle(data)
				if err != nil || decodeErr != nil {
					writePolicyError(w, http.StatusBadRequest, "Expected exactly one boolean enabled field.")
					return
				}
				body = strings.NewReader(`{"enabled":false}`)
				if enabled {
					body = strings.NewReader(`{"enabled":true}`)
				}
			}
		}
		if admin == nil {
			writePolicyError(w, http.StatusServiceUnavailable, "Visa Service administration is not configured.")
			return
		}
		request, err := http.NewRequestWithContext(r.Context(), r.Method, admin.baseURL+upstream, body)
		if err != nil {
			log.Printf("Trusted source control request invalid: %T", err)
			writePolicyError(w, http.StatusBadGateway, "Visa Service control request is unavailable.")
			return
		}
		request.Header.Set("X-API-Key", admin.apiKey)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		client := *admin.http
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		response, err := client.Do(request)
		if err != nil {
			log.Printf("Trusted source control upstream unavailable: %T", err)
			writePolicyError(w, http.StatusBadGateway, "Visa Service is unavailable; read back state before retrying a mutation.")
			return
		}
		defer response.Body.Close()
		switch response.StatusCode {
		case http.StatusOK, http.StatusAccepted:
			data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
			if err != nil || len(data) > 1<<20 || !validRuntimeSourceReadback(data, r.URL.Path == "/api/trusted-sources/runtime") ||
				(r.Method == http.MethodPut && response.StatusCode != http.StatusAccepted) ||
				(r.Method != http.MethodPut && response.StatusCode != http.StatusOK) {
				log.Printf("Trusted source control response invalid (HTTP %d)", response.StatusCode)
				writePolicyError(w, http.StatusBadGateway, "Visa Service returned invalid control readback.")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(response.StatusCode)
			if _, err := w.Write(data); err != nil {
				log.Printf("Trusted source control response write failed: %T", err)
			}
		case http.StatusNotFound, http.StatusBadRequest, http.StatusForbidden, http.StatusServiceUnavailable:
			writePolicyError(w, response.StatusCode, "Visa Service rejected the control request; refresh readback before retrying.")
		default:
			log.Printf("Trusted source control upstream rejected (HTTP %d)", response.StatusCode)
			writePolicyError(w, http.StatusBadGateway, "Visa Service control request failed.")
		}
	})
}

func validRuntimeSourceReadback(data []byte, inventory bool) bool {
	if inventory {
		var sources []struct {
			Name    string `json:"name"`
			Enabled *bool  `json:"enabled"`
			Health  string `json:"health"`
		}
		if json.Unmarshal(data, &sources) != nil || sources == nil {
			return false
		}
		seen := map[string]bool{}
		for _, source := range sources {
			if !runtimeSourceID.MatchString(source.Name) || strings.Contains(source.Name, "__source__") ||
				source.Enabled == nil || source.Health == "" || seen[source.Name] {
				return false
			}
			seen[source.Name] = true
		}
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if validateDocumentJSON(decoder, 0) != nil {
		return false
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false
	}
	var state struct {
		Enabled          *bool   `json:"enabled"`
		Revision         *string `json:"revision"`
		Pending          *bool   `json:"pending"`
		Declared         *bool   `json:"declared"`
		EffectiveEnabled *bool   `json:"effective_enabled"`
	}
	return json.Unmarshal(data, &state) == nil && state.Enabled != nil && state.Revision != nil &&
		state.Pending != nil && state.Declared != nil && state.EffectiveEnabled != nil &&
		(!*state.EffectiveEnabled || (*state.Enabled && *state.Declared))
}

func decodeStrictSourceToggle(data []byte) (bool, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return false, io.ErrUnexpectedEOF
	}
	token, err = decoder.Token()
	if err != nil || token != "enabled" {
		return false, io.ErrUnexpectedEOF
	}
	token, err = decoder.Token()
	value, ok := token.(bool)
	if err != nil || !ok {
		return false, io.ErrUnexpectedEOF
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return false, io.ErrUnexpectedEOF
	}
	if _, err := decoder.Token(); err != io.EOF {
		return false, io.ErrUnexpectedEOF
	}
	return value, nil
}
