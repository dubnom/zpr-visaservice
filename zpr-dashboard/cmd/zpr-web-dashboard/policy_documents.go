package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
)

// Provisional envelope: all documents belong to one saved/reviewed policy
// revision. There are no external includes or unreviewed catalog dependencies.
type policyDocuments struct {
	Documents []policyDocument `json:"documents"`
}

type policyDocument struct {
	ID             string                              `json:"id"`
	Source         string                              `json:"source"`
	TrustedSources map[string]policyTrustedDeclaration `json:"trusted_sources"`
}

type policyTrustedDeclaration struct {
	API               string   `json:"api" toml:"api"`
	ExpirationSeconds uint32   `json:"expiration_seconds" toml:"expiration_seconds"`
	ReturnsAttributes []string `json:"returns_attributes" toml:"returns_attributes"`
}

func policyObjectFields(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for name := range fields {
		if !slices.Contains(allowed, name) {
			return nil, fmt.Errorf("unknown or noncanonical policy JSON field %q", name)
		}
	}
	return fields, nil
}

func validatePolicyDocumentFields(data []byte) error {
	root, err := policyObjectFields(data, "documents")
	if err != nil {
		return err
	}
	var documents []json.RawMessage
	if err := json.Unmarshal(root["documents"], &documents); err != nil {
		return err
	}
	for _, raw := range documents {
		document, err := policyObjectFields(raw, "id", "source", "trusted_sources")
		if err != nil {
			return err
		}
		if rawSources, exists := document["trusted_sources"]; exists {
			var sources map[string]json.RawMessage
			if err := json.Unmarshal(rawSources, &sources); err != nil {
				return err
			}
			for _, declaration := range sources {
				if _, err := policyObjectFields(declaration, "api", "expiration_seconds", "returns_attributes"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func isPolicyDocuments(source string) bool {
	return strings.HasPrefix(strings.TrimSpace(source), "{")
}

func validateDocumentJSON(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("policy JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("null is not permitted in policy documents")
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			name, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := name.(string)
			if !ok || seen[key] {
				return errors.New("duplicate policy JSON property")
			}
			seen[key] = true
			if err := validateDocumentJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case json.Delim('['):
		for decoder.More() {
			if err := validateDocumentJSON(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

func decodePolicyDocuments(source string) (policyDocuments, error) {
	var result policyDocuments
	if len(source) > maxPolicySourceBytes || !utf8.ValidString(source) {
		return result, errors.New("policy documents exceed 1 MiB or contain invalid UTF-8")
	}
	decoder := json.NewDecoder(strings.NewReader(source))
	if err := validateDocumentJSON(decoder, 0); err != nil {
		return result, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return result, errors.New("trailing policy JSON data")
	}
	if err := validatePolicyDocumentFields([]byte(source)); err != nil {
		return result, err
	}
	if err := decodeOperatorConfig([]byte(source), &result); err != nil {
		return result, err
	}
	if len(result.Documents) == 0 || len(result.Documents) > 64 {
		return result, errors.New("policy envelope requires 1 to 64 documents")
	}
	seen := map[string]bool{}
	declarations := map[string]policyTrustedDeclaration{}
	for _, document := range result.Documents {
		if !runtimeSourceID.MatchString(document.ID) || seen[document.ID] {
			return result, errors.New("policy document IDs must be unique ASCII identifiers")
		}
		seen[document.ID] = true
		if len(document.TrustedSources) > 16 {
			return result, errors.New("at most 16 trusted sources per document")
		}
		for id, declaration := range document.TrustedSources {
			if !runtimeSourceID.MatchString(id) || strings.Contains(id, "__source__") || id == "default" {
				return result, fmt.Errorf("document %q has invalid trusted source ID %q", document.ID, id)
			}
			if declaration.API != "file" && declaration.API != "rest/1" {
				return result, fmt.Errorf("document %q source %q requires file or rest/1 API", document.ID, id)
			}
			if len(declaration.ReturnsAttributes) == 0 {
				return result, fmt.Errorf("document %q source %q requires return mappings", document.ID, id)
			}
			if previous, exists := declarations[id]; exists && !samePolicyDeclaration(previous, declaration) {
				return result, fmt.Errorf("conflicting trusted source %q in document %q", id, document.ID)
			}
			declarations[id] = declaration
		}
	}
	if len(declarations) > 16 {
		return result, errors.New("at most 16 effective trusted sources")
	}
	return result, nil
}

func samePolicyDeclaration(left, right any) bool {
	a, errA := json.Marshal(left)
	b, errB := json.Marshal(right)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

func policyDocumentsSource(envelope policyDocuments) string {
	var composed strings.Builder
	for _, document := range envelope.Documents {
		fmt.Fprintf(&composed, "# Policy document: %s\n%s\n", document.ID, document.Source)
	}
	return composed.String()
}

// Policy declarations can add a source or repeat an identical configured source;
// neither policy nor base configuration wins a conflict by ordering.
func preparePolicyDocuments(source, configPath, directory string) (string, string, error) {
	if !isPolicyDocuments(source) {
		return source, configPath, nil
	}
	base, err := os.ReadFile(configPath)
	if err != nil {
		return "", "", err
	}
	return preparePolicyDocumentsFromBase(source, configPath, directory, base)
}

func preparePolicyDocumentsFromBase(source, configPath, directory string, baseContents []byte) (string, string, error) {
	if !isPolicyDocuments(source) {
		return source, configPath, nil
	}
	envelope, err := decodePolicyDocuments(source)
	if err != nil {
		return "", "", err
	}
	config := make(map[string]any)
	if _, err := toml.Decode(string(baseContents), &config); err != nil {
		return "", "", err
	}
	sources, ok := config["trusted_services"].(map[string]any)
	if !ok {
		if _, exists := config["trusted_services"]; exists {
			return "", "", errors.New("invalid trusted_services configuration")
		}
		sources = map[string]any{}
		config["trusted_services"] = sources
	}
	for _, document := range envelope.Documents {
		ids := make([]string, 0, len(document.TrustedSources))
		for id := range document.TrustedSources {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			declaration := document.TrustedSources[id]
			var encoded bytes.Buffer
			if err := toml.NewEncoder(&encoded).Encode(declaration); err != nil {
				return "", "", err
			}
			var table map[string]any
			if _, err := toml.Decode(encoded.String(), &table); err != nil {
				return "", "", err
			}
			if previous, exists := sources[id]; exists && !samePolicyDeclaration(previous, table) {
				return "", "", fmt.Errorf("trusted source %q conflicts with another document or base configuration", id)
			}
			sources[id] = table
		}
	}
	sourceCount := len(sources)
	if _, builtin := sources["default"]; builtin {
		sourceCount--
	}
	if sourceCount > 16 {
		return "", "", errors.New("at most 16 effective trusted sources across documents and base configuration")
	}
	// Compiler certificate/bootstrap paths are relative to the config file,
	// not its invocation directory. Preserve that meaning in the private copy.
	base, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return "", "", err
	}
	if bootstrap, ok := config["bootstrap"].(map[string]any); ok {
		for id, value := range bootstrap {
			if path, ok := value.(string); ok && !filepath.IsAbs(path) {
				bootstrap[id] = filepath.Join(base, path)
			}
		}
	}
	for _, value := range sources {
		if table, ok := value.(map[string]any); ok {
			if path, ok := table["cert_path"].(string); ok && !filepath.IsAbs(path) {
				table["cert_path"] = filepath.Join(base, path)
			}
		}
	}
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).Encode(config); err != nil {
		return "", "", err
	}
	path := filepath.Join(directory, "document-config.zplc")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		return "", "", err
	}
	return policyDocumentsSource(envelope), path, nil
}

func policyDocumentCheckResult(source string, result policyCheckResponse) policyCheckResponse {
	if !isPolicyDocuments(source) {
		return result
	}
	result.Diagnostics = "Composed policy-document ZPL (line numbers are not JSON editor lines): " + result.Diagnostics
	result.Line = 0
	for index := range result.Warnings {
		warning := &result.Warnings[index]
		warning.Message = fmt.Sprintf("Composed ZPL line %d: %s", warning.Line, warning.Message)
		warning.Line = 0
	}
	return result
}
