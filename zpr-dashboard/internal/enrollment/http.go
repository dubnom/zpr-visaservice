package enrollment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const APIPrefix = "/api/enrollment/v1/"

type Principal struct {
	Name              string   `json:"name"`
	CertificateSHA256 string   `json:"certificate_sha256"`
	Organizations     []string `json:"organizations"`
	Permissions       []string `json:"permissions"`
}

type Organization struct {
	Profiles []string `json:"profiles"`
	Types    []string `json:"types"`
}

type Config struct {
	Version                   int                     `json:"version"`
	GUIInvitationCreation     bool                    `json:"gui_invitation_creation_enabled,omitempty"`
	InvitationLifetimeSeconds int                     `json:"invitation_lifetime_seconds"`
	ApprovalLifetimeSeconds   int                     `json:"approval_lifetime_seconds,omitempty"`
	Principals                []Principal             `json:"principals"`
	Organizations             map[string]Organization `json:"organizations"`
}

func DecodeConfig(reader io.Reader) (Config, error) {
	var config Config
	if err := decodeJSON(reader, &config); err != nil {
		return Config{}, fmt.Errorf("decode enrollment configuration: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.Version != 1 || c.InvitationLifetimeSeconds < 1 || c.InvitationLifetimeSeconds > 30*24*60*60 ||
		len(c.Principals) == 0 || len(c.Organizations) == 0 {
		return errors.New("enrollment configuration requires version 1, explicit lifetime (1 second to 30 days), principals, and organizations")
	}
	if c.ApprovalLifetimeSeconds < 0 || c.ApprovalLifetimeSeconds > 30*24*60*60 {
		return errors.New("enrollment approval lifetime must be between 1 second and 30 days when configured")
	}
	for name, organization := range c.Organizations {
		if name == "*" || !validText(name) || !validNames(organization.Profiles) || !validNames(organization.Types) {
			return errors.New("enrollment organizations require valid names, approved profiles, and device types")
		}
	}
	pins := map[string]bool{}
	names := map[string]bool{}
	for _, principal := range c.Principals {
		pin, err := hex.DecodeString(principal.CertificateSHA256)
		if !validText(principal.Name) || names[principal.Name] || err != nil || len(pin) != sha256.Size ||
			hex.EncodeToString(pin) != principal.CertificateSHA256 || pins[principal.CertificateSHA256] ||
			!validNames(principal.Organizations) || !validNames(principal.Permissions) {
			return errors.New("enrollment principals require unique names and lowercase certificate SHA-256 pins with explicit scope and permissions")
		}
		names[principal.Name], pins[principal.CertificateSHA256] = true, true
		for _, name := range principal.Organizations {
			if _, exists := c.Organizations[name]; !exists {
				return errors.New("enrollment principal references an unknown organization")
			}
		}
		for _, permission := range principal.Permissions {
			if !slices.Contains([]string{"read", "create", "cancel", "approve", "reject"}, permission) {
				return errors.New("unknown enrollment permission")
			}
			if (permission == "approve" || permission == "reject") && c.ApprovalLifetimeSeconds == 0 {
				return errors.New("enrollment review permissions require an explicit approval lifetime")
			}
		}
	}
	return nil
}

func validNames(names []string) bool {
	if len(names) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !validText(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

type adminAPI struct {
	store    *Store
	config   Config
	now      func() time.Time
	operator bool
}

func NewAdminHandler(store *Store, config Config) (http.Handler, error) {
	return newAdminHandler(store, config, false)
}

func newAdminHandler(store *Store, config Config, operator bool) (http.Handler, error) {
	if store == nil {
		return nil, errors.New("enrollment registry is required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	// Snapshot configuration so later caller mutation cannot change permissions.
	data, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	var snapshot Config
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	api := &adminAPI{store: store, config: snapshot, now: time.Now, operator: operator}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+APIPrefix+"catalog", api.catalog)
	mux.HandleFunc("GET "+APIPrefix+"invitations", api.list)
	mux.HandleFunc("POST "+APIPrefix+"invitations", api.create)
	mux.HandleFunc("GET "+APIPrefix+"invitations/{id}", api.get)
	mux.HandleFunc("POST "+APIPrefix+"invitations/{id}/cancel", api.cancel)
	mux.HandleFunc("POST "+APIPrefix+"invitations/{id}/approve", api.decide)
	mux.HandleFunc("POST "+APIPrefix+"invitations/{id}/reject", api.decide)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		// These are direct mTLS administration endpoints, not browser APIs.
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			apiError(w, http.StatusForbidden, "Browser enrollment administration requires named-user authorization.")
			return
		}
		if _, ok := api.principal(r); !ok {
			log.Print("Enrollment administration denied: unrecognized or unverified client certificate")
			apiError(w, http.StatusForbidden, "An explicitly authorized, verified client certificate is required.")
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func (api *adminAPI) principal(r *http.Request) (Principal, bool) {
	if api.operator {
		principal, ok := r.Context().Value(operatorPrincipalKey{}).(Principal)
		return principal, ok
	}
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || len(r.TLS.VerifiedChains) == 0 {
		return Principal{}, false
	}
	leaf := r.TLS.PeerCertificates[0]
	verified := false
	for _, chain := range r.TLS.VerifiedChains {
		if len(chain) > 0 && chain[0].Equal(leaf) {
			verified = true
		}
	}
	if !verified {
		return Principal{}, false
	}
	hash := sha256.Sum256(leaf.Raw)
	pin := hex.EncodeToString(hash[:])
	for _, principal := range api.config.Principals {
		if principal.CertificateSHA256 == pin {
			return principal, true
		}
	}
	return Principal{}, false
}

func (api *adminAPI) authorize(w http.ResponseWriter, r *http.Request, organization, permission string) (Principal, bool) {
	principal, ok := api.principal(r)
	if !ok || !slices.Contains(principal.Permissions, permission) ||
		!slices.Contains(principal.Organizations, organization) {
		log.Print("Enrollment administration denied: organization or permission outside authorized scope")
		apiError(w, http.StatusForbidden, "Enrollment permission or organization is not authorized.")
		return Principal{}, false
	}
	return principal, true
}

func (api *adminAPI) catalog(w http.ResponseWriter, r *http.Request) {
	principal, _ := api.principal(r)
	if !slices.Contains(principal.Permissions, "read") {
		apiError(w, http.StatusForbidden, "Enrollment read permission is required.")
		return
	}
	organizations := map[string]Organization{}
	createOrganizations := []string{}
	cancelOrganizations := []string{}
	for _, name := range principal.Organizations {
		organizations[name] = api.config.Organizations[name]
		if api.operator && api.config.GUIInvitationCreation && slices.Contains(principal.Permissions, "create") {
			createOrganizations = append(createOrganizations, name)
		}
		if api.operator && slices.Contains(principal.Permissions, "cancel") {
			cancelOrganizations = append(cancelOrganizations, name)
		}
	}
	slices.Sort(createOrganizations)
	slices.Sort(cancelOrganizations)
	apiJSON(w, http.StatusOK, struct {
		Organizations       map[string]Organization `json:"organizations"`
		Lifetime            int                     `json:"invitation_lifetime_seconds"`
		ApprovalLifetime    int                     `json:"approval_lifetime_seconds"`
		GUIMutations        bool                    `json:"gui_mutations_enabled"`
		CreateOrganizations []string                `json:"gui_create_organizations"`
		CancelOrganizations []string                `json:"gui_cancel_organizations"`
	}{organizations, api.config.InvitationLifetimeSeconds, api.config.ApprovalLifetimeSeconds, false, createOrganizations, cancelOrganizations})
}

func (api *adminAPI) create(w http.ResponseWriter, r *http.Request) {
	if api.operator && !api.config.GUIInvitationCreation {
		log.Print("Operator invitation creation denied: not enabled in enrollment configuration")
		apiError(w, http.StatusForbidden, "Named-user invitation creation is not enabled at Control-Service.")
		return
	}
	var asset Asset
	if !readBody(w, r, &asset) {
		return
	}
	principal, ok := api.authorize(w, r, asset.Organization, "create")
	if !ok {
		return
	}
	organization := api.config.Organizations[asset.Organization]
	if !slices.Contains(organization.Profiles, asset.Profile) || !slices.Contains(organization.Types, asset.Type) {
		apiError(w, http.StatusBadRequest, "Choose an approved profile and device type.")
		return
	}
	now := api.now().UTC()
	item, code, err := api.store.Create(r.Context(), asset, principal.Name, now,
		now.Add(time.Duration(api.config.InvitationLifetimeSeconds)*time.Second))
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusCreated, struct {
		Invitation Invitation `json:"invitation"`
		Code       string     `json:"enrollment_code"`
	}{item, code})
}

func (api *adminAPI) list(w http.ResponseWriter, r *http.Request) {
	organization := r.URL.Query().Get("organization")
	if _, ok := api.authorize(w, r, organization, "read"); !ok {
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			apiError(w, http.StatusBadRequest, "Limit must be between 1 and 100.")
			return
		}
		limit = value
	}
	items, err := api.store.List(r.Context(), organization, r.URL.Query().Get("after"), limit+1, api.now())
	if err != nil {
		storeError(w, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		next = items[len(items)-1].ID
	}
	apiJSON(w, http.StatusOK, struct {
		Invitations []Invitation `json:"invitations"`
		Next        string       `json:"next_after,omitempty"`
	}{items, next})
}

func (api *adminAPI) get(w http.ResponseWriter, r *http.Request) {
	organization := r.URL.Query().Get("organization")
	if _, ok := api.authorize(w, r, organization, "read"); !ok {
		return
	}
	item, err := api.store.Get(r.Context(), organization, r.PathValue("id"), api.now())
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, item)
}

func (api *adminAPI) cancel(w http.ResponseWriter, r *http.Request) {
	if api.operator {
		var input struct {
			Organization   string `json:"organization"`
			Revision       int    `json:"revision"`
			KeyFingerprint string `json:"key_fingerprint"`
			Reason         string `json:"reason"`
		}
		if !readBody(w, r, &input) {
			return
		}
		principal, ok := api.authorize(w, r, input.Organization, "cancel")
		if !ok {
			return
		}
		item, err := api.store.CancelReviewed(r.Context(), input.Organization, r.PathValue("id"), principal.Name,
			input.Reason, input.KeyFingerprint, input.Revision, api.now())
		if err != nil {
			storeError(w, err)
			return
		}
		apiJSON(w, http.StatusOK, item)
		return
	}
	var input struct {
		Organization string `json:"organization"`
	}
	if !readBody(w, r, &input) {
		return
	}
	principal, ok := api.authorize(w, r, input.Organization, "cancel")
	if !ok {
		return
	}
	item, err := api.store.Cancel(r.Context(), input.Organization, r.PathValue("id"), principal.Name, api.now())
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, item)
}

func (api *adminAPI) decide(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Organization   string `json:"organization"`
		Revision       int    `json:"revision"`
		KeyFingerprint string `json:"key_fingerprint"`
		Reason         string `json:"reason"`
	}
	if !readBody(w, r, &input) {
		return
	}
	permission, decision := "reject", "rejected"
	if strings.HasSuffix(r.URL.Path, "/approve") {
		permission, decision = "approve", "approved"
	}
	principal, ok := api.authorize(w, r, input.Organization, permission)
	if !ok {
		return
	}
	if decision == "approved" {
		item, err := api.store.Get(r.Context(), input.Organization, r.PathValue("id"), api.now())
		if err != nil {
			storeError(w, err)
			return
		}
		organization := api.config.Organizations[input.Organization]
		if !slices.Contains(organization.Profiles, item.Asset.Profile) || !slices.Contains(organization.Types, item.Asset.Type) {
			apiError(w, http.StatusConflict, "The requested profile or device type is no longer approved.")
			return
		}
	}
	item, err := api.store.Decide(r.Context(), input.Organization, r.PathValue("id"), principal.Name,
		decision, input.Reason, input.KeyFingerprint, input.Revision, api.now())
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, item)
}

func decodeJSON(reader io.Reader, value any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("expected exactly one JSON value")
	} else if err != io.EOF {
		return err
	}
	return nil
}

