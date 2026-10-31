package enrollment

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
)

var errSelfApproval = errors.New("self-approval permission required")

func (api *adminAPI) getBatchApproval(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) != 1 || len(values["organization"]) != 1 ||
		values.Get("organization") == "" || r.URL.RawPath != "" || r.URL.ForceQuery {
		apiError(w, http.StatusBadRequest, "Batch receipt requires exactly one organization query parameter.")
		return
	}
	organization := values.Get("organization")
	if _, ok := api.authorize(w, r, organization, "read"); !ok {
		return
	}
	receipt, err := api.store.GetBatchApproval(r.Context(), organization, r.PathValue("digest"))
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, receipt)
}

func (api *adminAPI) reviewBatch(w http.ResponseWriter, r *http.Request) {
	var request BatchReviewRequest
	if !readBody(w, r, &request) {
		return
	}
	if _, ok := api.authorize(w, r, request.Organization, "read"); !ok {
		return
	}
	inventory, err := api.store.ReviewBatch(r.Context(), request, api.now())
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, inventory)
}

func (api *adminAPI) approveBatch(w http.ResponseWriter, r *http.Request) {
	var request BatchApprovalRequest
	if !readBody(w, r, &request) {
		return
	}
	principal, ok := api.authorize(w, r, request.Organization, "approve")
	if !ok {
		return
	}
	inventory, err := api.store.approveBatch(r.Context(), request, principal.Name,
		api.config.Organizations[request.Organization], api.now(), slices.Contains(principal.Permissions, "approval.self"))
	if errors.Is(err, errSelfApproval) {
		log.Print("Enrollment batch self-approval denied: explicit named-user permission is required")
		apiError(w, http.StatusForbidden, "This inventory includes your own invitations. Approval requires approval.self permission; otherwise another authorized reviewer must approve.")
		return
	}
	if err != nil {
		storeError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, inventory)
}
