package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	organizationAssertionsCategory = "Assertions"
	organizationAssertionsName     = "Organization assertions"
	organizationAssertionsKind     = "assertions"
)

type storedAssertionSettings struct {
	Source          string `json:"source"`
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
}

type assertionSettingsResponse struct {
	OrganizationID   string            `json:"organization_id,omitempty"`
	OrganizationName string            `json:"organization_name,omitempty"`
	BaseDN           string            `json:"base_dn,omitempty"`
	Settings         assertionSettings `json:"settings"`
}

type assertionSettingsSaveRequest struct {
	Source                 string `json:"source"`
	Enabled                bool   `json:"enabled"`
	IntervalSeconds        int    `json:"interval_seconds"`
	ExpectedRevision       int    `json:"expected_revision"`
	ExpectedOrganizationID string `json:"expected_organization_id"`
}

type assertionSettingsStore interface {
	Load(context.Context) (assertionSettingsResponse, error)
	Save(context.Context, assertionSettings, int, string) (assertionSettingsResponse, error)
}

type policyServiceAssertionSettingsStore struct {
	client   *http.Client
	endpoint string
}

func newPolicyServiceAssertionSettingsStore() (assertionSettingsStore, string) {
	baseURL, transport, message := newPolicyServiceTransport()
	if message != "" {
		return nil, message
	}
	return &policyServiceAssertionSettingsStore{
		client: &http.Client{Transport: transport, Timeout: 12 * time.Second}, endpoint: baseURL.String(),
	}, ""
}

func (store *policyServiceAssertionSettingsStore) Load(ctx context.Context) (assertionSettingsResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, store.endpoint+"/api/assertions/settings", nil)
	if err != nil {
		return assertionSettingsResponse{}, err
	}
	return store.do(request)
}

