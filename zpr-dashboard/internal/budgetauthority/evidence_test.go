package budgetauthority

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestValidateCurrent(t *testing.T) {
	now := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	scope := Scope{
		OrganizationID: "org-1",
		ScenarioID:     "scenario-1",
		PlanDigest:     "sha256:plan",
	}
	valid := Evidence{
		ApprovalID:     "approval-1",
		OrganizationID: scope.OrganizationID,
		ScenarioID:     scope.ScenarioID,
		PlanDigest:     scope.PlanDigest,
		Decision:       DecisionApproved,
		ApprovedBy:     "operator-1",
		Version:        "version-1",
		CheckedAt:      now.Add(-time.Second),
		ValidFrom:      now.Add(-time.Minute),
		ExpiresAt:      now.Add(time.Minute),
	}

	tests := []struct {
		name    string
		mutate  func(*Evidence, *Scope, *time.Time)
		wantErr bool
	}{
		{name: "valid"},
		{
			name: "freshness boundary accepted",
			mutate: func(evidence *Evidence, _ *Scope, checkTime *time.Time) {
				evidence.CheckedAt = checkTime.Add(-maxEvidenceFreshness)
			},
		},
		{
			name: "unapproved decision",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.Decision = "pending"
			},
			wantErr: true,
		},
		{
			name: "missing approval reference",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.ApprovalID = " "
			},
			wantErr: true,
		},
		{
			name: "missing approver",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.ApprovedBy = ""
			},
			wantErr: true,
		},
		{
			name: "missing version",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.Version = ""
			},
			wantErr: true,
		},
		{
			name: "organization mismatch",
			mutate: func(_ *Evidence, requested *Scope, _ *time.Time) {
				requested.OrganizationID = "org-2"
			},
			wantErr: true,
		},
		{
			name: "scenario mismatch",
			mutate: func(_ *Evidence, requested *Scope, _ *time.Time) {
				requested.ScenarioID = "scenario-2"
			},
			wantErr: true,
		},
		{
			name: "plan digest mismatch",
			mutate: func(_ *Evidence, requested *Scope, _ *time.Time) {
				requested.PlanDigest = "sha256:other"
			},
			wantErr: true,
		},
		{
			name: "revoked",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.Revoked = true
			},
			wantErr: true,
		},
		{
			name: "not yet valid",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.ValidFrom = now.Add(time.Second)
			},
			wantErr: true,
		},
		{
			name: "expired",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.ExpiresAt = now
			},
			wantErr: true,
		},
		{
			name: "stale check",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.CheckedAt = now.Add(-maxEvidenceFreshness - time.Nanosecond)
			},
			wantErr: true,
		},
		{
			name: "future check",
			mutate: func(evidence *Evidence, _ *Scope, _ *time.Time) {
				evidence.CheckedAt = now.Add(time.Nanosecond)
			},
			wantErr: true,
		},
		{
			name: "zero validation time",
			mutate: func(_ *Evidence, _ *Scope, checkTime *time.Time) {
				*checkTime = time.Time{}
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			evidence := valid
			requested := scope
			checkTime := now
			if test.mutate != nil {
				test.mutate(&evidence, &requested, &checkTime)
			}
			err := ValidateCurrent(evidence, requested, checkTime)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateCurrent() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestUnavailableResolverDeniesByDefault(t *testing.T) {
	evidence, err := (UnavailableResolver{}).ResolveCurrent(
		context.Background(),
		Scope{OrganizationID: "org-1", ScenarioID: "scenario-1", PlanDigest: "sha256:plan"},
	)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ResolveCurrent() error = %v, want %v", err, ErrUnavailable)
	}
	if evidence != (Evidence{}) {
		t.Fatalf("ResolveCurrent() evidence = %+v, want zero value", evidence)
	}
}
