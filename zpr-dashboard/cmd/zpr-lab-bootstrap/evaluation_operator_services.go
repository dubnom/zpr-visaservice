package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

type evaluationMutualTLSServiceBinding struct {
	URL                   string `json:"url"`
	ClientCertificateFile string `json:"client_certificate_file"`
	ClientKeyFile         string `json:"client_key_file"`
	CAFile                string `json:"ca_file"`
}

type evaluationOperatorServices struct {
	Version                      int                                `json:"version"`
	Organization                 string                             `json:"organization"`
	PolicyService                *evaluationMutualTLSServiceBinding `json:"policy_service,omitempty"`
	EnrollmentService            *evaluationMutualTLSServiceBinding `json:"enrollment_service,omitempty"`
	ChangeReviewConfigFile       string                             `json:"change_review_config_file,omitempty"`
	OperatorOIDCConfigFile       string                             `json:"operator_oidc_config_file,omitempty"`
	OperatorDelegationTrustFile  string                             `json:"operator_delegation_trust_file,omitempty"`
	VisaServiceConfigFile        string                             `json:"visa_service_config_file,omitempty"`
	VisaServiceReadOnlyFiles     []string                           `json:"visa_service_read_only_files,omitempty"`
	enrollmentDelegationAudience string
}

func loadEvaluationOperatorServices(path, state string, inventory evaluationInventory) (*evaluationOperatorServices, error) {
	if path == "" {
		return nil, nil
	}
	read := func(path string) ([]byte, error) {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, ",\"\r\n=") {
			return nil, errors.New("operator service files require canonical absolute bind-mount-safe paths")
		}
		data, err := servicehttp.ReadOperatorFile(path, 1<<20, true)
		if err != nil {
			return nil, fmt.Errorf("read protected operator service input %s: %w", path, err)
		}
		return data, nil
	}
	data, err := read(path)
	if err != nil {
		return nil, err
	}
	var config evaluationOperatorServices
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("operator services JSON: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("operator services requires one JSON object")
	}
	if config.Version != 1 || config.Organization != evaluationOrganization(inventory) {
		return nil, errors.New("operator services requires version 1 and this installation's exact organization")
	}
	for _, binding := range []*evaluationMutualTLSServiceBinding{config.PolicyService, config.EnrollmentService} {
		if binding == nil {
			continue
		}
		origin, err := url.Parse(binding.URL)
		if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil || binding.URL != "https://"+origin.Host {
			return nil, errors.New("operator service requires an exact HTTPS origin")
		}
		seen := map[string]bool{}
		for _, path := range []string{binding.ClientCertificateFile, binding.ClientKeyFile, binding.CAFile} {
			if seen[path] {
				return nil, errors.New("operator service TLS inputs must be distinct")
			}
			seen[path] = true
			if _, err := read(path); err != nil {
				return nil, err
			}
		}
		transport, err := servicehttp.MutualTLSClientTransport(binding.ClientCertificateFile, binding.ClientKeyFile, binding.CAFile, origin.Hostname())
		if err != nil {
			return nil, fmt.Errorf("operator service TLS: %w", err)
		}
		transport.CloseIdleConnections()
		if binding == config.EnrollmentService {
			data, err := read(filepath.Join(state, evaluationDelegationTrust))
			if err != nil {
				return nil, err
			}
			var trust operatordelegation.Config
			decoder := json.NewDecoder(bytes.NewReader(data))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&trust); err != nil {
				return nil, err
			}
			if decoder.Decode(new(any)) != io.EOF {
				return nil, errors.New("operator delegation trust requires one JSON object")
			}
			if err := trust.Validate(); err != nil {
				return nil, err
			}
			config.enrollmentDelegationAudience = evaluationServiceAudience(inventory.Config)
			if trust.Audience != config.enrollmentDelegationAudience {
				return nil, errors.New("external enrollment delegation must retain this installation's exact Control-Service audience")
			}
			pin := sha256.Sum256(transport.TLSClientConfig.Certificates[0].Certificate[0])
			if !slices.ContainsFunc(trust.Keys, func(key operatordelegation.TrustedKey) bool {
				return key.CertificateSHA256 == hex.EncodeToString(pin[:])
			}) {
				return nil, errors.New("external enrollment transport identity must match retained delegation certificate pin")
			}
		}
	}
	if config.ChangeReviewConfigFile != "" {
		if config.PolicyService == nil {
			return nil, errors.New("operator change review requires an explicit policy service binding")
		}
		if _, err := read(config.ChangeReviewConfigFile); err != nil {
			return nil, err
		}
	}
	if config.OperatorDelegationTrustFile != "" {
		data, err := read(config.OperatorDelegationTrustFile)
		if err != nil {
			return nil, err
		}
		var configured operatordelegation.Config
		if err := decodeEvaluationDelegationTrust(data, &configured); err != nil {
			return nil, err
		}
		base, err := read(filepath.Join(state, evaluationDelegationTrust))
		if err != nil {
			return nil, err
		}
		var original operatordelegation.Config
		if err := decodeEvaluationDelegationTrust(base, &original); err != nil {
			return nil, err
		}
		if configured.Version != original.Version || configured.Audience != original.Audience ||
			!reflect.DeepEqual(configured.Keys, original.Keys) || len(configured.Grants) != len(original.Grants) {
			return nil, errors.New("operator delegation override must preserve retained audience, keys and named identities")
		}
		for i, grant := range configured.Grants {
			base := original.Grants[i]
			if grant.Issuer != base.Issuer || grant.Subject != base.Subject ||
				!slices.Equal(grant.Organizations, base.Organizations) ||
				!slices.Equal(grant.Organizations, []string{config.Organization}) {
				return nil, errors.New("operator delegation override must preserve exact named organization grants")
			}
			permissions := slices.Clone(grant.Permissions)
			if !slices.Contains(base.Permissions, "approval.self") {
				permissions = slices.DeleteFunc(permissions, func(permission string) bool { return permission == "approval.self" })
			}
			if !slices.Equal(permissions, base.Permissions) ||
				(slices.Contains(grant.Permissions, "approval.self") &&
					!slices.Contains(base.Permissions, "subject-link.approve") && !slices.Contains(base.Permissions, "approve")) {
				return nil, errors.New("operator delegation override may add approval.self only to existing named approvers")
			}
		}
	}
	if config.OperatorOIDCConfigFile != "" {
		data, err := read(config.OperatorOIDCConfigFile)
		if err != nil {
			return nil, err
		}
		var configured operatorauth.Config
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&configured); err != nil {
			return nil, err
		}
		if decoder.Decode(new(any)) != io.EOF {
			return nil, errors.New("operator OIDC requires one JSON object")
		}
		if err := configured.Validate(); err != nil {
			return nil, err
		}
		base, err := os.ReadFile(filepath.Join(state, "operator-oidc.json"))
		if err != nil {
			return nil, err
		}
		var original operatorauth.Config
		if err := json.Unmarshal(base, &original); err != nil {
			return nil, err
		}
		grants := configured.Grants
		configured.Grants = original.Grants
		if !reflect.DeepEqual(configured, original) || len(grants) != len(original.Grants) {
			return nil, errors.New("operator OIDC override must preserve the frozen issuer, client, callback and named identities")
		}
		for i, grant := range grants {
			base := original.Grants[i]
			if grant.Issuer != base.Issuer || grant.Subject != base.Subject ||
				!reflect.DeepEqual(grant.Organizations, base.Organizations) ||
				!slices.Equal(grant.Organizations, []string{config.Organization}) {
				return nil, errors.New("operator OIDC override must preserve exact named organization grants")
			}
			for _, permission := range base.Permissions {
				if !slices.Contains(grant.Permissions, permission) {
					return nil, errors.New("operator OIDC override cannot remove retained permissions")
				}
			}
			for _, permission := range grant.Permissions {
				policyReviewPermission := slices.Contains([]string{"policy.apply", "change.read", "change.submit", "change.review", "gateway.read", "approval.self"}, permission)
				enrollmentPermission := config.EnrollmentService != nil && slices.Contains([]string{"read", "create", "cancel", "approve", "reject"}, permission)
				if !slices.Contains(base.Permissions, permission) && (!slices.Contains(base.Permissions, "policy.edit") || (!policyReviewPermission && !enrollmentPermission)) {
					return nil, errors.New("operator OIDC override may add explicitly configured policy/enrollment service permissions only to existing named editors")
				}
			}
		}
	}
	if config.VisaServiceConfigFile == "" {
		if len(config.VisaServiceReadOnlyFiles) != 0 {
			return nil, errors.New("additional Visa files require an explicit Visa configuration")
		}
		return &config, nil
	}
	if !inventory.Config.DataPlane {
		return nil, errors.New("operator Visa configuration requires the data-plane runtime")
	}
	data, err = read(config.VisaServiceConfigFile)
	if err != nil {
		return nil, err
	}
	base, err := os.ReadFile(filepath.Join(state, "vs.toml"))
	if err != nil {
		return nil, err
	}
	var original, configured map[string]any
	if _, err := toml.Decode(string(base), &original); err != nil {
		return nil, err
	}
	if _, err := toml.Decode(string(data), &configured); err != nil {
		return nil, err
	}
	guarded, ok := configured["guarded_policy"].(map[string]any)
	if !ok {
		return nil, errors.New("operator Visa configuration requires explicit guarded policy trust")
	}
	encoded, err := json.Marshal(guarded)
	if err != nil {
		return nil, err
	}
	var trust struct {
		Audience              string                           `json:"audience"`
		PolicyServiceAudience string                           `json:"policy_service_audience"`
		TargetID              string                           `json:"target_id"`
		Organization          string                           `json:"organization"`
		Keys                  []operatordelegation.PolicyKey   `json:"keys"`
		Grants                []operatordelegation.PolicyGrant `json:"grants"`
	}
	strict := json.NewDecoder(bytes.NewReader(encoded))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&trust); err != nil {
		return nil, fmt.Errorf("operator Visa guarded trust: %w", err)
	}
	proofTrust := operatordelegation.PolicyTrust{Audience: trust.PolicyServiceAudience, VisaServiceAudience: trust.Audience,
		Organization: trust.Organization, TargetID: trust.TargetID, Keys: trust.Keys, Grants: trust.Grants}
	if err := proofTrust.Validate(time.Now()); err != nil {
		return nil, fmt.Errorf("operator Visa guarded trust: %w", err)
	}
	if trust.Organization != config.Organization || config.PolicyService == nil || trust.PolicyServiceAudience != config.PolicyService.URL ||
		trust.Audience != "https://localhost:"+strconv.Itoa(inventory.Config.VSAdminPort) {
		return nil, errors.New("operator Visa trust differs from this exact organization and configured service origins")
	}
	// Authorization may be attached without rebinding retained core identity,
	// TLS, storage, geography or subject snapshot configuration.
	for _, key := range []string{"guarded_policy", "trusted_service_http"} {
		delete(original, key)
		delete(configured, key)
	}
	if !reflect.DeepEqual(original, configured) {
		return nil, errors.New("operator Visa configuration may add guarded/trusted-service authorization only; frozen base settings differ")
	}
	readOnly, writable := evaluationDataPlaneMounts(state, inventory.Config)
	seen := map[string]bool{path: true}
	for _, name := range append([]string{config.VisaServiceConfigFile}, config.VisaServiceReadOnlyFiles...) {
		if seen[name] || slices.Contains(readOnly, name) || slices.Contains(writable, name) {
			return nil, errors.New("operator Visa inputs must be distinct from each other and existing data-plane mounts")
		}
		for _, mounted := range append(slices.Clone(readOnly), writable...) {
			if strings.HasPrefix(name, mounted+string(filepath.Separator)) {
				return nil, errors.New("operator Visa inputs cannot overlap existing data-plane directory mounts")
			}
		}
		seen[name] = true
		if _, err := read(name); err != nil {
			return nil, err
		}
	}
	return &config, nil
}

