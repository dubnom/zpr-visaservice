package budgetauthority

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenRequiresExplicitDatabasePath(t *testing.T) {
	if _, err := Open(""); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Open(empty) error = %v, want unavailable", err)
	}
}

func TestStoreApprovalRevocationAndAuditPersist(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "budget-authority.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	scope := Scope{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"}
	approver := Actor{Issuer: "https://idp.example", Subject: "admin-a"}
	evidence, err := store.Approve(ctx, scope, DecisionApproved, approver, now.Add(MaxApprovalLifetime), now)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if evidence.ApprovalID == "" || evidence.Version != "1" ||
		evidence.ApprovedBy != `oidc:["https://idp.example","admin-a"]` ||
		evidence.Decision != DecisionApproved || evidence.Revoked {
		t.Fatalf("approval evidence = %+v", evidence)
	}
	fileInfo, err := os.Stat(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("database permissions = %o, want 600", fileInfo.Mode().Perm())
	}

	beforeResolve := time.Now().UTC()
	resolved, err := store.ResolveCurrent(ctx, scope)
	afterResolve := time.Now().UTC()
	if err != nil {
		t.Fatalf("ResolveCurrent() error = %v", err)
	}
	if resolved.ApprovalID != evidence.ApprovalID || resolved.Version != "1" ||
		resolved.CheckedAt.Before(beforeResolve) || resolved.CheckedAt.After(afterResolve) ||
		resolved.ValidFrom != evidence.ValidFrom || resolved.ExpiresAt != evidence.ExpiresAt {
		t.Fatalf("resolved evidence = %+v", resolved)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	persisted, err := store.ResolveCurrent(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveCurrent() after reopen: %v", err)
	}
	if persisted.ApprovalID != evidence.ApprovalID || persisted.Version != evidence.Version || persisted.Revoked {
		t.Fatalf("persisted evidence = %+v", persisted)
	}

	revoker := Actor{Issuer: "https://idp.example", Subject: "admin-b"}
	revoked, err := store.Revoke(ctx, evidence.ApprovalID, scope.OrganizationID, revoker, time.Now())
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if !revoked.Revoked || revoked.ApprovalID != evidence.ApprovalID {
		t.Fatalf("revocation evidence = %+v", revoked)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	persistedRevocation, err := store.ResolveCurrent(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveCurrent() revoked record after reopen: %v", err)
	}
	if !persistedRevocation.Revoked {
		t.Fatalf("revocation did not persist: %+v", persistedRevocation)
	}

	var auditEvents int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM budget_authority_audit WHERE approval_id=?`,
		evidence.ApprovalID).Scan(&auditEvents); err != nil {
		t.Fatal(err)
	}
	if auditEvents != 5 {
		t.Fatalf("audit event count = %d, want 5", auditEvents)
	}
	var auditedValidFrom, auditedExpiresAt int64
	if err := store.db.QueryRowContext(ctx, `SELECT valid_from_unix_nano,expires_at_unix_nano
FROM budget_authority_audit WHERE approval_id=? AND event='revoked'`,
		evidence.ApprovalID).Scan(&auditedValidFrom, &auditedExpiresAt); err != nil {
		t.Fatal(err)
	}
	if auditedValidFrom != evidence.ValidFrom.UnixNano() || auditedExpiresAt != evidence.ExpiresAt.UnixNano() {
		t.Fatalf("revocation audit validity = %d/%d, want %d/%d",
			auditedValidFrom, auditedExpiresAt, evidence.ValidFrom.UnixNano(), evidence.ExpiresAt.UnixNano())
	}
}

func TestSeparateStoreObservesCurrentRevocation(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "budget-authority.db")
	writer, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	reader, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	now := time.Now().UTC()
	scope := Scope{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"}
	approver := Actor{Issuer: "https://idp.example", Subject: "admin-a"}
	approval, err := writer.Approve(ctx, scope, DecisionApproved, approver, now.Add(time.Minute), now)
	if err != nil {
		t.Fatalf("Approve() error = %v", err)
	}

	current, err := reader.ResolveCurrent(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveCurrent() from separate store: %v", err)
	}
	if current.ApprovalID != approval.ApprovalID || current.Revoked {
		t.Fatalf("separate store approval = %+v", current)
	}

	revoker := Actor{Issuer: "https://idp.example", Subject: "admin-b"}
	if _, err := writer.Revoke(ctx, approval.ApprovalID, scope.OrganizationID, revoker, time.Now().UTC()); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	current, err = reader.ResolveCurrent(ctx, scope)
	if err != nil {
		t.Fatalf("ResolveCurrent() after separate-store revocation: %v", err)
	}
	if current.ApprovalID != approval.ApprovalID || !current.Revoked {
		t.Fatalf("separate store returned stale approval after revocation: %+v", current)
	}
}

func TestStoreRejectsUnapprovedAndOverlongApprovals(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "budget-authority.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now()
	scope := Scope{OrganizationID: "org-a", ScenarioID: "scenario-a", PlanDigest: "sha256:plan-a"}
	actor := Actor{Issuer: "https://idp.example", Subject: "admin-a"}
	for _, test := range []struct {
		name      string
		decision  Decision
		expiresAt time.Time
	}{
		{name: "non-approved decision", decision: "pending", expiresAt: now.Add(time.Minute)},
		{name: "expired", decision: DecisionApproved, expiresAt: now},
		{name: "overlong", decision: DecisionApproved, expiresAt: now.Add(MaxApprovalLifetime + time.Nanosecond)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.Approve(ctx, scope, test.decision, actor, test.expiresAt, now); !errors.Is(err, ErrInvalidApproval) {
				t.Fatalf("Approve() error = %v, want invalid approval", err)
			}
		})
	}
	if _, err := store.ResolveCurrent(ctx, scope); !errors.Is(err, ErrNoApproval) {
		t.Fatalf("ResolveCurrent() error = %v, want no approval", err)
	}
}

func TestDelegationReplayProtectionPersistsAcrossOpen(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "budget-authority.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	expires := now.Add(30 * time.Second)
	hash := strings.Repeat("a", 64)
	if err := store.ConsumeOperatorDelegation(ctx, hash, expires, now); err != nil {
		t.Fatalf("first replay consume: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.ConsumeOperatorDelegation(ctx, hash, expires, now); !errors.Is(err, ErrReplay) {
		t.Fatalf("replayed delegation error = %v, want replay denied", err)
	}
}
