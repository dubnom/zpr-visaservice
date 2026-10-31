package enrollment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

const maxBatchInvitations = 128

type BatchReviewRequest struct {
	Organization  string   `json:"organization"`
	InvitationIDs []string `json:"invitation_ids"`
}

type BatchApprovalRequest struct {
	Organization    string   `json:"organization"`
	InvitationIDs   []string `json:"invitation_ids"`
	InventoryDigest string   `json:"inventory_digest"`
	Reason          string   `json:"reason"`
}

type BatchInventory struct {
	Version      int          `json:"version"`
	Organization string       `json:"organization"`
	Digest       string       `json:"digest"`
	Invitations  []Invitation `json:"invitations"`
}

type BatchApprovalReceipt struct {
	Inventory BatchInventory `json:"reviewed_inventory"`
	Principal string         `json:"principal"`
	Reason    string         `json:"reason"`
	DecidedAt time.Time      `json:"decided_at"`
}

func (s *Store) GetBatchApproval(ctx context.Context, organization, digest string) (BatchApprovalReceipt, error) {
	if !validText(organization) || !validFingerprint(digest) {
		return BatchApprovalReceipt{}, ErrInvalid
	}
	var receipt BatchApprovalReceipt
	var snapshot []byte
	var decidedAt int64
	err := s.db.QueryRowContext(ctx, `SELECT inventory_json,principal,reason,decided_at
 FROM enrollment_batch_reviews WHERE organization=? AND inventory_digest=?`,
		organization, digest).Scan(&snapshot, &receipt.Principal, &receipt.Reason, &decidedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return BatchApprovalReceipt{}, ErrNotFound
	}
	if err != nil {
		return BatchApprovalReceipt{}, err
	}
	if err := decodeJSON(bytes.NewReader(snapshot), &receipt.Inventory); err != nil {
		return BatchApprovalReceipt{}, fmt.Errorf("decode recorded batch inventory: %w", err)
	}
	recordedDigest, err := batchInventoryDigest(receipt.Inventory)
	if err != nil || receipt.Inventory.Version != 1 || receipt.Inventory.Organization != organization ||
		receipt.Inventory.Digest != digest || recordedDigest != digest ||
		!validText(receipt.Principal) || !validText(receipt.Reason) || decidedAt <= 0 {
		return BatchApprovalReceipt{}, errors.New("recorded enrollment batch approval is invalid")
	}
	receipt.DecidedAt = time.Unix(0, decidedAt).UTC()
	return receipt, nil
}

func readBatchInventory(ctx context.Context, tx *sql.Tx, request BatchReviewRequest, now time.Time) (BatchInventory, error) {
	if !validText(request.Organization) || now.IsZero() ||
		len(request.InvitationIDs) == 0 || len(request.InvitationIDs) > maxBatchInvitations {
		return BatchInventory{}, ErrInvalid
	}
	ids := slices.Clone(request.InvitationIDs)
	slices.Sort(ids)
	inventory := BatchInventory{Version: 1, Organization: request.Organization}
	keys := make(map[string]bool)
	for index, id := range ids {
		if !validText(id) || (index > 0 && ids[index-1] == id) {
			return BatchInventory{}, ErrInvalid
		}
		item, err := readInvitation(ctx, tx, request.Organization, id, now)
		if err != nil {
			return BatchInventory{}, err
		}
		if item.State != "pending_approval" {
			return BatchInventory{}, ErrUnavailable
		}
		if !validFingerprint(item.KeyFingerprint) || !validFingerprint(item.RuntimeKeyFingerprint) ||
			item.KeyFingerprint == item.RuntimeKeyFingerprint || keys[item.KeyFingerprint] || keys[item.RuntimeKeyFingerprint] {
			return BatchInventory{}, ErrInvalid
		}
		keys[item.KeyFingerprint], keys[item.RuntimeKeyFingerprint] = true, true
		inventory.Invitations = append(inventory.Invitations, item)
	}
	digest, err := batchInventoryDigest(inventory)
	inventory.Digest = digest
	return inventory, err
}

func batchInventoryDigest(inventory BatchInventory) (string, error) {
	inventory.Digest = ""
	content, err := json.Marshal(inventory)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(append([]byte("zpr-enrollment-batch-inventory-v1\x00"), content...))
	return hex.EncodeToString(hash[:]), nil
}

func (s *Store) ReviewBatch(ctx context.Context, request BatchReviewRequest, now time.Time) (BatchInventory, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BatchInventory{}, err
	}
	defer tx.Rollback()
	return readBatchInventory(ctx, tx, request, now)
}

func (s *Store) ApproveBatch(ctx context.Context, request BatchApprovalRequest, principal string, catalog Organization, now time.Time) (BatchInventory, error) {
	return s.approveBatch(ctx, request, principal, catalog, now, true)
}

func (s *Store) approveBatch(ctx context.Context, request BatchApprovalRequest, principal string, catalog Organization, now time.Time, allowSelf bool) (BatchInventory, error) {
	if !validText(principal) || !validText(request.Reason) || !validFingerprint(request.InventoryDigest) {
		return BatchInventory{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return BatchInventory{}, err
	}
	defer tx.Rollback()
	inventory, err := readBatchInventory(ctx, tx, BatchReviewRequest{
		Organization: request.Organization, InvitationIDs: request.InvitationIDs,
	}, now)
	if err != nil {
		return BatchInventory{}, err
	}
	if inventory.Digest != request.InventoryDigest {
		return BatchInventory{}, ErrRevision
	}
	for _, item := range inventory.Invitations {
		if item.CreatedBy == principal && !allowSelf {
			return BatchInventory{}, errSelfApproval
		}
	}
	snapshot, err := json.Marshal(inventory)
	if err != nil {
		return BatchInventory{}, err
	}
	for index, item := range inventory.Invitations {
		if !slices.Contains(catalog.Profiles, item.Asset.Profile) || !slices.Contains(catalog.Types, item.Asset.Type) {
			return BatchInventory{}, ErrUnavailable
		}
		approved, err := decideInvitation(ctx, tx, item, principal, "approved", request.Reason,
			item.KeyFingerprint, item.RuntimeKeyFingerprint, item.Revision, now)
		if err != nil {
			return BatchInventory{}, err
		}
		inventory.Invitations[index] = approved
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO enrollment_batch_reviews
 (organization,inventory_digest,principal,reason,decided_at,inventory_json) VALUES(?,?,?,?,?,?)`,
		request.Organization, inventory.Digest, principal, request.Reason, now.UnixNano(), snapshot); err != nil {
		return BatchInventory{}, err
	}
	if err := tx.Commit(); err != nil {
		return BatchInventory{}, err
	}
	return inventory, nil
}
