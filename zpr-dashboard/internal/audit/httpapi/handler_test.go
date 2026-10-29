package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neboagency.com/zpr-dashborad/internal/audit"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

type testRepository struct {
	query     audit.Query
	page      audit.Page
	err       error
	appended  []audit.Event
	appendErr error
}

type testAuditVerifier struct {
	grant operatorauth.Grant
	err   error
}

func (verifier testAuditVerifier) VerifyAuditRead(_ *http.Request, body []byte, _ time.Time) (operatorauth.Grant, error) {
	if len(body) != 0 {
		return operatorauth.Grant{}, errors.New("body not allowed")
	}
	return verifier.grant, verifier.err
}

func (repository *testRepository) Append(_ context.Context, event audit.Event) (audit.Event, error) {
	if repository.appendErr != nil {
		return audit.Event{}, repository.appendErr
	}
	repository.appended = append(repository.appended, event)
	return event, nil
}

func (repository *testRepository) List(_ context.Context, query audit.Query) (audit.Page, error) {
	repository.query = query
	return repository.page, repository.err
}

func TestHandlerUsesTrustedOrganizationScopeAndReturnsPage(t *testing.T) {
	repository := &testRepository{page: audit.Page{Events: []audit.Event{{
		ID: "event-1", OrganizationID: "northstar", Action: "policy.read",
	}}}}
	handler, err := NewHandler(repository, func(*http.Request) (RequestScope, error) {
		return RequestScope{OrganizationID: "northstar"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet,
		"/api/audit/events?from=2026-10-28T12%3A00%3A00Z&until=2026-10-28T13%3A00%3A00Z&source=control-room&action=policy.read&outcome=success&limit=25&cursor=next",
		nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("status=%d headers=%v body=%s", response.Code, response.Header(), response.Body)
	}
	if repository.query.OrganizationID != "northstar" || repository.query.Limit != 25 ||
		repository.query.Source != "control-room" || repository.query.Action != "policy.read" ||
		repository.query.Outcome != "success" || repository.query.Cursor != "next" ||
		!repository.query.From.Equal(time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)) ||
		!repository.query.Until.Equal(time.Date(2026, 10, 28, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected repository query: %+v", repository.query)
	}
	if got := response.Body.String(); got == "" || !strings.Contains(got, `"organization_id":"northstar"`) {
		t.Fatalf("unexpected page response: %s", got)
	}
}

func TestHandlerDoesNotAcceptCallerOrganization(t *testing.T) {
	repository := &testRepository{}
	handler, err := NewHandler(repository, func(*http.Request) (RequestScope, error) {
		return RequestScope{OrganizationID: "authorized-org"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/audit/events?organization_id=attacker-org", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("caller-controlled organization status = %d", response.Code)
	}
	if repository.query.OrganizationID != "" {
		t.Fatalf("repository called for rejected caller scope: %+v", repository.query)
	}
}

func TestHandlerFailsClosedOnScopeAndRepositoryErrors(t *testing.T) {
	tests := []struct {
		name       string
		resolve    ScopeResolver
		repository *testRepository
		status     int
	}{
		{
			name: "scope failure",
			resolve: func(*http.Request) (RequestScope, error) {
				return RequestScope{}, errors.New("not authorized")
			},
			repository: &testRepository{},
			status:     http.StatusForbidden,
		},
		{
			name: "empty scope",
			resolve: func(*http.Request) (RequestScope, error) {
				return RequestScope{}, nil
			},
			repository: &testRepository{},
			status:     http.StatusForbidden,
		},
		{
			name: "repository failure",
			resolve: func(*http.Request) (RequestScope, error) {
				return RequestScope{OrganizationID: "northstar"}, nil
			},
			repository: &testRepository{err: errors.New("database details")},
			status:     http.StatusServiceUnavailable,
		},
		{
			name: "audit append failure",
			resolve: func(*http.Request) (RequestScope, error) {
				return RequestScope{
					OrganizationID: "northstar", ActorIssuer: "https://idp.example", ActorSubject: "operator",
				}, nil
			},
			repository: &testRepository{appendErr: errors.New("append failed")},
			status:     http.StatusServiceUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, err := NewHandler(test.repository, test.resolve)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/audit/events", nil))
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if strings.Contains(response.Body.String(), "database details") {
				t.Fatal("repository error leaked to caller")
			}
		})
	}
}

func TestHandlerRejectsInvalidQueryAndMethod(t *testing.T) {
	repository := &testRepository{}
	handler, err := NewHandler(repository, func(*http.Request) (RequestScope, error) {
		return RequestScope{OrganizationID: "northstar"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, target string
		status         int
	}{
		{http.MethodGet, "/api/audit/events?limit=101", http.StatusBadRequest},
		{http.MethodGet, "/api/audit/events?from=nope", http.StatusBadRequest},
		{http.MethodGet, "/api/audit/events?source=a&source=b", http.StatusBadRequest},
		{http.MethodGet, "/api/audit/events?unexpected=value", http.StatusBadRequest},
		{http.MethodPost, "/api/audit/events", http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(test.method, test.target, nil))
		if response.Code != test.status {
			t.Errorf("%s %s status=%d body=%s", test.method, test.target, response.Code, response.Body)
		}
	}
	if repository.query.OrganizationID != "" {
		t.Fatalf("invalid requests reached repository: %+v", repository.query)
	}
}

func TestNewHandlerRequiresDependencies(t *testing.T) {
	if _, err := NewHandler(nil, func(*http.Request) (RequestScope, error) {
		return RequestScope{OrganizationID: "org"}, nil
	}); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("nil repository error = %v", err)
	}
	if _, err := NewHandler(&testRepository{}, nil); !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("nil scope resolver error = %v", err)
	}
}

func TestDelegatedHandlerUsesVerifiedOrganizationAndPermission(t *testing.T) {
	repository := &testRepository{}
	handler, err := NewDelegatedHandler(repository, testAuditVerifier{grant: operatorauth.Grant{
		Issuer: "https://idp.example", Subject: "operator",
		Organizations: []string{"northstar"}, Permissions: []string{"audit.read"},
	}}, func() time.Time { return time.Unix(1, 0) })
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/operator-enrollment/v1/audit/events?organization_id=northstar", nil))
	if response.Code != http.StatusOK || repository.query.OrganizationID != "northstar" {
		t.Fatalf("status=%d scope=%q body=%s", response.Code, repository.query.OrganizationID, response.Body)
	}
	if len(repository.appended) != 1 {
		t.Fatalf("successful request audit events=%d", len(repository.appended))
	}
	event := repository.appended[0]
	if event.ActorIssuer != "https://idp.example" || event.ActorSubject != "operator" ||
		event.OrganizationID != "northstar" || event.Action != "audit.history.read" ||
		event.Outcome != "requested" || event.CorrelationID == "" {
		t.Fatalf("incomplete audit read event: %+v", event)
	}
}

func TestDelegatedHandlerRejectsUnverifiedScopeAndBodies(t *testing.T) {
	for _, test := range []struct {
		name     string
		verifier testAuditVerifier
		path     string
		body     string
		status   int
	}{
		{
			name:     "invalid proof",
			verifier: testAuditVerifier{err: errors.New("bad proof")},
			path:     "/api/operator-enrollment/v1/audit/events?organization_id=northstar",
			status:   http.StatusForbidden,
		},
		{
			name: "foreign organization",
			verifier: testAuditVerifier{grant: operatorauth.Grant{
				Organizations: []string{"other"}, Permissions: []string{"audit.read"},
			}},
			path:   "/api/operator-enrollment/v1/audit/events?organization_id=northstar",
			status: http.StatusForbidden,
		},
		{
			name: "missing permission",
			verifier: testAuditVerifier{grant: operatorauth.Grant{
				Organizations: []string{"northstar"}, Permissions: []string{"policy.read"},
			}},
			path:   "/api/operator-enrollment/v1/audit/events?organization_id=northstar",
			status: http.StatusForbidden,
		},
		{
			name: "body",
			verifier: testAuditVerifier{grant: operatorauth.Grant{
				Organizations: []string{"northstar"}, Permissions: []string{"audit.read"},
			}},
			path:   "/api/operator-enrollment/v1/audit/events?organization_id=northstar",
			body:   "unexpected",
			status: http.StatusBadRequest,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &testRepository{}
			handler, err := NewDelegatedHandler(repository, test.verifier, time.Now)
			if err != nil {
				t.Fatal(err)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, strings.NewReader(test.body)))
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body)
			}
			if repository.query.OrganizationID != "" {
				t.Fatalf("invalid request reached repository: %+v", repository.query)
			}
		})
	}
}

func TestDelegatedHandlerDoesNotReturnDataWhenReadEventCannotBeRecorded(t *testing.T) {
	repository := &testRepository{
		appendErr: errors.New("audit write unavailable"),
		page: audit.Page{Events: []audit.Event{{
			ID: "must-not-return", OrganizationID: "northstar", Action: "sensitive.read",
		}}},
	}
	handler, err := NewDelegatedHandler(repository, testAuditVerifier{grant: operatorauth.Grant{
		Issuer: "https://idp.example", Subject: "operator",
		Organizations: []string{"northstar"}, Permissions: []string{"audit.read"},
	}}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/operator-enrollment/v1/audit/events?organization_id=northstar", nil))
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "must-not-return") {
		t.Fatalf("audit write failure status=%d body=%s", response.Code, response.Body)
	}
	if repository.query.OrganizationID != "" {
		t.Fatalf("repository query ran before read access was recorded: %+v", repository.query)
	}
}