func (services *evaluationOperatorServices) environment(mode string) []string {
	if services == nil {
		return nil
	}
	if mode == "control-room" {
		var env []string
		if services.ChangeReviewConfigFile != "" {
			env = append(env, "ZPR_CHANGE_REVIEW_CONFIG_FILE="+services.ChangeReviewConfigFile)
		}
		if services.OperatorOIDCConfigFile != "" {
			env = append(env, "ZPR_OPERATOR_OIDC_CONFIG_FILE="+services.OperatorOIDCConfigFile)
		}
		return env
	}
	if mode != "control-service" {
		return nil
	}
	var env []string
	if services.OperatorDelegationTrustFile != "" {
		env = append(env, "ZPR_OPERATOR_DELEGATION_TRUST_FILE="+services.OperatorDelegationTrustFile)
	}
	if binding := services.PolicyService; binding != nil {
		env = append(env, "ZPR_POLICY_SERVICE_URL="+binding.URL, "ZPR_POLICY_CLIENT_CERT_FILE="+binding.ClientCertificateFile,
			"ZPR_POLICY_CLIENT_KEY_FILE="+binding.ClientKeyFile, "ZPR_POLICY_SERVICE_CA_FILE="+binding.CAFile)
	}
	if binding := services.EnrollmentService; binding != nil {
		env = append(env, "ZPR_ENROLLMENT_SERVICE_URL="+binding.URL, "ZPR_ENROLLMENT_CLIENT_CERT_FILE="+binding.ClientCertificateFile,
			"ZPR_ENROLLMENT_CLIENT_KEY_FILE="+binding.ClientKeyFile, "ZPR_ENROLLMENT_SERVICE_CA_FILE="+binding.CAFile,
			"ZPR_ENROLLMENT_DELEGATION_AUDIENCE="+services.enrollmentDelegationAudience)
	}
	return env
}

func decodeEvaluationDelegationTrust(data []byte, config *operatordelegation.Config) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(config); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return errors.New("operator delegation trust requires one JSON object")
	}
	return config.Validate()
}

func evaluationOperatorDataPlaneArguments(state string, inventory evaluationInventory, image string, installPolicy bool, services *evaluationOperatorServices) []string {
	args := evaluationDataPlaneArguments(state, inventory, image, installPolicy)
	if services == nil || services.VisaServiceConfigFile == "" {
		return args
	}
	args = args[:len(args)-1]
	args = append(args, "-e", "ZPR_DP_VS_CONFIG="+services.VisaServiceConfigFile)
	for _, path := range append([]string{services.VisaServiceConfigFile}, services.VisaServiceReadOnlyFiles...) {
		args = append(args, "--mount", "type=bind,source="+path+",target="+path+",readonly")
	}
	return append(args, image)
}
