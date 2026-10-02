package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxStagedPolicyBundleBytes = 64 << 20
	policyStageTimeout         = 90 * time.Second
)

type stagedPolicyCandidate struct {
	ID             string    `json:"id"`
	RecordID       string    `json:"record_id"`
	RecordName     string    `json:"record_name"`
	RecordRevision int       `json:"record_revision"`
	SourceSHA256   string    `json:"source_sha256"`
	ConfigSHA256   string    `json:"config_sha256"`
	BundleSHA256   string    `json:"bundle_sha256"`
	BundleSize     int64     `json:"bundle_size"`
	StagedAt       time.Time `json:"staged_at"`
	bundlePath     string
}

func (workspace *policyWorkspace) configurePolicyStaging() {
	workspace.stageDirectory = strings.TrimSpace(os.Getenv("ZPR_POLICY_STAGE_DIR"))
	workspace.stageConfigPath = strings.TrimSpace(os.Getenv("ZPR_POLICY_STAGE_CONFIG_FILE"))
	if workspace.stageConfigPath == "" {
		workspace.stageConfigPath = workspace.configPath
	}
	workspace.stageSigningKeyPath = strings.TrimSpace(os.Getenv("ZPR_POLICY_STAGE_SIGNING_KEY_FILE"))
	if workspace.stageDirectory == "" {
		return
	}
	metadataPath := filepath.Join(workspace.stageDirectory, "latest.json")
	content, err := os.ReadFile(metadataPath)
	if err != nil {
		return
	}
	var candidate stagedPolicyCandidate
	if len(content) > 16<<10 || json.Unmarshal(content, &candidate) != nil || len(candidate.BundleSHA256) != sha256.Size*2 {
		return
	}
	candidate.bundlePath = filepath.Join(workspace.stageDirectory, "candidate-"+candidate.ID+".bin2")
	workspace.stagedCandidate = &candidate
}

func (workspace *policyWorkspace) stagingReady() bool {
	return workspace.compiler != "" && workspace.stageDirectory != "" && workspace.stageConfigPath != "" && workspace.stageSigningKeyPath != ""
}

func (workspace *policyWorkspace) compileAndStagePolicyRecord(ctx context.Context, record policyRecord) (*stagedPolicyCandidate, error) {
	workspace.stageMu.Lock()
	defer workspace.stageMu.Unlock()
	if !workspace.stagingReady() {
		return nil, errors.New("Policy staging is not configured.")
	}
	if record.Kind != "policy" || record.CurrentRevision <= 0 || strings.TrimSpace(record.Content) == "" {
		return nil, errors.New("A saved, non-empty policy record is required for staging.")
	}
	configPath, err := resolvedRegularFile(workspace.stageConfigPath)
	if err != nil {
		return nil, errors.New("Runtime ZPLC config is unavailable for staging.")
	}
	signingKeyPath, err := resolvedRegularFile(workspace.stageSigningKeyPath)
	if err != nil {
		return nil, errors.New("Policy signing key is unavailable for staging.")
	}
	if len(record.Content) > maxPolicySourceBytes {
		return nil, errors.New("Selected policy source exceeds the size limit.")
	}
	configContents, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.New("Runtime ZPLC config cannot be read for staging.")
	}
	stageDirectory, err := filepath.Abs(workspace.stageDirectory)
	if err != nil {
		return nil, errors.New("Unable to resolve the policy staging directory.")
	}
	if err := os.MkdirAll(stageDirectory, 0o700); err != nil {
		return nil, errors.New("Unable to create the private policy staging directory.")
	}
	if err := ensureParentDirectory(filepath.Join(stageDirectory, "candidate.bin2")); err != nil {
		return nil, errors.New("Policy staging directory must be private.")
	}
	temporaryDirectory, err := os.MkdirTemp(stageDirectory, ".compile-")
	if err != nil {
		return nil, errors.New("Unable to create a temporary policy staging workspace.")
	}
	defer os.RemoveAll(temporaryDirectory)
	sourcePath := filepath.Join(temporaryDirectory, "candidate.zpl")
	bundlePath := filepath.Join(temporaryDirectory, "candidate.bin2")
	if err := os.WriteFile(sourcePath, []byte(record.Content), 0o600); err != nil {
		return nil, errors.New("Unable to prepare the staged policy source.")
	}
	compileContext, cancel := context.WithTimeout(ctx, policyStageTimeout)
	defer cancel()
	command := exec.CommandContext(compileContext, workspace.compiler, "-c", configPath, "-k", signingKeyPath, "-f", "v2", "-o", bundlePath, sourcePath)
	command.Dir = filepath.Dir(configPath)
	output := &limitedBuffer{limit: maxPolicyOutputBytes}
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		diagnostics := strings.TrimSpace(strings.ReplaceAll(output.String(), temporaryDirectory, "staged-policy"))
		if diagnostics == "" {
			return nil, errors.New("ZPLC could not compile the selected policy with the runtime base source.")
		}
		return nil, fmt.Errorf("ZPLC compilation failed: %s", diagnostics)
	}
	bundleInfo, err := os.Stat(bundlePath)
	if err != nil || !bundleInfo.Mode().IsRegular() || bundleInfo.Size() <= 0 || bundleInfo.Size() > maxStagedPolicyBundleBytes {
		return nil, errors.New("ZPLC produced no valid staged policy bundle.")
	}
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		return nil, errors.New("Unable to read the compiled staged policy bundle.")
	}
	sourceHash := sha256.Sum256([]byte(record.Content))
	configHash := sha256.Sum256(configContents)
	bundleHash := sha256.Sum256(bundle)
	bundleSHA256 := hex.EncodeToString(bundleHash[:])
	candidate := &stagedPolicyCandidate{
		ID: bundleSHA256, RecordID: record.ID, RecordName: record.Name,
		RecordRevision: record.CurrentRevision, SourceSHA256: hex.EncodeToString(sourceHash[:]), ConfigSHA256: hex.EncodeToString(configHash[:]),
		BundleSHA256: bundleSHA256, BundleSize: int64(len(bundle)), StagedAt: time.Now().UTC(),
		bundlePath: filepath.Join(stageDirectory, "candidate-"+bundleSHA256+".bin2"),
	}
	if err := os.Rename(bundlePath, candidate.bundlePath); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return nil, errors.New("Unable to store the staged policy bundle.")
		}
	}
	metadata, err := json.Marshal(candidate)
	if err != nil {
		return nil, errors.New("Unable to record staged policy metadata.")
	}
	metadataPath := filepath.Join(temporaryDirectory, "latest.json")
	if err := os.WriteFile(metadataPath, metadata, 0o600); err != nil {
		return nil, errors.New("Unable to prepare staged policy metadata.")
	}
	if err := os.Rename(metadataPath, filepath.Join(stageDirectory, "latest.json")); err != nil {
		return nil, errors.New("Unable to publish staged policy metadata.")
	}
	workspace.mu.Lock()
	workspace.stagedCandidate = candidate
	workspace.mu.Unlock()
	return candidate, nil
}
