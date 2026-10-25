package operatordelegation

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

const (
	SubjectLinkPublicationPrefix  = "/api/subject-link-publications/"
	subjectLinkPublicationPurpose = "subject-link-publication-v1"
)

// SubjectLinkPublicationRoute is the single route grammar shared by Room
// authorization, Room proof signing and Control-Service verification. Review
// and registry permissions never map to these routes.
type SubjectLinkPublicationRoute struct {
	Recipient, Epoch, Action, Sequence, Permission string
	// Idempotent routes require exactly one Idempotency-Key; others forbid it.
	Idempotent bool
}

func publicationSequence(value string, allowZero bool) bool {
	n, err := strconv.ParseUint(value, 10, 64)
	return err == nil && (n > 0 || allowZero) && strconv.FormatUint(n, 10) == value
}

// ParseSubjectLinkPublicationRoute classifies only the documented exact routes.
func ParseSubjectLinkPublicationRoute(method, path string) (SubjectLinkPublicationRoute, bool) {
	rest, ok := strings.CutPrefix(path, SubjectLinkPublicationPrefix)
	if !ok {
		return SubjectLinkPublicationRoute{}, false
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || !reviewProofID.MatchString(parts[0]) || !reviewProofID.MatchString(parts[1]) {
		return SubjectLinkPublicationRoute{}, false
	}
	route := SubjectLinkPublicationRoute{Recipient: parts[0], Epoch: parts[1]}
	switch {
	case method == http.MethodGet && len(parts) == 2:
		route.Action, route.Permission = "state", "subject-link.read"
	case method == http.MethodGet && len(parts) == 4 && parts[2] == "events" && publicationSequence(parts[3], true):
		route.Action, route.Sequence, route.Permission = "event", parts[3], "subject-link.read"
	case method == http.MethodGet && len(parts) == 4 && parts[2] == "candidates" && publicationSequence(parts[3], false):
		route.Action, route.Sequence, route.Permission = "candidate", parts[3], "subject-link.read"
	case method == http.MethodGet && len(parts) == 4 && parts[2] == "signed" && publicationSequence(parts[3], false):
		route.Action, route.Sequence, route.Permission = "signed", parts[3], "subject-link.read"
	case method == http.MethodPost && len(parts) == 3 && parts[2] == "provision":
		route.Action, route.Permission = "provision", "subject-link.publish"
	case method == http.MethodPost && len(parts) == 3 && parts[2] == "intents":
		route.Action, route.Permission, route.Idempotent = "intent", "subject-link.publish", true
	case method == http.MethodPost && len(parts) == 3 && parts[2] == "candidates":
		route.Action, route.Permission, route.Idempotent = "capture", "subject-link.snapshot.capture", true
	case method == http.MethodPost && len(parts) == 5 && parts[2] == "candidates" && publicationSequence(parts[3], false) && parts[4] == "sign":
		route.Action, route.Sequence, route.Permission, route.Idempotent = "sign", parts[3], "subject-link.snapshot.sign", true
	default:
		return SubjectLinkPublicationRoute{}, false
	}
	return route, true
}

func subjectLinkPublicationScope(r *http.Request, body []byte, organization string) (reviewProofScope, error) {
	denied := errors.New("invalid subject-link publication proof request")
	if !reviewProofID.MatchString(organization) || len(body) > MaxBody || r.URL.RawPath != "" ||
		r.URL.ForceQuery || len(r.URL.RequestURI()) > 2048 {
		return reviewProofScope{}, denied
	}
	route, ok := ParseSubjectLinkPublicationRoute(r.Method, r.URL.Path)
	if !ok {
		return reviewProofScope{}, denied
	}
	scope := reviewProofScope{purpose: subjectLinkPublicationPurpose, organization: organization, permission: route.Permission}
	if r.Method == http.MethodGet {
		if len(body) != 0 || r.URL.RawQuery != "organization_id="+url.QueryEscape(organization) {
			return reviewProofScope{}, denied
		}
	} else if r.URL.RawQuery != "" {
		return reviewProofScope{}, denied
	}
	keys := r.Header.Values("Idempotency-Key")
	if route.Idempotent {
		if len(keys) != 1 || !reviewProofID.MatchString(keys[0]) {
			return reviewProofScope{}, denied
		}
		scope.retryKey = keys[0]
	} else if len(keys) != 0 {
		return reviewProofScope{}, denied
	}
	return scope, nil
}

// SignSubjectLinkPublication uses an isolated purpose: review, registry,
// enrollment and active-binding proofs cannot publish, capture or sign.
// The caller must first authorize the current session, organization and CSRF.
func (s *Signer) SignSubjectLinkPublication(r *http.Request, body []byte, id operatorauth.Identity, organization string, now time.Time) (string, error) {
	scope, err := subjectLinkPublicationScope(r, body, organization)
	if err != nil {
		return "", err
	}
	return s.signSubjectLinkRequest(r, body, id, now, scope)
}

// VerifySubjectLinkPublication authenticates the mTLS caller and exact request,
// checks independent backend grants, then consumes the proof nonce.
func (v *Verifier) VerifySubjectLinkPublication(r *http.Request, body []byte, organization string, now time.Time) (operatorauth.Grant, error) {
	scope, err := subjectLinkPublicationScope(r, body, organization)
	if err != nil {
		return operatorauth.Grant{}, err
	}
	return v.verify(r, body, now, scope)
}
