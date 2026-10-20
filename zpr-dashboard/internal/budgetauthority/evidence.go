package budgetauthority

import (
	"context"
	"errors"
	"strings"
	"time"
)

const maxEvidenceFreshness = 5 * time.Second

// ErrUnavailable indicates that no authoritative budget evidence source is configured.
var ErrUnavailable = errors.New("budget authority is unavailable")

// Scope identifies the exact scenario plan covered by budget evidence.
type Scope struct {
	OrganizationID string
	ScenarioID     string
	PlanDigest     string
}

// Decision is the source's decision for a budget approval.
type Decision string

// DecisionApproved is the only decision accepted by ValidateCurrent.
const DecisionApproved Decision = "approved"

// Evidence is a source-backed budget decision and its currentness metadata.
type Evidence struct {
	ApprovalID     string
	OrganizationID string
	ScenarioID     string
	PlanDigest     string
	Decision       Decision
	ApprovedBy     string
	Version        string
	CheckedAt      time.Time
	ValidFrom      time.Time
	ExpiresAt      time.Time
	Revoked        bool
}

// Resolver retrieves the authoritative current budget decision for a plan scope.
type Resolver interface {
	ResolveCurrent(context.Context, Scope) (Evidence, error)
}

// UnavailableResolver denies resolution when no authoritative source is configured.
type UnavailableResolver struct{}

func (UnavailableResolver) ResolveCurrent(context.Context, Scope) (Evidence, error) {
	return Evidence{}, ErrUnavailable
}

// ValidateCurrent accepts only approved, scope-matched, unrevoked evidence
// whose validity interval includes now and whose source check is at most five
// seconds old.
func ValidateCurrent(evidence Evidence, scope Scope, now time.Time) error {
	if strings.TrimSpace(evidence.ApprovalID) == "" ||
		strings.TrimSpace(evidence.ApprovedBy) == "" ||
		strings.TrimSpace(evidence.Version) == "" ||
		strings.TrimSpace(scope.OrganizationID) == "" ||
		strings.TrimSpace(scope.ScenarioID) == "" ||
		strings.TrimSpace(scope.PlanDigest) == "" {
		return errors.New("budget evidence identity and scope are required")
	}
	if evidence.Decision != DecisionApproved {
		return errors.New("budget evidence is not approved")
	}
	if evidence.OrganizationID != scope.OrganizationID ||
		evidence.ScenarioID != scope.ScenarioID ||
		evidence.PlanDigest != scope.PlanDigest {
		return errors.New("budget evidence scope does not match")
	}
	if evidence.Revoked {
		return errors.New("budget evidence is revoked")
	}
	if now.IsZero() || evidence.CheckedAt.IsZero() ||
		evidence.ValidFrom.IsZero() || evidence.ExpiresAt.IsZero() {
		return errors.New("budget evidence timestamps are required")
	}
	if now.Before(evidence.ValidFrom) || !now.Before(evidence.ExpiresAt) ||
		evidence.ValidFrom.After(evidence.ExpiresAt) {
		return errors.New("budget evidence is outside its validity period")
	}
	if evidence.CheckedAt.After(now) ||
		evidence.CheckedAt.Before(evidence.ValidFrom) ||
		!evidence.CheckedAt.Before(evidence.ExpiresAt) ||
		now.Sub(evidence.CheckedAt) > maxEvidenceFreshness {
		return errors.New("budget evidence currentness check is stale or invalid")
	}
	return nil
}