func (store *policyServiceAssertionSettingsStore) Save(ctx context.Context, settings assertionSettings, expected int, organizationID string) (assertionSettingsResponse, error) {
	body, err := json.Marshal(assertionSettingsSaveRequest{
		Source: settings.Source, Enabled: settings.Enabled, IntervalSeconds: settings.IntervalSeconds,
		ExpectedRevision: expected, ExpectedOrganizationID: organizationID,
	})
	if err != nil {
		return assertionSettingsResponse{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, store.endpoint+"/api/assertions/settings", strings.NewReader(string(body)))
	if err != nil {
		return assertionSettingsResponse{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	return store.do(request)
}

func (store *policyServiceAssertionSettingsStore) do(request *http.Request) (assertionSettingsResponse, error) {
	request.Header.Set("Accept", "application/json")
	response, err := store.client.Do(request)
	if err != nil {
		return assertionSettingsResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if err != nil {
		return assertionSettingsResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &failure) == nil && failure.Error != "" {
			if response.StatusCode == http.StatusConflict {
				return assertionSettingsResponse{}, fmt.Errorf("%w: %s", errAssertionRevision, failure.Error)
			}
			return assertionSettingsResponse{}, fmt.Errorf("%s", failure.Error)
		}
		return assertionSettingsResponse{}, fmt.Errorf("Policy Repository returned HTTP %d", response.StatusCode)
	}
	var result assertionSettingsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return assertionSettingsResponse{}, errors.New("Policy Repository returned invalid assertion settings")
	}
	return result, nil
}

func seedOrganizationAssertions(ctx context.Context, repository policyRepository) error {
	categories, records, err := repository.Catalog(ctx)
	if err != nil {
		return err
	}
	var categoryID string
	for _, category := range categories {
		if category.Path == organizationAssertionsCategory {
			categoryID = category.ID
			break
		}
	}
	if categoryID == "" {
		category, err := repository.CreateCategory(ctx, nil, organizationAssertionsCategory)
		if err != nil {
			return err
		}
		categoryID = category.ID
	}
	for _, record := range records {
		if record.CategoryID == categoryID && record.Name == organizationAssertionsName {
			if record.Kind != organizationAssertionsKind {
				return errors.New("organization assertion record has an incompatible kind")
			}
			return nil
		}
	}
	content, err := json.Marshal(storedAssertionSettings{IntervalSeconds: 60})
	if err != nil {
		return err
	}
	_, err = repository.CreateRecord(ctx, categoryID, organizationAssertionsName, organizationAssertionsKind, "application/vnd.zpr.assertions+json", json.RawMessage(`{"scope":"organization"}`), string(content), "system", "Initial organization assertion settings")
	return err
}

func organizationAssertionRecord(ctx context.Context, repository policyRepository) (policyRecord, error) {
	categories, records, err := repository.Catalog(ctx)
	if err != nil {
		return policyRecord{}, err
	}
	var categoryID string
	for _, category := range categories {
		if category.Path == organizationAssertionsCategory {
			categoryID = category.ID
			break
		}
	}
	for _, record := range records {
		if record.CategoryID == categoryID && record.Name == organizationAssertionsName && record.Kind == organizationAssertionsKind {
			return repository.GetRecord(ctx, record.ID)
		}
	}
	return policyRecord{}, errRecordNotFound
}

func loadOrganizationAssertionSettings(ctx context.Context, repository policyRepository) (assertionSettings, error) {
	record, err := organizationAssertionRecord(ctx, repository)
	if err != nil {
		return assertionSettings{}, err
	}
	var stored storedAssertionSettings
	if err := json.Unmarshal([]byte(record.Content), &stored); err != nil {
		return assertionSettings{}, errors.New("organization assertion record is invalid")
	}
	settings := assertionSettings{Revision: record.CurrentRevision, Source: stored.Source, Enabled: stored.Enabled, IntervalSeconds: stored.IntervalSeconds}
	if err := validateStoredAssertionSettings(settings); err != nil {
		return assertionSettings{}, errors.New("organization assertion record is invalid")
	}
	return settings, nil
}

func validateStoredAssertionSettings(settings assertionSettings) error {
	rules, err := parseAssertions(settings.Source)
	if err != nil {
		return err
	}
	if settings.IntervalSeconds < 30 || settings.IntervalSeconds > 3600 {
		return errors.New("Interval must be between 30 and 3600 seconds")
	}
	if settings.Enabled && len(rules) == 0 {
		return errors.New("Periodic checks require at least one assertion")
	}
	return nil
}

func saveOrganizationAssertionSettings(ctx context.Context, repository policyRepository, settings assertionSettings, expected int) (assertionSettings, error) {
	if err := validateStoredAssertionSettings(settings); err != nil {
		return assertionSettings{}, err
	}
	record, err := organizationAssertionRecord(ctx, repository)
	if err != nil {
		return assertionSettings{}, err
	}
	content, err := json.Marshal(storedAssertionSettings{Source: settings.Source, Enabled: settings.Enabled, IntervalSeconds: settings.IntervalSeconds})
	if err != nil {
		return assertionSettings{}, err
	}
	_, err = repository.AppendRevision(ctx, record.ID, expected, string(content), policyAuthor(), "Updated organization assertions and schedule")
	if err != nil {
		return assertionSettings{}, err
	}
	settings.Revision = expected + 1
	return settings, nil
}

func (a *application) handleGetAssertionSettings(w http.ResponseWriter, r *http.Request) {
	if !localEditorRequest(w, r) {
		return
	}
	if a.policy == nil {
		writePolicyError(w, http.StatusServiceUnavailable, a.policyErr)
		return
	}
	settings, err := loadOrganizationAssertionSettings(r.Context(), a.policy.store)
	if err != nil {
		writePolicyStoreError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, assertionSettingsResponse{
		OrganizationID: os.Getenv("ZPR_POLICY_ORGANIZATION_ID"), OrganizationName: os.Getenv("ZPR_POLICY_ORGANIZATION_NAME"),
		BaseDN: os.Getenv("ZPR_POLICY_ORGANIZATION_BASE_DN"), Settings: settings,
	})
}

func (a *application) handleSaveAssertionSettings(w http.ResponseWriter, r *http.Request) {
	if !localEditorRequest(w, r) {
		return
	}
	if a.policy == nil {
		writePolicyError(w, http.StatusServiceUnavailable, a.policyErr)
		return
	}
	var request assertionSettingsSaveRequest
	if err := decodeAssertionRequest(w, r, &request); err != nil {
		assertionHTTPError(w, err)
		return
	}
	if request.ExpectedOrganizationID == "" || request.ExpectedOrganizationID != os.Getenv("ZPR_POLICY_ORGANIZATION_ID") {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "The active organization changed; reload assertion settings before saving"})
		return
	}
	settings, err := saveOrganizationAssertionSettings(r.Context(), a.policy.store, assertionSettings{
		Source: request.Source, Enabled: request.Enabled, IntervalSeconds: request.IntervalSeconds,
	}, request.ExpectedRevision)
	if err != nil {
		writePolicyStoreError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, assertionSettingsResponse{
		OrganizationID: os.Getenv("ZPR_POLICY_ORGANIZATION_ID"), OrganizationName: os.Getenv("ZPR_POLICY_ORGANIZATION_NAME"),
		BaseDN: os.Getenv("ZPR_POLICY_ORGANIZATION_BASE_DN"), Settings: settings,
	})
}
