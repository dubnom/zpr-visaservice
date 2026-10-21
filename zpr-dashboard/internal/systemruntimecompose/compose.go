package systemruntimecompose

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"

	"neboagency.com/zpr-dashborad/internal/operatorauth"
	"neboagency.com/zpr-dashborad/internal/systemconfig"
)

const (
	ControlRoomServiceName    = "control-room"
	ControlServiceServiceName = "control-service"
	DashboardExecutable       = "/usr/local/bin/zpr-web-dashboard"
	MaxRuntimeBytes           = 1 << 20
)

var pinnedImageReference = regexp.MustCompile(
	`^[A-Za-z0-9][A-Za-z0-9._/:+-]{0,254}@sha256:[0-9a-f]{64}$`,
)

var localImageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Plan struct {
	Services map[string]Service `json:"services"`
}

type Service struct {
	Image       string            `json:"image"`
	PullPolicy  string            `json:"pull_policy,omitempty"`
	NetworkMode string            `json:"network_mode"`
	Entrypoint  []string          `json:"entrypoint"`
	Command     []string          `json:"command"`
	Environment map[string]string `json:"environment"`
	Volumes     []Volume          `json:"volumes"`
}

type Volume struct {
	Type     string      `json:"type"`
	Source   string      `json:"source"`
	Target   string      `json:"target"`
	ReadOnly bool        `json:"read_only"`
	Bind     BindOptions `json:"bind"`
}

type BindOptions struct {
	CreateHostPath bool `json:"create_host_path"`
}

