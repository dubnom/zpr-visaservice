package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"neboagency.com/zpr-dashborad/internal/changereview"
)

// These internal composition methods require prior caller authorization.
// The runtime lock fences API-managed source edits through the SQLite commit.
// External file writers are detected, not transactionally controlled.
func (runtime *assertionRuntime) submitSubjectLinkReview(ctx context.Context, store *changereview.Store, proposal changereview.SubjectLinkProposal, retryKey string) (changereview.SubjectLinkReview, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return store.SubmitSubjectLinkWithSourceOnce(ctx, proposal, retryKey, runtime.checkSubjectLinkSource)
}

func (runtime *assertionRuntime) approveSubjectLinkReview(ctx context.Context, store *changereview.Store, org, id, digest, version, registryRevision, reason string, reviewer changereview.Principal, retryKey string) (changereview.SubjectLinkReview, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return store.ApproveSubjectLinkWithSourceOnce(ctx, org, id, digest, version, registryRevision, reason, reviewer, retryKey, runtime.checkSubjectLinkSource)
}

// Called only with runtime.mu held and after exact retry receipt lookup.
func (runtime *assertionRuntime) checkSubjectLinkSource(ctx context.Context, proposal changereview.SubjectLinkProposal) error {
	if runtime.organizationID != proposal.Organization || runtime.sourcesRevision != proposal.RegistryRevision {
		return changereview.ErrConflict
	}
	checkFile := func() error {
		if runtime.sourcesPath == "" {
			return errors.New("Durable trusted source registry is required for subject review")
		}
		data, err := readOperatorFile(runtime.sourcesPath, 64<<10, true)
		if err != nil {
			return err
		}
		if sourceConfigRevision(data) != proposal.RegistryRevision {
			return changereview.ErrConflict
		}
		return nil
	}
	if err := checkFile(); err != nil {
		return err
	}
	if proposal.Target.SelectorKind != "ldap_entry_uuid" {
		return changereview.ErrInvalid
	}
	for _, entry := range runtime.sourcesConfig.Sources {
		if entry.Name != proposal.Target.SourceID {
			continue
		}
		if entry.Version == 0 || strconv.FormatUint(entry.Version, 10) != proposal.Target.SourceVersion {
			return changereview.ErrConflict
		}
		// Initial support uses exact approved LDAP attribute names, not an
		// invented user.* mapping or implicit group-membership expansion.
		if _, err := approvedAssertionAttributes(strings.Join(proposal.EnrichmentKeys, ",")); err != nil {
			return err
		}
		if _, err := lookupAssertionLDAPSubject(ctx, entry, proposal.Organization, proposal.Target.SubjectID, proposal.EnrichmentKeys); err != nil {
			return err
		}
		return checkFile()
	}
	return changereview.ErrNotFound
}
