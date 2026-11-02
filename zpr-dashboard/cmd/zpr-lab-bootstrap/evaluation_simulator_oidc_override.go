package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

// evaluationSimulatorOIDCOverridePermissions are the only permissions an
// explicit operator-supplied Simulator session override may add to an
// existing named operator: the enrollment administration contract, the
// bounded scenario budget approval, and Simulator-private workspace scenario
// authoring (edit/publish operate only on the Simulator's own workspace
// artifacts, never on enrollment or production state). Identity, issuer,
// client, callback, organizations and every retained permission are
// preserved exactly, and approval.self follows its own earlier explicit
// mechanism, never this one.
var evaluationSimulatorOIDCOverridePermissions = []string{
	"read", "create", "cancel", "approve", "reject", "revoke", "budget.approve",
	"scenario.edit", "scenario.publish", "scenario.reset",
}

// applyEvaluationSimulatorOIDCOverride validates an explicit operator-owned
// Simulator OIDC session override against the frozen verified configuration
// and points the Simulator at it. The frozen file is never modified and an
// invalid override fails closed before the Simulator starts.
func applyEvaluationSimulatorOIDCOverride(evaluation, overridePath string, env []string) ([]string, error) {
	if !filepath.IsAbs(overridePath) || filepath.Clean(overridePath) != overridePath ||
		strings.ContainsAny(overridePath, ",\"\r\n=") {
		return nil, errors.New("simulator-oidc-file requires a canonical absolute bind-mount-safe path")
	}
	data, err := servicehttp.ReadOperatorFile(overridePath, 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read protected Simulator OIDC override: %w", err)
	}
	var configured operatorauth.Config
	if err := decodeEvaluationSimulatorOIDC(data, &configured); err != nil {
		return nil, fmt.Errorf("Simulator OIDC override: %w", err)
	}
	base, err := servicehttp.ReadOperatorFile(filepath.Join(evaluation, "simulator-oidc.json"), 65536, true)
	if err != nil {
		return nil, fmt.Errorf("read frozen Simulator OIDC configuration: %w", err)
	}
	var original operatorauth.Config
	if err := decodeEvaluationSimulatorOIDC(base, &original); err != nil {
		return nil, fmt.Errorf("frozen Simulator OIDC configuration: %w", err)
	}
	grants := configured.Grants
	configured.Grants = original.Grants
	if !reflect.DeepEqual(configured, original) || len(grants) != len(original.Grants) {
		return nil, errors.New("Simulator OIDC override must preserve the frozen issuer, client, callback, session lifetime and named identities")
	}
	for i, grant := range grants {
		retained := original.Grants[i]
		if grant.Issuer != retained.Issuer || grant.Subject != retained.Subject ||
			!slices.Equal(grant.Organizations, retained.Organizations) {
			return nil, errors.New("Simulator OIDC override must preserve exact named organization grants")
		}
		for _, permission := range retained.Permissions {
			if !slices.Contains(grant.Permissions, permission) {
				return nil, errors.New("Simulator OIDC override cannot remove retained permissions")
			}
		}
		for _, permission := range grant.Permissions {
			if !slices.Contains(retained.Permissions, permission) &&
				!slices.Contains(evaluationSimulatorOIDCOverridePermissions, permission) {
				return nil, errors.New("Simulator OIDC override may add only explicit enrollment administration and bounded budget approval permissions")
			}
		}
	}
	const prefix = "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE="
	replaced := false
	updated := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			entry = prefix + overridePath
			replaced = true
		}
		updated = append(updated, entry)
	}
	if !replaced {
		return nil, errors.New("Simulator OIDC override requires the prepared paired login environment")
	}
	return updated, nil
}

func decodeEvaluationSimulatorOIDC(data []byte, config *operatorauth.Config) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(config); err != nil {
		return errors.New("configuration must be strict JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("configuration must contain exactly one JSON document")
	}
	return config.Validate()
}
