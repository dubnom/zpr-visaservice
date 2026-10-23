package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/controlservice/adapterlogs"
)

type evaluationRegistration struct {
	OrganizationID string         `json:"organization_id"`
	SourceID       string         `json:"source_id"`
	Revision       string         `json:"revision"`
	Config         map[string]any `json:"config"`
}

func evaluationSourceRegistration(state string, inventory evaluationInventory) evaluationRegistration {
	org, source := "evaluation-"+inventory.InstallID, "directory_"+inventory.InstallID
	return evaluationRegistration{OrganizationID: org, SourceID: source, Config: map[string]any{
		"name": source, "display_name": "Starter directory (EVALUATION ONLY)", "kind": "ldap",
		"url": fmt.Sprintf("ldaps://localhost:%d", inventory.Config.LDAPPort), "ca_file": filepath.Join(state, "ca.pem"),
		"attributes":    []string{"uid", "cn", "mail"},
		"organizations": map[string]any{org: map[string]string{"base_dn": "dc=evaluation,dc=zpr", "bind_dn": "cn=zpr-reader,dc=evaluation,dc=zpr", "bind_password_file": filepath.Join(state, "ldap-reader.secret")}},
	}}
}

func evaluationLogSource(state string, inventory evaluationInventory) adapterlogs.SourceConfig {
	return adapterlogs.SourceConfig{Name: "OpenObserve (EVALUATION ONLY)", Kind: "controller", URL: fmt.Sprintf("https://localhost:%d", inventory.Config.LogPort), CAFile: filepath.Join(state, "ca.pem"), PasswordFile: filepath.Join(state, "logging-admin.secret"), Username: "admin@evaluation.invalid", Organization: "default", Stream: "lifecycle_" + inventory.InstallID}
}

func registerEvaluationSource(ctx context.Context, state string, inventory evaluationInventory) error {
	tlsConfig, err := evaluationTLS(state, true)
	if err != nil {
		return err
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return registerEvaluationSourceWithClient(ctx, state, inventory, client, fmt.Sprintf("https://localhost:%d", inventory.Config.ServicePort))
}

func registerEvaluationSourceWithClient(ctx context.Context, state string, inventory evaluationInventory, client *http.Client, endpoint string) error {
	fail := errors.New("evaluation source registration/read failed; inspect protected configuration; no fallback or reseed attempted")
	call := func(method, path string, value any, result any) error {
		var body []byte
		if value != nil {
			body, _ = json.Marshal(value)
		}
		request, err := http.NewRequestWithContext(ctx, method, endpoint+path, bytes.NewReader(body))
		if err != nil {
			return fail
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return fail
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("evaluation source contract %s %s returned HTTP %d; no fallback attempted", method, request.URL.Path, response.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
		if err != nil || len(data) > 65536 {
			return fail
		}
		if result != nil && json.Unmarshal(data, result) != nil {
			return fail
		}
		return nil
	}
	var registry struct {
		OrganizationID string `json:"organization_id"`
		Revision       string `json:"revision"`
		Durable        bool   `json:"durable"`
		Sources        []struct {
			SourceID string         `json:"source_id"`
			Editable bool           `json:"editable"`
			Config   map[string]any `json:"config"`
		} `json:"sources"`
	}
	if err := call(http.MethodGet, "/api/assertions/sources", nil, &registry); err != nil {
		return err
	}
	registration := evaluationSourceRegistration(state, inventory)
	if registry.OrganizationID != registration.OrganizationID || !registry.Durable {
		return fail
	}
	registration.Revision = registry.Revision
	exists := false
	for _, source := range registry.Sources {
		if source.SourceID != registration.SourceID {
			continue
		}
		if !source.Editable || source.Config == nil {
			return fail
		}
		registration.Config = source.Config
		exists = true
	}
	if err := call(http.MethodPost, "/api/assertions/sources/test", registration, nil); err != nil {
		return err
	}
	if !exists {
		if err := call(http.MethodPost, "/api/assertions/sources", registration, nil); err != nil {
			return err
		}
	}
	var snapshot struct {
		OrganizationID string `json:"organization_id"`
		SourceID       string `json:"source_id"`
	}
	if err := call(http.MethodGet, "/api/assertions/source?source="+url.QueryEscape(registration.SourceID), nil, &snapshot); err != nil {
		return err
	}
	if snapshot.OrganizationID != registration.OrganizationID || snapshot.SourceID != registration.SourceID {
		return fail
	}
	return nil
}

func collectEvaluationLog(ctx context.Context, state string, inventory evaluationInventory, event string) error {
	config, err := adapterlogs.ReadInventory(filepath.Join(state, "adapter-logs.json"))
	if err != nil {
		return errors.New("evaluation logging inventory unavailable")
	}
	var selected *adapterlogs.SourceConfig
	for _, target := range config.Adapters {
		if target.ID == "logs_"+inventory.InstallID {
			if len(target.Sources) != 1 || target.Sources[0].URL == "" {
				return errors.New("evaluation logging destination changed; no fallback attempted")
			}
			source := target.Sources[0]
			selected = &source
		}
	}
	if selected == nil {
		return errors.New("evaluation logging destination missing; no fallback attempted")
	}
	data, _ := json.Marshal([]map[string]any{{"_timestamp": time.Now().UnixMicro(), "event": event, "install_id": inventory.InstallID, "source_id": "logs_" + inventory.InstallID, "organization_id": "evaluation-" + inventory.InstallID, "label": "EVALUATION ONLY"}})
	if err := adapterlogs.Append(ctx, *selected, data); err != nil {
		return errors.New("evaluation lifecycle log ingestion failed")
	}
	if output, err := adapterlogs.Query(ctx, *selected); err != nil || !strings.Contains(output, inventory.InstallID) {
		return errors.New("evaluation lifecycle log query failed")
	}
	return nil
}
