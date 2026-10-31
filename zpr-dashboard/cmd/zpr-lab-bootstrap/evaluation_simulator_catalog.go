package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
)

var evaluationSimulatorCatalogIDs = []string{"great-lakes", "load-lab", "northstar", "redwood", "velocity"}

func prepareEvaluationSimulatorCatalog(evaluation, state, catalog string, env []string) ([]string, error) {
	if !filepath.IsAbs(catalog) {
		return nil, errors.New("catalog requires an absolute path")
	}
	resolved, err := filepath.EvalSymlinks(catalog)
	if err != nil || resolved != filepath.Clean(catalog) {
		return nil, errors.New("catalog must be an existing non-symlink directory")
	}
	for _, id := range evaluationSimulatorCatalogIDs {
		data, err := os.ReadFile(filepath.Join(catalog, "organizations", id+".json"))
		var profile struct {
			ID string `json:"id"`
		}
		if err != nil || json.Unmarshal(data, &profile) != nil || profile.ID != id {
			return nil, fmt.Errorf("required catalog profile %s unavailable or invalid", id)
		}
	}
	for _, directory := range []string{"organizations", "scenarios"} {
		root := filepath.Join(catalog, directory)
		if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(catalog, path)
			if err != nil {
				return err
			}
			if relative == filepath.Join("organizations", "demo.json") ||
				relative == filepath.Join("organizations", "demo") {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			target := filepath.Join(state, relative)
			if entry.IsDir() {
				if err := os.Mkdir(target, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
					return err
				}
				info, err := os.Lstat(target)
				if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
					return fmt.Errorf("unsafe catalog destination %s", relative)
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("catalog asset %s must be a regular file", relative)
			}
			if info, err := os.Lstat(target); err == nil {
				if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
					return fmt.Errorf("unsafe catalog asset %s", relative)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return writeEvaluationOnce(state, relative, data)
		}); err != nil {
			return nil, fmt.Errorf("install %s: %w", directory, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(catalog, "manifest.json"))
	var manifest map[string]any
	if err != nil || json.Unmarshal(data, &manifest) != nil {
		return nil, errors.New("packaged catalog manifest unavailable or invalid")
	}
	data, err = os.ReadFile(filepath.Join(state, "manifest.json"))
	var pairedManifest struct {
		OrganizationID string `json:"organization_id"`
	}
	if err != nil || json.Unmarshal(data, &pairedManifest) != nil || pairedManifest.OrganizationID == "" {
		return nil, errors.New("paired manifest organization unavailable")
	}
	manifest["organization_id"] = pairedManifest.OrganizationID
	data, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(filepath.Join(state, "catalog-manifest.json")); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return nil, errors.New("catalog manifest must be a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := writeEvaluationOnce(state, "catalog-manifest.json", append(data, '\n')); err != nil {
		return nil, err
	}
	data, err = os.ReadFile(filepath.Join(evaluation, "simulator-oidc.json"))
	var login operatorauth.Config
	if err != nil || json.Unmarshal(data, &login) != nil || login.Validate() != nil {
		return nil, errors.New("paired Simulator login configuration unavailable")
	}
	// Scope only the Simulator's local session; production grants stay unchanged.
	for i := range login.Grants {
		login.Grants[i].Organizations = append(login.Grants[i].Organizations, evaluationSimulatorCatalogIDs...)
	}
	if err := login.Validate(); err != nil {
		return nil, err
	}
	data, err = json.MarshalIndent(login, "", "  ")
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(filepath.Join(state, "catalog-oidc.json")); err == nil {
		if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return nil, errors.New("catalog login configuration must be a private regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := writeEvaluationOnce(state, "catalog-oidc.json", append(data, '\n')); err != nil {
		return nil, err
	}
	catalogEnv := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if strings.HasPrefix(entry, "SIMULATION_PREGEN_DIR=") {
			continue
		}
		if strings.HasPrefix(entry, "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE=") {
			entry = "ZPR_SIMULATOR_OPERATOR_OIDC_CONFIG_FILE=" + filepath.Join(state, "catalog-oidc.json")
		}
		if strings.HasPrefix(entry, "SIMULATION_MANIFEST=") {
			entry = "SIMULATION_MANIFEST=" + filepath.Join(state, "catalog-manifest.json")
		}
		catalogEnv = append(catalogEnv, entry)
	}
	return append(catalogEnv, "SIMULATION_PREGEN_DIR="+filepath.Join(state, "organizations", "northstar")), nil
}
