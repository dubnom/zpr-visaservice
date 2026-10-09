package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"unicode"
)

const (
	gatewayInstanceSchemaVersion = 1
	maxGatewayConfigBytes        = 64 << 10
	maxGatewayDestinations       = 32
	maxGatewayPathPrefixes       = 32
	minGatewayTimeoutMS          = 100
	maxGatewayTimeoutMS          = 30_000
)

type gatewayInstanceContract struct {
	OrganizationID  string `json:"organization_id"`
	InstanceID      string `json:"instance_id"`
	AdapterCN       string `json:"adapter_cn"`
	ServiceName     string `json:"service_name"`
	ExternalNetwork string `json:"external_network"`
}

type gatewayInstanceConfig struct {
	SchemaVersion    int                        `json:"schema_version"`
	OrganizationID   string                     `json:"organization_id"`
	InstanceID       string                     `json:"instance_id"`
	AdapterCN        string                     `json:"adapter_cn"`
	ServiceName      string                     `json:"service_name"`
	Destinations     []gatewayDestinationConfig `json:"destinations"`
	Methods          []string                   `json:"methods"`
	TimeoutMS        int                        `json:"timeout_ms"`
	MaxResponseBytes int64                      `json:"max_response_bytes"`
}

type gatewayDestinationConfig struct {
	Origin       string   `json:"origin"`
	PathPrefixes []string `json:"path_prefixes"`
}

type gatewaySourceError struct {
	message string
	path    []any
}

func (err *gatewaySourceError) Error() string { return err.message }

func gatewayFieldError(message string, path ...any) error {
	return &gatewaySourceError{message: message, path: path}
}

func gatewayDiagnosticLine(data []byte, err error) int {
	var sourceError *gatewaySourceError
	if !errors.As(err, &sourceError) {
		return 0
	}
	offset, found := gatewayJSONFieldOffset(data, sourceError.path)
	if !found {
		return 0
	}
	return bytes.Count(data[:offset], []byte("\n")) + 1
}

func gatewayJSONFieldOffset(data []byte, path []any) (int, bool) {
	if len(path) == 0 {
		return 0, true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return 0, false
	}
	resultOffset, resultFound := 0, false
	for index := 0; decoder.More(); index++ {
		matches := false
		switch token {
		case json.Delim('{'):
			key, err := decoder.Token()
			if err != nil {
				return 0, false
			}
			name, isKey := key.(string)
			field, isField := path[0].(string)
			matches = isKey && isField && strings.EqualFold(name, field)
		case json.Delim('['):
			matches = index == path[0]
		default:
			return 0, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return 0, false
		}
		if matches {
			start := int(decoder.InputOffset()) - len(value)
			offset, found := gatewayJSONFieldOffset(value, path[1:])
			resultOffset, resultFound = start+offset, found
		}
	}
	return resultOffset, resultFound
}