func ReadRuntime(reader io.Reader) (systemconfig.ControlRoomRuntime, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxRuntimeBytes+1))
	if err != nil {
		return systemconfig.ControlRoomRuntime{}, errors.New("unable to read Control Room runtime inputs")
	}
	if len(data) > MaxRuntimeBytes {
		return systemconfig.ControlRoomRuntime{}, errors.New("Control Room runtime inputs exceed 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var runtime systemconfig.ControlRoomRuntime
	if err := decoder.Decode(&runtime); err != nil {
		return systemconfig.ControlRoomRuntime{}, errors.New("Control Room runtime inputs are invalid or contain unknown fields")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return systemconfig.ControlRoomRuntime{}, errors.New("Control Room runtime inputs must contain exactly one JSON document")
	}
	return runtime, nil
}

func Build(runtime systemconfig.ControlRoomRuntime, image, oidcSource string) (Plan, []byte, error) {
	pullPolicy := ""
	if localImageID.MatchString(image) {
		pullPolicy = "never"
	} else if !pinnedImageReference.MatchString(image) {
		return Plan{}, nil, errors.New("image must be an immutable repository@sha256:<64 lowercase hex> reference or local sha256:<64 lowercase hex> image ID")
	}
	if !canonicalAbsolutePath(oidcSource) {
		return Plan{}, nil, errors.New("OIDC output path must be a canonical absolute file path")
	}
	if err := validateRuntime(runtime); err != nil {
		return Plan{}, nil, err
	}

	oidcConfig, err := validateOIDCConfig(runtime.OIDCConfig)
	if err != nil {
		return Plan{}, nil, err
	}
	controlService := runtime.ControlService
	controlServiceVolumes, err := toVolumes(controlService.FileMounts)
	if err != nil {
		return Plan{}, nil, fmt.Errorf("invalid Control-Service mounts: %w", err)
	}
	controlRoomVolumes, err := toVolumes(runtime.FileMounts)
	if err != nil {
		return Plan{}, nil, fmt.Errorf("invalid Control Room mounts: %w", err)
	}
	controlRoomVolumes = append(controlRoomVolumes, Volume{
		Type:     "bind",
		Source:   oidcSource,
		Target:   runtime.OIDCConfigPath,
		ReadOnly: true,
		Bind:     BindOptions{CreateHostPath: false},
	})
	for _, volume := range controlRoomVolumes[:len(controlRoomVolumes)-1] {
		if volume.Target == runtime.OIDCConfigPath || volume.Source == oidcSource {
			return Plan{}, nil, errors.New("OIDC output path conflicts with a configured Control Room mount")
		}
	}
	slices.SortFunc(controlRoomVolumes, func(left, right Volume) int {
		return strings.Compare(left.Target, right.Target)
	})

	plan := Plan{
		Services: map[string]Service{
			ControlServiceServiceName: {
				Image:       image,
				PullPolicy:  pullPolicy,
				NetworkMode: "host",
				Entrypoint:  []string{DashboardExecutable},
				Command:     []string{"-mode", "control-service"},
				Environment: cloneEnvironment(controlService.Environment),
				Volumes:     controlServiceVolumes,
			},
			ControlRoomServiceName: {
				Image:       image,
				PullPolicy:  pullPolicy,
				NetworkMode: "host",
				Entrypoint:  []string{DashboardExecutable},
				Command: []string{
					"-mode", "control-room", "-listen", runtime.ListenAddress,
				},
				Environment: cloneEnvironment(runtime.Environment),
				Volumes:     controlRoomVolumes,
			},
		},
	}
	return plan, append(oidcConfig, '\n'), nil
}

func validateRuntime(runtime systemconfig.ControlRoomRuntime) error {
	if runtime.ControlService == nil {
		return errors.New("runtime inputs do not include the Control-Service role")
	}
	if !privateListenerMatchesManagementIP(
		runtime.ListenAddress,
		runtime.Environment["ZPR_CONTROL_ROOM_MANAGEMENT_IP"],
		runtime.Environment["ZPR_CONTROL_ROOM_ORIGIN"],
	) {
		return errors.New("Control Room listener must preserve its configured private management IP and origin port")
	}
	if !privateListenerMatchesServiceURL(
		runtime.ControlService.ListenAddress,
		runtime.ControlService.Environment["ZPR_CONTROL_SERVICE_LISTEN"],
		runtime.Environment["ZPR_CONTROL_SERVICE_URL"],
	) {
		return errors.New("Control-Service listener and Control-Service HTTPS URL are inconsistent or not private")
	}
	if runtime.OIDCConfigMode != 0o600 ||
		!canonicalAbsolutePath(runtime.OIDCConfigPath) ||
		runtime.Environment["ZPR_OPERATOR_OIDC_CONFIG_FILE"] != runtime.OIDCConfigPath {
		return errors.New("runtime inputs must define the OIDC configuration at its canonical path with mode 0600")
	}
	if err := requireEnvironment(runtime.Environment, []string{
		"ZPR_CONTROL_ROOM_CERT_FILE",
		"ZPR_CONTROL_ROOM_KEY_FILE",
		"ZPR_CONTROL_ROOM_ORIGIN",
		"ZPR_CONTROL_ROOM_MANAGEMENT_IP",
		"ZPR_CONTROL_ROOM_ORGANIZATION_ID",
		"ZPR_OPERATOR_OIDC_CONFIG_FILE",
		"ZPR_OPERATOR_OIDC_SECRET_FILE",
		"ZPR_CONTROL_SERVICE_URL",
		"ZPR_CONTROL_CLIENT_CERT_FILE",
		"ZPR_CONTROL_CLIENT_KEY_FILE",
		"ZPR_CONTROL_SERVICE_CA_FILE",
		"ZPR_OPERATOR_DELEGATION_SIGNER_FILE",
		"ZPR_OPERATOR_DELEGATION_KEY_FILE",
		"ZPR_OPERATOR_DELEGATION_AUDIENCE",
	}); err != nil {
		return fmt.Errorf("invalid Control Room environment: %w", err)
	}
	if runtime.Environment["ZPR_OPERATOR_DELEGATION_AUDIENCE"] !=
		runtime.Environment["ZPR_CONTROL_SERVICE_URL"] {
		return errors.New("Control Room delegation audience must match the configured Control-Service URL")
	}
	if err := requireEnvironment(runtime.ControlService.Environment, []string{
		"ZPR_CONTROL_SERVICE_LISTEN",
		"ZPR_CONTROL_SERVICE_CERT_FILE",
		"ZPR_CONTROL_SERVICE_KEY_FILE",
		"ZPR_CONTROL_SERVICE_CLIENT_CA_FILE",
		"ZPR_BUDGET_AUTHORITY_DB_PATH",
		"ZPR_OPERATOR_DELEGATION_TRUST_FILE",
	}); err != nil {
		return fmt.Errorf("invalid Control-Service environment: %w", err)
	}
	databasePath := runtime.ControlService.Environment["ZPR_BUDGET_AUTHORITY_DB_PATH"]
	if !canonicalAbsolutePath(databasePath) || path.Dir(databasePath) == "/" {
		return errors.New("budget authority database path must be below a dedicated absolute directory")
	}
	for _, environment := range []map[string]string{
		runtime.Environment,
		runtime.ControlService.Environment,
	} {
		for name := range environment {
			upperName := strings.ToUpper(name)
			if strings.Contains(upperName, "SIMULATION") || strings.Contains(upperName, "SIMULATOR") {
				return errors.New("production runtime inputs must not contain Simulator settings")
			}
		}
	}
	if err := requireMountedFiles(runtime.FileMounts, runtime.Environment, []string{
		"ZPR_CONTROL_ROOM_CERT_FILE",
		"ZPR_CONTROL_ROOM_KEY_FILE",
		"ZPR_OPERATOR_OIDC_SECRET_FILE",
		"ZPR_CONTROL_CLIENT_CERT_FILE",
		"ZPR_CONTROL_CLIENT_KEY_FILE",
		"ZPR_CONTROL_SERVICE_CA_FILE",
		"ZPR_OPERATOR_DELEGATION_SIGNER_FILE",
		"ZPR_OPERATOR_DELEGATION_KEY_FILE",
		"ZPR_OPERATOR_OIDC_CA_FILE",
	}); err != nil {
		return fmt.Errorf("invalid Control Room mounts: %w", err)
	}
	if err := requireMountedFiles(runtime.ControlService.FileMounts, runtime.ControlService.Environment, []string{
		"ZPR_CONTROL_SERVICE_CERT_FILE",
		"ZPR_CONTROL_SERVICE_KEY_FILE",
		"ZPR_CONTROL_SERVICE_CLIENT_CA_FILE",
		"ZPR_OPERATOR_DELEGATION_TRUST_FILE",
	}); err != nil {
		return fmt.Errorf("invalid Control-Service mounts: %w", err)
	}
	if err := validateMounts(
		runtime.ControlService.FileMounts,
		databasePath,
		true,
	); err != nil {
		return fmt.Errorf("invalid Control-Service mounts: %w", err)
	}
	if err := validateMounts(runtime.FileMounts, "", false); err != nil {
		return fmt.Errorf("invalid Control Room mounts: %w", err)
	}
	return nil
}

func requireMountedFiles(mounts []systemconfig.RuntimeFileMount, environment map[string]string, names []string) error {
	mounted := make(map[string]bool, len(mounts))
	for _, mount := range mounts {
		mounted[mount.Destination] = true
	}
	for _, name := range names {
		value := environment[name]
		if value == "" {
			continue
		}
		if !canonicalAbsolutePath(value) || !mounted[value] {
			return fmt.Errorf("%s must reference a configured read-only file mount", name)
		}
	}
	return nil
}

func validateOIDCConfig(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return nil, errors.New("runtime inputs do not contain operator OIDC configuration")
	}
	var config operatorauth.Config
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, errors.New("embedded operator OIDC configuration is invalid or contains unknown fields")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("embedded operator OIDC configuration must contain one JSON document")
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("embedded operator OIDC configuration is invalid: %w", err)
	}
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, errors.New("cannot encode operator OIDC configuration")
	}
	return encoded, nil
}

