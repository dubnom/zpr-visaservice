package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

const policyApplyProofHeader = "X-ZPR-Policy-Approval"
const policyApplyCandidateRoute = "/api/policy/operations/candidate"

type policyApplyCandidate struct {
	Stage     stagedPolicyCandidate                 `json:"stage"`
	Container string                                `json:"container"`
	Current   operatordelegation.PolicyCurrentState `json:"current"`
}

func verifiedPolicyPeer(r *http.Request) bool {
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		return false
	}
	for _, chain := range r.TLS.VerifiedChains {
		if len(chain) > 0 && chain[0].Equal(r.TLS.PeerCertificates[0]) {
			return true
		}
	}
	return false
}

func (p *policyApplyResources) handler(workspace *policyWorkspace) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fail := func(code int, err error) { writePolicyError(w, code, err.Error()) }
		if !verifiedPolicyPeer(r) {
			fail(403, errors.New("verified Policy-Service mTLS peer required in addition to signed approval"))
			return
		}
		if r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
			fail(400, errors.New("canonical policy operation URI required"))
			return
		}
		if r.Method == "GET" && r.URL.Path == policyApplyCandidateRoute {
			stage, container, err := p.stagedCandidate(r.Context(), workspace)
			if err != nil {
				fail(409, err)
				return
			}
			current, err := p.client.current(r.Context(), time.Now())
			if err != nil {
				fail(503, err)
				return
			}
			writeJSON(w, 200, policyApplyCandidate{stage, base64.StdEncoding.EncodeToString(container), current})
			return
		}
		if r.Method != "POST" || r.URL.Path != policyApplyProducerRoute {
			http.NotFound(w, r)
			return
		}
		values := r.Header.Values(policyApplyProofHeader)
		if len(values) != 1 || len(values[0]) > 16384 {
			fail(403, errors.New("exact named-reviewer Policy-Service approval required"))
			return
		}
		var proof operatordelegation.PolicyProof
		if decodeOperatorConfig([]byte(values[0]), &proof) != nil {
			fail(403, errors.New("malformed policy approval"))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 96<<20))
		if err != nil {
			fail(413, errors.New("policy command exceeds limit"))
			return
		}
		now := time.Now()
		approval, err := operatordelegation.VerifyPolicyPair(p.client.trust, r.Method, r.URL.RequestURI(), body, proof, now)
		if err != nil {
			fail(403, err)
			return
		}
		if approval.Claims().Action != "install" {
			fail(403, errors.New("only reviewed policy installation is supported"))
			return
		}
		// Existing operations reconcile even after their original source is superseded.
		// A new operation must still name the authoritative current saved/staged artifact.
		_, err = p.store.PolicyOperation(r.Context(), p.client.trust.Organization, approval.Claims().Operation.OperationID)
		if errors.Is(err, changereview.ErrNotFound) {
			stage, _, stageErr := p.stagedCandidate(r.Context(), workspace)
			op := approval.Claims().Operation
			if stageErr != nil {
				fail(409, stageErr)
				return
			}
			if stage.BundleSHA256 != op.CandidateDigest || uint64(stage.RecordRevision) != op.SourceRevision {
				fail(409, errors.New("signed candidate differs from current Policy-Service stage"))
				return
			}
		} else if err != nil {
			fail(503, err)
			return
		}
		record, err := p.client.apply(r.Context(), p.store, body, proof, now)
		if err != nil {
			fail(409, err)
			return
		}
		writeJSON(w, 200, record)
	})
}

func (p *policyApplyResources) stagedCandidate(ctx context.Context, workspace *policyWorkspace) (stagedPolicyCandidate, []byte, error) {
	if workspace == nil || workspace.store == nil {
		return stagedPolicyCandidate{}, nil, errors.New("saved policy repository unavailable")
	}
	workspace.mu.Lock()
	var stage stagedPolicyCandidate
	if workspace.stagedCandidate != nil {
		stage = *workspace.stagedCandidate
	}
	workspace.mu.Unlock()
	if stage.bundlePath == "" {
		return stage, nil, errors.New("no authoritative staged artifact")
	}
	record, err := workspace.store.GetRecord(ctx, stage.RecordID)
	if err != nil {
		return stage, nil, err
	}
	if record.Archived || record.Kind != "policy" || record.CurrentRevision != stage.RecordRevision ||
		policyContainerDigest([]byte(record.Content)) != stage.SourceSHA256 {
		return stage, nil, errors.New("saved source no longer matches stage")
	}
	data, err := readOperatorFile(stage.bundlePath, maxStagedPolicyBundleBytes, true)
	if err != nil {
		return stage, nil, err
	}
	if int64(len(data)) != stage.BundleSize || policyContainerDigest(data) != stage.BundleSHA256 ||
		stage.ID != stage.BundleSHA256 {
		return stage, nil, errors.New("staged artifact changed")
	}
	config, err := readOperatorFile(workspace.stageConfigPath, 1<<20, true)
	if err != nil {
		return stage, nil, err
	}
	if policyContainerDigest(config) != stage.ConfigSHA256 {
		return stage, nil, errors.New("compiler configuration changed since staging")
	}
	if err := validatePolicySourceBindings(stage, workspace.stageConfigPath); err != nil {
		return stage, nil, err
	}
	workspace.mu.Lock()
	same := workspace.stagedCandidate != nil && reflect.DeepEqual(*workspace.stagedCandidate, stage)
	workspace.mu.Unlock()
	if !same {
		return stage, nil, errors.New("stage replaced during capture")
	}
	return stage, data, nil
}

func configuredPolicyApplyResources() (*policyApplyResources, error) {
	path := os.Getenv("ZPR_POLICY_APPLY_CONFIG_FILE")
	if path == "" {
		return nil, nil
	}
	resources, err := loadPolicyApplyResources(path, time.Now())
	if err != nil {
		return nil, err
	}
	if os.Getenv("ZPR_POLICY_ORGANIZATION_ID") != resources.client.trust.Organization {
		_ = resources.Close()
		return nil, errors.New("Policy-Service source organization differs from guarded trust")
	}
	return resources, nil
}

// Optional resources are only supplied by protected startup configuration.
func policyServiceHandler(workspace *policyWorkspace, resources *policyApplyResources) http.Handler {
	workspace.guardedApply = resources
	return policyServiceMux(workspace)
}

func encodePolicyApproval(proof operatordelegation.PolicyProof) (string, error) {
	b, err := json.Marshal(proof)
	return string(b), err
}
