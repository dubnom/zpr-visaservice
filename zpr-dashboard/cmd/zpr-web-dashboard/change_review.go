package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

const changeReviewPrefix = "/api/change-review/"

var reviewIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type changeReviewConfig struct {
	Version           int    `json:"version"`
	Database          string `json:"database"`
	AllowSelfApproval bool   `json:"allow_self_approval"`
}
type changeReviewCapture func(context.Context, string, string, string, int) (changereview.Proposal, error)
type changeReviewAPI struct {
	store     *changereview.Store
	auth      operatorRequestAuthorizer
	capture   changeReviewCapture
	allowSelf bool
}
type changeReviewInput struct {
	Organization    string `json:"organization"`
	Kind            string `json:"kind"`
	Target          string `json:"target"`
	Revision        int    `json:"revision"`
	Reason          string `json:"reason"`
	Digest          string `json:"digest"`
	ExpectedVersion int    `json:"expected_version"`
}

func configuredChangeReview(auth *operatorauth.Auth, proxy http.Handler) (http.Handler, func(), error) {
	path := strings.TrimSpace(os.Getenv("ZPR_CHANGE_REVIEW_CONFIG_FILE"))
	if path == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writePolicyError(w, http.StatusServiceUnavailable, "Change review is not configured.")
		}), func() {}, nil
	}
	if auth == nil {
		return nil, nil, errors.New("change review requires named-user OIDC authorization")
	}
	data, err := readOperatorFile(path, 65536, false)
	if err != nil {
		return nil, nil, err
	}
	var config changeReviewConfig
	if err = decodeOperatorConfig(data, &config); err != nil {
		return nil, nil, err
	}
	if config.Version != 1 {
		return nil, nil, errors.New("change review configuration requires version 1")
	}
	store, err := changereview.Open(config.Database)
	if err != nil {
		return nil, nil, err
	}
	return &changeReviewAPI{store: store, auth: auth, capture: changeReviewDomainCapture(proxy), allowSelf: config.AllowSelfApproval},
		func() {
			if err := store.Close(); err != nil {
				log.Printf("Change review database close failed: %T", err)
			}
		}, nil
}
func reviewPrincipal(id operatorauth.Identity) changereview.Principal {
	return changereview.Principal{Issuer: id.Issuer, Subject: id.Subject, Name: id.DisplayName}
}
func (a *changeReviewAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	fail := func(status int, message string) {
		log.Printf("Change review request rejected (HTTP %d)", status)
		writePolicyError(w, status, message)
	}
	if operatorAuthorizationDisabled(a.auth) {
		fail(http.StatusForbidden, "Change review requires named-user authorization.")
		return
	}
	if r.URL.RawPath != "" || r.URL.ForceQuery || !strings.HasPrefix(r.URL.Path, changeReviewPrefix) {
		fail(400, "Use the canonical change review route.")
		return
	}
	var input changeReviewInput
	if r.Method == http.MethodGet {
		for key, values := range r.URL.Query() {
			if (key != "organization" && key != "offset") || len(values) != 1 {
				fail(400, "Invalid review query.")
				return
			}
		}
		input.Organization = r.URL.Query().Get("organization")
	} else if r.Method == http.MethodPost {
		if r.URL.RawQuery != "" {
			fail(400, "Review mutations do not accept URL parameters.")
			return
		}
		if !decodePolicyRequest(w, r, 8192, &input) {
			return
		}
	} else {
		fail(405, "Change review supports GET and POST only.")
		return
	}
	if !validGatewayIdentifier(input.Organization) || input.Organization == "*" {
		fail(400, "An explicit organization is required.")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, changeReviewPrefix)
	permission := "change.read"
	if r.Method == http.MethodPost {
		switch {
		case path == "requests":
			permission = "change.submit"
		case strings.HasSuffix(path, "/approve") || strings.HasSuffix(path, "/reject") || strings.HasSuffix(path, "/apply"):
			permission = "change.review"
		case strings.HasSuffix(path, "/cancel"):
			permission = "change.submit"
		default:
			fail(404, "Unknown change review operation.")
			return
		}
	}
	identity, err := a.auth.Authorize(r, input.Organization, permission)
	if err != nil {
		fail(403, "An authorized organization-scoped session and mutation CSRF proof are required.")
		return
	}
	if r.Method == http.MethodGet && path == "capabilities" {
		writeJSON(w, 200, map[string]any{"allow_self_approval": a.allowSelf, "application_enabled": false,
			"message": "Approval records a reviewed revision only. Production application contracts are not configured."})
		return
	}
	if path == "requests" {
		if r.Method == http.MethodGet {
			for _, domain := range []string{"policy", "gateway"} {
				if _, err := a.auth.Authorize(r, input.Organization, domain+".read"); err != nil {
					fail(403, "Listing the shared queue requires policy and gateway read permissions.")
					return
				}
			}
			offset := 0
			if raw := r.URL.Query().Get("offset"); raw != "" {
				offset, err = strconv.Atoi(raw)
				if err != nil {
					fail(400, "Invalid page offset.")
					return
				}
			}
			records, err := a.store.List(r.Context(), input.Organization, offset)
			if err != nil {
				a.storeError(w, err)
				return
			}
			next := -1
			if len(records) > 50 {
				records = records[:50]
				next = offset + 50
			}
			writeJSON(w, 200, map[string]any{"requests": records, "next_offset": next})
			return
		}
		if input.Kind != "policy" && input.Kind != "gateway" {
			fail(400, "Only saved policy and gateway revisions are supported.")
			return
		}
		if _, err := a.auth.Authorize(r, input.Organization, input.Kind+".edit"); err != nil {
			fail(403, "Submitting this change requires its domain edit permission.")
			return
		}
		proposal, err := a.capture(r.Context(), input.Organization, input.Kind, input.Target, input.Revision)
		if err != nil {
			fail(409, err.Error())
			return
		}
		proposal.Reason = input.Reason
		proposal.Author = reviewPrincipal(identity)
		record, err := a.store.Submit(r.Context(), proposal)
		if err != nil {
			a.storeError(w, err)
			return
		}
		writeJSON(w, 201, record)
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "requests" || !reviewIdentifier.MatchString(parts[1]) {
		fail(404, "Unknown change request.")
		return
	}
	record, err := a.store.Get(r.Context(), input.Organization, parts[1])
	if err != nil {
		a.storeError(w, err)
		return
	}
	if _, err := a.auth.Authorize(r, input.Organization, record.Proposal.Kind+".read"); err != nil {
		fail(403, "Reading this change requires its domain read permission.")
		return
	}
	if r.Method == http.MethodGet && len(parts) == 2 {
		writeJSON(w, 200, record)
		return
	}
	if r.Method != http.MethodPost || len(parts) != 3 {
		fail(405, "Unsupported change operation.")
		return
	}
	if parts[2] == "apply" {
		fail(409, "Application is disabled: no reviewed production activation contract is configured. Approval is not deployment.")
		return
	}
	if parts[2] == "approve" {
		current, err := a.capture(r.Context(), input.Organization, record.Proposal.Kind, record.Proposal.Target, record.Proposal.Revision)
		if err != nil || current.Validation != record.Proposal.Validation || current.After != record.Proposal.After || current.Before != record.Proposal.Before ||
			current.Impact != record.Proposal.Impact || current.BaseRevision != record.Proposal.BaseRevision {
			fail(409, "Source or validation context changed. Submit a newly validated change; this request was not approved.")
			return
		}
	}
	record, err = a.store.Decide(r.Context(), input.Organization, record.ID, input.Digest, input.ExpectedVersion, parts[2],
		input.Reason, reviewPrincipal(identity), a.allowSelf)
	if err != nil {
		a.storeError(w, err)
		return
	}
	writeJSON(w, 200, record)
}
func (a *changeReviewAPI) storeError(w http.ResponseWriter, err error) {
	status := 500
	message := "Unable to persist or read change review."
	switch {
	case errors.Is(err, changereview.ErrInvalid):
		status = 400
		message = err.Error()
	case errors.Is(err, changereview.ErrNotFound):
		status = 404
		message = err.Error()
	case errors.Is(err, changereview.ErrConflict) || errors.Is(err, changereview.ErrSelfReview):
		status = 409
		message = err.Error()
	}
	log.Printf("Change review storage/decision failed (HTTP %d): %T", status, err)
	writePolicyError(w, status, message)
}

// The reader uses the existing production mTLS proxy, never browser content or Simulator.
func changeReviewDomainCapture(proxy http.Handler) changeReviewCapture {
	return func(ctx context.Context, org, kind, target string, revision int) (changereview.Proposal, error) {
		p := changereview.Proposal{Organization: org, Kind: kind, Target: target, Revision: revision}
		if !validGatewayIdentifier(org) || !reviewIdentifier.MatchString(target) || revision <= 0 {
			return p, errors.New("Invalid source revision identity.")
		}
		call := func(method, path string, body any, out any) error {
			var data []byte
			var err error
			if body != nil {
				data, err = json.Marshal(body)
				if err != nil {
					return err
				}
			}
			ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			request, err := http.NewRequestWithContext(ctx, method, "http://control-room.internal"+path, bytes.NewReader(data))
			if err != nil {
				return err
			}
			request.Header.Set("Content-Type", "application/json")
			response := &reviewResponse{header: make(http.Header)}
			proxy.ServeHTTP(response, request)
			if response.overflow || response.status != 200 {
				return errors.New("Production source or validation service is unavailable; no review decision was recorded.")
			}
			if err = json.Unmarshal(response.body.Bytes(), out); err != nil {
				return errors.New("Invalid production source response.")
			}
			return nil
		}
		switch kind {
		case "gateway":
			var contracts gatewayContractsResponse
			if err := call("GET", "/api/gateways/contracts", nil, &contracts); err != nil {
				return p, err
			}
			if contracts.OrganizationID != org {
				return p, errors.New("Gateway organization changed.")
			}
			var contract *gatewayInstanceContract
			for i := range contracts.Contracts {
				if contracts.Contracts[i].InstanceID == target {
					contract = &contracts.Contracts[i]
					break
				}
			}
			if contract == nil || contract.OrganizationID != org {
				return p, errors.New("Gateway is not installed in this organization.")
			}
			var record gatewayConfigRecord
			if err := call("GET", "/api/gateways/configs/"+url.PathEscape(target), nil, &record); err != nil {
				return p, err
			}
			if record.OrganizationID != org || record.InstanceID != target || record.CurrentRevision != revision {
				return p, errors.New("Gateway draft revision changed; reload before submitting.")
			}
			for _, r := range record.Revisions {
				if r.Revision == revision {
					p.After = string(r.Config)
				}
				if r.Revision == revision-1 {
					p.Before = string(r.Config)
					p.BaseRevision = r.Revision
				}
			}
			if p.After == "" || revision > 1 && p.BaseRevision != revision-1 {
				return p, errors.New("Gateway revision history is incomplete.")
			}
			if _, err := parseGatewayInstanceConfig([]byte(p.After), *contract); err != nil {
				return p, errors.New("Gateway revision does not pass current validation.")
			}
			identity, _ := json.Marshal(contract)
			p.Validation = "Saved gateway revision passes the current installed-instance contract."
			p.Impact = "Draft review only; no runtime activation. Installed contract: " + string(identity)
		case "policy":
			var status policyStatus
			if err := call("GET", "/api/policy", nil, &status); err != nil {
				return p, err
			}
			if status.OrganizationID != org || !status.Configured {
				return p, errors.New("Policy organization context is unavailable or changed.")
			}
			var record policyRecord
			if err := call("GET", "/api/policy/records/"+target, nil, &record); err != nil {
				return p, err
			}
			if record.ID != target || record.CurrentRevision != revision || record.Kind != "policy" || record.Archived {
				return p, errors.New("Policy revision changed or is not a current policy draft.")
			}
			p.After = record.Content
			if revision > 1 {
				var previous policyRevision
				if err := call("GET", "/api/policy/records/"+target+"/revisions/"+strconv.Itoa(revision-1), nil, &previous); err != nil {
					return p, err
				}
				if previous.RecordID != target || previous.Number != revision-1 {
					return p, errors.New("Policy base revision is inconsistent.")
				}
				p.Before = previous.Content
				p.BaseRevision = previous.Number
			}
			var checked policyCheckResponse
			if err := call("POST", "/api/policy/check", policySourceRequest{Source: p.After}, &checked); err != nil {
				return p, err
			}
			if !checked.Valid {
				return p, errors.New("Policy revision does not pass validation.")
			}
			// Re-read after validation to reject a concurrent edit.
			var latest policyRecord
			if err := call("GET", "/api/policy/records/"+target, nil, &latest); err != nil {
				return p, err
			}
			if latest.ID != target || latest.Kind != "policy" || latest.Name != record.Name ||
				latest.CurrentRevision != revision || latest.Content != p.After || latest.Archived {
				return p, errors.New("Policy changed during validation.")
			}
			p.Validation = "Saved policy revision passes current compiler validation."
			context, _ := json.Marshal(struct {
				Organization string
				Name         string
				Config       bool
				Warnings     []lintDiagnostic
			}{
				status.OrganizationID, record.Name, status.Configured, checked.Warnings})
			p.Impact = "Draft review only; runtime base revision and application contract are unavailable. Context: " + string(context)
		default:
			return p, errors.New("Unsupported review domain.")
		}
		return p, nil
	}
}

type reviewResponse struct {
	header   http.Header
	body     bytes.Buffer
	status   int
	overflow bool
}

func (w *reviewResponse) Header() http.Header { return w.header }
func (w *reviewResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *reviewResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if w.body.Len()+len(data) > 512<<10 {
		w.overflow = true
		return 0, io.ErrShortBuffer
	}
	return w.body.Write(data)
}