func validateMounts(mounts []systemconfig.RuntimeFileMount, databasePath string, allowDatabaseParent bool) error {
	if len(mounts) == 0 {
		return errors.New("at least one file mount is required")
	}
	seen := make(map[string]bool, len(mounts))
	writable := 0
	for _, mount := range mounts {
		if !canonicalAbsolutePath(mount.Source) ||
			!canonicalAbsolutePath(mount.Destination) ||
			mount.Source != mount.Destination ||
			seen[mount.Destination] {
			return errors.New("mount paths must be distinct canonical absolute paths with identical source and destination")
		}
		seen[mount.Destination] = true
		if !mount.ReadOnly {
			writable++
			if !allowDatabaseParent || mount.Destination != path.Dir(databasePath) {
				return errors.New("the only writable mount allowed is the configured database parent directory")
			}
		}
	}
	if allowDatabaseParent && writable != 1 {
		return errors.New("Control-Service requires exactly one writable database-parent mount")
	}
	if !allowDatabaseParent && writable != 0 {
		return errors.New("Control Room file mounts must all be read-only")
	}
	return nil
}

func toVolumes(mounts []systemconfig.RuntimeFileMount) ([]Volume, error) {
	volumes := make([]Volume, 0, len(mounts))
	for _, mount := range mounts {
		if !canonicalAbsolutePath(mount.Source) ||
			!canonicalAbsolutePath(mount.Destination) ||
			mount.Source != mount.Destination {
			return nil, errors.New("mount paths must be canonical absolute paths with identical source and destination")
		}
		volumes = append(volumes, Volume{
			Type:     "bind",
			Source:   mount.Source,
			Target:   mount.Destination,
			ReadOnly: mount.ReadOnly,
			Bind:     BindOptions{CreateHostPath: false},
		})
	}
	slices.SortFunc(volumes, func(left, right Volume) int {
		return strings.Compare(left.Target, right.Target)
	})
	return volumes, nil
}

func requireEnvironment(environment map[string]string, names []string) error {
	for _, name := range names {
		if strings.TrimSpace(environment[name]) == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	return nil
}

func cloneEnvironment(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for name, value := range source {
		clone[name] = value
	}
	return clone
}

func privateListenerMatchesManagementIP(address, managementIP, origin string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host != managementIP || !privateIP(host) {
		return false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" ||
		origin != "https://"+parsed.Host {
		return false
	}
	return port == effectivePort(parsed)
}

func privateListenerMatchesServiceURL(address, configuredAddress, serviceURL string) bool {
	if address != configuredAddress {
		return false
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || !privateIP(host) {
		return false
	}
	parsed, err := url.Parse(serviceURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" ||
		parsed.ForceQuery || parsed.Fragment != "" || parsed.Opaque != "" ||
		serviceURL != "https://"+parsed.Host {
		return false
	}
	return port == effectivePort(parsed)
}

func effectivePort(parsed *url.URL) string {
	if port := parsed.Port(); port != "" {
		return port
	}
	return "443"
}

func privateIP(value string) bool {
	ip := net.ParseIP(value)
	return ip != nil && ip.String() == value && ip.IsPrivate() &&
		!ip.IsUnspecified() && !ip.IsMulticast()
}

func canonicalAbsolutePath(value string) bool {
	return value != "" && len(value) <= 4096 && path.IsAbs(value) &&
		path.Clean(value) == value && value != "/" &&
		!strings.ContainsAny(value, "\\\x00\r\n")
}

func ValidPinnedImageReference(image string) bool {
	return pinnedImageReference.MatchString(image)
}