func parseGatewayInstanceConfig(data []byte, contract gatewayInstanceContract) (gatewayInstanceConfig, error) {
	var config gatewayInstanceConfig
	if len(data) == 0 || len(data) > maxGatewayConfigBytes {
		return config, errors.New("gateway configuration must be between 1 byte and 64 KiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, fmt.Errorf("decode gateway configuration: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return config, errors.New("gateway configuration must contain exactly one JSON object")
	}
	if err := validateGatewayContract(contract); err != nil {
		return config, err
	}
	if config.SchemaVersion != gatewayInstanceSchemaVersion {
		return config, gatewayFieldError(fmt.Sprintf("unsupported gateway schema version %d", config.SchemaVersion), "schema_version")
	}
	if config.OrganizationID != contract.OrganizationID || config.InstanceID != contract.InstanceID || config.AdapterCN != contract.AdapterCN || config.ServiceName != contract.ServiceName {
		return config, errors.New("gateway configuration identity does not match its declared ZPL contract")
	}
	if len(config.Destinations) == 0 || len(config.Destinations) > maxGatewayDestinations {
		return config, gatewayFieldError(fmt.Sprintf("gateway configuration requires 1 to %d destinations", maxGatewayDestinations), "destinations")
	}
	if len(config.Methods) == 0 || len(config.Methods) > 2 {
		return config, gatewayFieldError("gateway methods must contain GET, HEAD, or both", "methods")
	}
	seenMethods := make(map[string]bool, len(config.Methods))
	for _, method := range config.Methods {
		if method != "GET" && method != "HEAD" || seenMethods[method] {
			return config, gatewayFieldError("gateway methods must contain unique GET and/or HEAD values", "methods")
		}
		seenMethods[method] = true
	}
	if config.TimeoutMS < minGatewayTimeoutMS || config.TimeoutMS > maxGatewayTimeoutMS {
		return config, gatewayFieldError(fmt.Sprintf("gateway timeout must be between %d and %d milliseconds", minGatewayTimeoutMS, maxGatewayTimeoutMS), "timeout_ms")
	}
	if config.MaxResponseBytes < 1 || config.MaxResponseBytes > maxGatewayResponseBytes {
		return config, gatewayFieldError(fmt.Sprintf("gateway response limit must be between 1 and %d bytes", maxGatewayResponseBytes), "max_response_bytes")
	}

	seenOrigins := make(map[string]bool, len(config.Destinations))
	for index, destination := range config.Destinations {
		if strings.TrimSpace(destination.Origin) == "" {
			return config, gatewayFieldError(fmt.Sprintf("destination %d requires an HTTPS origin, for example https://api.example.com; set allowed paths in path_prefixes", index+1), "destinations", index, "origin")
		}
		upstream, err := parseGatewayUpstream(destination.Origin)
		if err != nil {
			return config, gatewayFieldError(fmt.Sprintf("invalid gateway destination: %v", err), "destinations", index, "origin")
		}
		if !validGatewayFQDN(upstream.Hostname()) {
			return config, gatewayFieldError("gateway destination must use a fully qualified DNS hostname", "destinations", index, "origin")
		}
		if upstream.Path != "" && upstream.Path != "/" || upstream.Port() != "" && upstream.Port() != "443" {
			return config, gatewayFieldError("gateway destination must be an HTTPS origin on port 443 without a path", "destinations", index, "origin")
		}
		origin := strings.ToLower(upstream.Hostname())
		if seenOrigins[origin] {
			return config, gatewayFieldError(fmt.Sprintf("duplicate gateway destination %q", origin), "destinations", index, "origin")
		}
		seenOrigins[origin] = true
		if len(destination.PathPrefixes) == 0 || len(destination.PathPrefixes) > maxGatewayPathPrefixes {
			return config, gatewayFieldError(fmt.Sprintf("each gateway destination requires 1 to %d path prefixes", maxGatewayPathPrefixes), "destinations", index, "path_prefixes")
		}
		seenPrefixes := make(map[string]bool)
		for prefixIndex, prefix := range destination.PathPrefixes {
			if err := validateGatewayPathPrefix(prefix); err != nil {
				return config, gatewayFieldError(err.Error(), "destinations", index, "path_prefixes", prefixIndex)
			}
			if seenPrefixes[prefix] {
				return config, gatewayFieldError(fmt.Sprintf("duplicate gateway path prefix %q", prefix), "destinations", index, "path_prefixes", prefixIndex)
			}
			seenPrefixes[prefix] = true
		}
	}
	return config, nil
}

func validGatewayFQDN(hostname string) bool {
	hostname = strings.TrimSuffix(strings.ToLower(hostname), ".")
	if len(hostname) > 253 || !strings.Contains(hostname, ".") {
		return false
	}
	for _, label := range strings.Split(hostname, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func validateGatewayContract(contract gatewayInstanceContract) error {
	for label, value := range map[string]string{
		"organization ID":     contract.OrganizationID,
		"gateway instance ID": contract.InstanceID,
		"adapter CN":          contract.AdapterCN,
	} {
		if !validGatewayIdentifier(value) {
			return fmt.Errorf("declared gateway %s is invalid", label)
		}
	}
	if contract.ExternalNetwork != "" && !validGatewayIdentifier(contract.ExternalNetwork) {
		return errors.New("declared gateway external-network metadata is invalid")
	}
	if !strings.HasSuffix(contract.ServiceName, ".svc.zpr") || !validGatewayIdentifier(strings.TrimSuffix(contract.ServiceName, ".svc.zpr")) {
		return errors.New("declared gateway service must be a valid name under svc.zpr")
	}
	return nil
}

func validGatewayIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 63 || value[0] < 'a' || value[0] > 'z' || value[len(value)-1] == '-' {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return false
		}
	}
	return true
}

func validateGatewayPathPrefix(prefix string) error {
	if prefix == "" || !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#\\") {
		return fmt.Errorf("gateway path prefix %q must be an absolute path without query, fragment, or backslash", prefix)
	}
	decoded, err := url.PathUnescape(prefix)
	if err != nil || !strings.HasPrefix(decoded, "/") {
		return fmt.Errorf("gateway path prefix %q is invalid", prefix)
	}
	for _, character := range decoded {
		if unicode.IsControl(character) {
			return fmt.Errorf("gateway path prefix %q contains a control character", prefix)
		}
	}
	clean := path.Clean(decoded)
	if decoded != clean && decoded != clean+"/" {
		return fmt.Errorf("gateway path prefix %q is not canonical", prefix)
	}
	return nil
}
