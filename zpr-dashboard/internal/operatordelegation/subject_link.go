package operatordelegation

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

const subjectLinkReviewPurpose = "subject-link-review-v1"

var reviewProofID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type reviewProofScope struct {
	purpose, organization, permission, retryKey string
}

func subjectLinkReviewScope(r *http.Request, body []byte, organization string) (reviewProofScope, error) {
	denied := errors.New("invalid subject-link review proof request")
	if !reviewProofID.MatchString(organization) || len(body) > MaxBody || r.URL.RawPath != "" ||
		r.URL.ForceQuery || len(r.URL.RequestURI()) > 2048 {
		return reviewProofScope{}, denied
	}
	scope := reviewProofScope{purpose: subjectLinkReviewPurpose, organization: organization}
	const prefix = "/api/subject-link-reviews/"
	rest, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		return reviewProofScope{}, denied
	}
	switch r.Method {
	case http.MethodGet:
		if !reviewProofID.MatchString(rest) || len(body) != 0 || len(r.Header.Values("Idempotency-Key")) != 0 ||
			r.URL.RawQuery != "organization_id="+url.QueryEscape(organization) {
			return reviewProofScope{}, denied
		}
		scope.permission = "subject-link.read"
	case http.MethodPost:
		if r.URL.RawQuery != "" {
			return reviewProofScope{}, denied
		}
		if rest == "proposals" {
			scope.permission = "subject-link.propose"
		} else if id, ok := strings.CutSuffix(rest, "/approve"); ok && reviewProofID.MatchString(id) {
			scope.permission = "subject-link.approve"
		} else {
			return reviewProofScope{}, denied
		}
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) != 1 || !reviewProofID.MatchString(keys[0]) {
			return reviewProofScope{}, denied
		}
		scope.retryKey = keys[0]
	default:
		return reviewProofScope{}, denied
	}
	return scope, nil
}

// SignSubjectLinkReview is distinct from enrollment and active-binding proofs.
// The caller must first authorize the current session, organization and CSRF.
func (s *Signer) SignSubjectLinkReview(r *http.Request, body []byte, id operatorauth.Identity, organization string, now time.Time) (string, error) {
	scope, err := subjectLinkReviewScope(r, body, organization)
	if err != nil {
		return "", err
	}
	return s.signSubjectLinkRequest(r, body, id, now, scope)
}

func (s *Signer) signSubjectLinkRequest(r *http.Request, body []byte, id operatorauth.Identity, now time.Time, scope reviewProofScope) (string, error) {
	if !https(id.Issuer, false) || !text(id.Subject) || len(AuditIdentity(id.Issuer, id.Subject)) > 256 ||
		now.UnixNano() <= 0 || !time.Unix(0, now.UnixNano()).Equal(now) ||
		now.Add(30*time.Second).UnixNano() <= now.UnixNano() {
		return "", errors.New("invalid subject-link review principal or clock")
	}
	return s.sign(r, body, id, now, scope)
}

func subjectLinkRegistryScope(r *http.Request, body []byte, organization string) (reviewProofScope, error) {
	denied := errors.New("invalid subject-link registry proof request")
	if !reviewProofID.MatchString(organization) || len(body) > MaxBody || r.URL.RawPath != "" ||
		r.URL.ForceQuery || len(r.URL.RequestURI()) > 2048 || len(r.Header.Values("Idempotency-Key")) != 0 {
		return reviewProofScope{}, denied
	}
	rest, ok := strings.CutPrefix(r.URL.Path, "/api/subject-link-registry/")
	if !ok {
		return reviewProofScope{}, denied
	}
	scope := reviewProofScope{purpose: "subject-link-registry-v1", organization: organization}
	parts := strings.Split(rest, "/")
	registration := len(parts) >= 2 && (parts[0] == "authorities" || parts[0] == "sources") && reviewProofID.MatchString(parts[1])
	switch {
	case r.Method == http.MethodPost && rest == "context" && r.URL.RawQuery == "":
		scope.permission = "subject-link.read"
	case r.Method == http.MethodPut && registration && len(parts) == 2 && r.URL.RawQuery == "":
		scope.permission = "subject-link.registry.manage"
	case r.Method == http.MethodGet && registration &&
		(len(parts) == 2 || (len(parts) == 4 && parts[2] == "versions" && registryProofVersion(parts[3]))) &&
		len(body) == 0 && r.URL.RawQuery == "organization_id="+url.QueryEscape(organization):
		scope.permission = "subject-link.read"
	default:
		return reviewProofScope{}, denied
	}
	return scope, nil
}

func registryProofVersion(value string) bool {
	version, err := strconv.ParseUint(value, 10, 64)
	return err == nil && version > 0 && strconv.FormatUint(version, 10) == value
}

// Registry proofs reuse the existing exact-request envelope, with an isolated
// purpose and permission. They cannot approve reviews or publish active links.
func (s *Signer) SignSubjectLinkRegistry(r *http.Request, body []byte, id operatorauth.Identity, organization string, now time.Time) (string, error) {
	scope, err := subjectLinkRegistryScope(r, body, organization)
	if err != nil {
		return "", err
	}
	return s.signSubjectLinkRequest(r, body, id, now, scope)
}

func (v *Verifier) VerifySubjectLinkRegistry(r *http.Request, body []byte, organization string, now time.Time) (operatorauth.Grant, error) {
	scope, err := subjectLinkRegistryScope(r, body, organization)
	if err != nil {
		return operatorauth.Grant{}, err
	}
	return v.verify(r, body, now, scope)
}

// VerifySubjectLinkReview authenticates the mTLS caller and exact request,
// checks independent backend grants, then consumes the proof nonce.
// Organization must come from the handler's strictly decoded request context.
// This does not validate proposal content, authorize publication or read receipts.
func (v *Verifier) VerifySubjectLinkReview(r *http.Request, body []byte, organization string, now time.Time) (operatorauth.Grant, error) {
	scope, err := subjectLinkReviewScope(r, body, organization)
	if err != nil {
		return operatorauth.Grant{}, err
	}
	return v.verify(r, body, now, scope)
}
