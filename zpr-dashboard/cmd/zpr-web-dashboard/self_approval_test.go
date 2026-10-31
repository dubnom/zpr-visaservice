package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func TestChangeReviewOwnApprovalRequiresAdditionalPermission(t *testing.T) {
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	for _, mode := range []string{"self-denied", "self-allowed", "independent"} {
		t.Run(mode, func(t *testing.T) {
			store, err := changereview.Open(filepath.Join(t.TempDir(), "private", "review.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			proposal := changereview.Proposal{Organization: "great-lakes", Kind: "policy", Target: "policy", Revision: 1,
				After: "source", Validation: "Validated", Impact: "Draft only", Reason: "Change",
				Author: changereview.Principal{Issuer: "https://idp.example", Subject: "author"}}
			record, err := store.Submit(t.Context(), proposal)
			if err != nil {
				t.Fatal(err)
			}
			auth := &reviewAuthFixture{subject: "author", denied: "approval.self"}
			if mode == "self-allowed" {
				auth.denied = ""
			} else if mode == "independent" {
				auth.subject = "reviewer"
			}
			captures := 0
			api := &changeReviewAPI{store: store, auth: auth, capture: func(context.Context, string, string, string, int) (changereview.Proposal, error) {
				captures++
				return proposal, nil
			}}
			body, err := json.Marshal(changeReviewInput{Organization: proposal.Organization,
				Digest: record.Digest, ExpectedVersion: 1, Reason: "Reviewed exact request"})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest("POST", changeReviewPrefix+"requests/"+record.ID+"/approve", strings.NewReader(string(body)))
			request.Header.Set("X-ZPR-CSRF", "proof")
			response := httptest.NewRecorder()
			api.ServeHTTP(response, request)
			got, err := store.Get(t.Context(), proposal.Organization, record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "self-denied" {
				if response.Code != 403 || got.State != "submitted" || got.Version != 1 || len(got.Events) != 1 || captures != 0 {
					t.Fatalf("denied self-approval mutated request: %d %+v", response.Code, got)
				}
			} else if response.Code != 200 || got.State != "approved" || got.SelfReviewed() != (mode == "self-allowed") {
				t.Fatalf("approval failed or audit identity changed: %d %+v", response.Code, got)
			}
		})
	}
}

func TestSubjectReviewSelfApprovalRequiresBothGrants(t *testing.T) {
	for _, mode := range []string{"neither", "session-only", "backend-only", "both"} {
		t.Run(mode, func(t *testing.T) {
			f := subjectReviewHTTP(t)
			if mode == "backend-only" || mode == "both" {
				f.trust.Grants[0].Permissions = append(f.trust.Grants[0].Permissions, "approval.self")
				f.verifyWithLeaf(t, f.leaf)
			}
			record := subjectReviewRecord(t, subjectReviewResponse(t, f.api,
				f.request(t, "POST", subjectReviewPrefix+"proposals", "submit", "author", subjectReviewJSON(t, f.input)), 200))
			body := subjectReviewJSON(t, subjectReviewApproval{"production", record.Digest, "1", f.input.RegistryRevision, "Explicit review"})
			request := f.request(t, "POST", subjectReviewPrefix+record.ID+"/approve", "decision", "author", body)
			identity := operatorauth.Identity{Issuer: record.Proposal.Author.Issuer, Subject: record.Proposal.Author.Subject}
			if mode == "session-only" || mode == "both" {
				identity.Permissions = []string{"approval.self"}
			}
			proof, err := f.signer.SignSubjectLinkReview(request, body, identity, "production", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set(operatordelegation.Header, proof)
			want := 403
			if mode == "both" {
				want = 200
			}
			subjectReviewResponse(t, f.api, request, want)
			got, err := f.api.store.GetSubjectLink(t.Context(), "production", record.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "both" && (got.State != "submitted" || got.Version != record.Version || len(got.Events) != len(record.Events)) {
				t.Fatal("denied self-approval changed review", got)
			}
			if mode == "both" && (!got.SelfReviewed() || got.State != "review-approved") {
				t.Fatal("authorized self-approval lost audit marker", got)
			}
		})
	}
}