func readBody(w http.ResponseWriter, r *http.Request, value any) bool {
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		apiError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json.")
		return false
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, "Mutation parameters belong in the JSON body, not the URL.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err := decodeJSON(r.Body, value); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			apiError(w, http.StatusRequestEntityTooLarge, "Enrollment request exceeds 8192 bytes.")
		} else {
			apiError(w, http.StatusBadRequest, "Invalid enrollment JSON request.")
		}
		return false
	}
	return true
}

func storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalid):
		apiError(w, http.StatusBadRequest, "Invalid enrollment fields.")
	case errors.Is(err, ErrNotFound):
		apiError(w, http.StatusNotFound, "Invitation not found.")
	case errors.Is(err, ErrRevision):
		apiError(w, http.StatusConflict, "Enrollment changed since review; reload it before deciding.")
	case errors.Is(err, ErrConflict), errors.Is(err, ErrUnavailable):
		apiError(w, http.StatusConflict, "The asset or invitation is not available for this operation.")
	default:
		log.Printf("Enrollment registry operation failed: %T", err)
		apiError(w, http.StatusInternalServerError, "Enrollment registry operation failed.")
	}
}

func apiError(w http.ResponseWriter, status int, message string) {
	apiJSON(w, status, struct {
		Error string `json:"error"`
	}{message})
}

func apiJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("Enrollment response delivery failed: %T", err)
	}
}
