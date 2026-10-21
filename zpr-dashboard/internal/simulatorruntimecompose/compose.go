package simulatorruntimecompose

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

	"neboagency.com/zpr-dashborad/internal/budgetauthority"
	"neboagency.com/zpr-dashborad/internal/simulatorbudgetconfig"
)

const MaxRuntimeBytes = 1 << 20

var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
var dnsNamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*\.?$`)

type Fragment struct {
	Services map[string]Service `json:"services"`
}

type Service struct {
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

func ReadRuntime(reader io.Reader) (simulatorbudgetconfig.Runtime, error) {
	data, err := io.ReadAll(io.LimitReader(reader, MaxRuntimeBytes+1))
	if err != nil {
		return simulatorbudgetconfig.Runtime{}, errors.New("unable to read Simulator runtime inputs")
	}
	if len(data) > MaxRuntimeBytes {
		return simulatorbudgetconfig.Runtime{}, errors.New("Simulator runtime inputs exceed 1 MiB")
	}
	if err := validateJSONShape(data, 16); err != nil {
		return simulatorbudgetconfig.Runtime{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var runtime simulatorbudgetconfig.Runtime
	if err := decoder.Decode(&runtime); err != nil {
		return simulatorbudgetconfig.Runtime{}, errors.New("Simulator runtime inputs are invalid or contain unknown fields")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return simulatorbudgetconfig.Runtime{}, errors.New("Simulator runtime inputs must contain exactly one JSON document")
	}
	return runtime, nil
}

func Build(runtime simulatorbudgetconfig.Runtime, serviceName string) (Fragment, error) {
	if !serviceNamePattern.MatchString(serviceName) ||
		strings.EqualFold(serviceName, "control-room") ||
		strings.EqualFold(serviceName, "control-service") {
		return Fragment{}, errors.New("Compose service name must be a nonempty simple service identifier")
	}
	if err := validateRuntime(runtime); err != nil {
		return Fragment{}, err
	}
	environment := make(map[string]string, len(runtime.Environment))
	for name, value := range runtime.Environment {
		environment[name] = value
	}
	volumes := make([]Volume, 0, len(runtime.FileMounts))
	for _, mount := range runtime.FileMounts {
		volumes = append(volumes, Volume{
			Type:     "bind",
			Source:   mount.Source,
			Target:   mount.Destination,
			ReadOnly: true,
			Bind:     BindOptions{CreateHostPath: false},
		})
	}
	slices.SortFunc(volumes, func(left, right Volume) int {
		return strings.Compare(left.Target, right.Target)
	})
	return Fragment{
		Services: map[string]Service{
			serviceName: {Environment: environment, Volumes: volumes},
		},
	}, nil
}

func validateRuntime(runtime simulatorbudgetconfig.Runtime) error {
	if runtime.Environment == nil || runtime.FileMounts == nil {
		return errors.New("Simulator runtime inputs require environment and file_mounts")
	}
	allowed := map[string]bool{
		budgetauthority.ResolverURLEnv:            true,
		budgetauthority.ResolverClientCertFileEnv: true,
		budgetauthority.ResolverClientKeyFileEnv:  true,
		budgetauthority.ResolverCAFileEnv:         true,
		budgetauthority.ResolverTLSServerNameEnv:  true,
	}
	required := []string{
		budgetauthority.ResolverURLEnv,
		budgetauthority.ResolverClientCertFileEnv,
		budgetauthority.ResolverClientKeyFileEnv,
		budgetauthority.ResolverCAFileEnv,
	}
	for name := range runtime.Environment {
		if !allowed[name] {
			return fmt.Errorf("unsupported Simulator runtime environment variable %q", name)
		}
	}
	for _, name := range required {
		if strings.TrimSpace(runtime.Environment[name]) == "" {
			return fmt.Errorf("required Simulator runtime environment variable %s is missing", name)
		}
	}
	if err := validateAuthorityURL(runtime.Environment[budgetauthority.ResolverURLEnv]); err != nil {
		return err
	}
	if serverName, present := runtime.Environment[budgetauthority.ResolverTLSServerNameEnv]; present &&
		!validServerName(serverName) {
		return errors.New("Simulator TLS server name must be a nonempty DNS name or canonical IP literal")
	}
	if len(runtime.FileMounts) != 3 {
		return errors.New("Simulator runtime inputs must contain exactly three TLS file mounts")
	}
	envFiles := []string{
		budgetauthority.ResolverClientCertFileEnv,
		budgetauthority.ResolverClientKeyFileEnv,
		budgetauthority.ResolverCAFileEnv,
	}
	mounted := make(map[string]bool, len(runtime.FileMounts))
	used := make(map[string]bool, len(envFiles))
	for _, mount := range runtime.FileMounts {
		if !canonicalAbsolutePath(mount.Source) ||
			!canonicalAbsolutePath(mount.Destination) ||
			mount.Source != mount.Destination ||
			!mount.ReadOnly || mounted[mount.Destination] {
			return errors.New("TLS mounts must be distinct canonical absolute same-path read-only files")
		}
		mounted[mount.Destination] = true
	}
	for _, name := range envFiles {
		file := runtime.Environment[name]
		if !canonicalAbsolutePath(file) || !mounted[file] || used[file] {
			return fmt.Errorf("%s must match a distinct read-only TLS mount", name)
		}
		used[file] = true
	}
	return nil
}

func validateAuthorityURL(value string) error {
	origin, err := url.Parse(value)
	if err != nil || origin.Scheme != "https" || origin.Host == "" ||
		origin.User != nil || (origin.Path != "" && origin.Path != "/") ||
		origin.RawQuery != "" || origin.ForceQuery || origin.Fragment != "" ||
		origin.Opaque != "" ||
		(value != "https://"+origin.Host && value != "https://"+origin.Host+"/") {
		return errors.New("budget authority URL must be an exact HTTPS origin")
	}
	return nil
}

func validServerName(value string) bool {
	if strings.ContainsAny(value, " /\\:@?#[]\x00\r\n") {
		return false
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String() == value
	}
	return len(value) <= 253 && dnsNamePattern.MatchString(value)
}

func canonicalAbsolutePath(value string) bool {
	return value != "" && len(value) <= 4096 && path.IsAbs(value) &&
		path.Clean(value) == value && value != "/" &&
		!strings.ContainsAny(value, `\$`) &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func validateJSONShape(data []byte, maxDepth int) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSONValue(decoder, 0, maxDepth, false); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("Simulator runtime inputs must contain exactly one JSON document")
	}
	return nil
}

func checkJSONValue(decoder *json.Decoder, depth, maxDepth int, allowUppercaseKeys bool) error {
	if depth > maxDepth {
		return errors.New("Simulator runtime inputs exceed maximum JSON nesting")
	}
	token, err := decoder.Token()
	if err != nil || token == nil {
		return errors.New("Simulator runtime inputs are not valid JSON")
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return errors.New("Simulator runtime inputs are not valid JSON")
			}
			name, ok := key.(string)
			if !ok || (!allowUppercaseKeys && name != strings.ToLower(name)) || seen[name] {
				return errors.New("Simulator runtime inputs contain an invalid or duplicate object key")
			}
			seen[name] = true
			if err := checkJSONValue(decoder, depth+1, maxDepth, allowUppercaseKeys || name == "environment"); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1, maxDepth, allowUppercaseKeys); err != nil {
				return err
			}
		}
	default:
		return errors.New("Simulator runtime inputs are not valid JSON")
	}
	if _, err := decoder.Token(); err != nil {
		return errors.New("Simulator runtime inputs are not valid JSON")
	}
	return nil
}
