package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func handleScenarioSourceCheck(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Source     string `json:"source"`
		ScenarioID string `json:"scenario_id,omitempty"`
		Format     string `json:"format,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*maxScenarioFileSize))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeWorkspaceError(w, http.StatusBadRequest, "invalid scenario check request")
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || len(request.Source) > maxScenarioFileSize {
		writeWorkspaceError(w, http.StatusBadRequest, "scenario check request exceeds its limit or contains trailing data")
		return
	}
	format := strings.ToLower(strings.TrimSpace(request.Format))
	if format == "" {
		format = "json"
	}
	scenario, diagnostic := parseScenarioSource(request.Source, format)
	if diagnostic != nil {
		writeScenarioCheckDiagnostic(w, *diagnostic)
		return
	}
	organizationID := r.PathValue("organization")
	if scenario.OrganizationID != "" && scenario.OrganizationID != organizationID {
		writeScenarioCheckDiagnostic(w, scenarioSourceDiagnostic{Message: "Scenario organization_id must match the selected organization."})
		return
	}
	if request.ScenarioID != "" && scenario.ID != request.ScenarioID {
		writeScenarioCheckDiagnostic(w, scenarioSourceDiagnostic{Message: "The existing scenario ID cannot be changed."})
		return
	}
	manifest, _, err := manifestForOrganization(organizationID)
	if err != nil {
		writeWorkspaceError(w, http.StatusNotFound, "unknown organization or simulation manifest unavailable")
		return
	}
	scenario.OrganizationID = organizationID
	if err := validateSimulatorScenario(scenario, manifest); err != nil {
		writeScenarioCheckDiagnostic(w, scenarioSourceDiagnostic{Message: err.Error()})
		return
	}
	canonicalJSON, err := json.MarshalIndent(scenario, "", "  ")
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "scenario canonicalization failed")
		return
	}
	canonicalYAML, err := scenarioToYAML(scenario)
	if err != nil {
		writeWorkspaceError(w, http.StatusInternalServerError, "scenario YAML conversion failed")
		return
	}
	writeSimulatorJSON(w, map[string]any{
		"valid":          true,
		"diagnostics":    "Scenario source and definition valid. Nothing saved, published, or run.",
		"scenario":       scenario,
		"canonical_json": string(canonicalJSON),
		"canonical_yaml": canonicalYAML,
	})
}

type scenarioSourceDiagnostic struct {
	Message string `json:"error"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
}

func writeScenarioCheckDiagnostic(w http.ResponseWriter, diagnostic scenarioSourceDiagnostic) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid":  false,
		"error":  diagnostic.Message,
		"line":   diagnostic.Line,
		"column": diagnostic.Column,
	})
}

func parseScenarioSource(sourceText, format string) (simulatorScenario, *scenarioSourceDiagnostic) {
	switch format {
	case "json":
		return parseScenarioJSON(sourceText)
	case "yaml", "yml":
		return parseScenarioYAML(sourceText)
	default:
		return simulatorScenario{}, &scenarioSourceDiagnostic{Message: "scenario source format must be json or yaml"}
	}
}

func parseScenarioJSON(sourceText string) (simulatorScenario, *scenarioSourceDiagnostic) {
	reject := func(message string, offset int64) (simulatorScenario, *scenarioSourceDiagnostic) {
		line, column := sourceOffsetLineColumn(sourceText, offset)
		return simulatorScenario{}, &scenarioSourceDiagnostic{Message: message, Line: line, Column: column}
	}
	var scenario simulatorScenario
	source := json.NewDecoder(strings.NewReader(sourceText))
	source.DisallowUnknownFields()
	if err := source.Decode(&scenario); err != nil {
		var syntax *json.SyntaxError
		var typeError *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntax):
			return reject(err.Error(), syntax.Offset)
		case errors.As(err, &typeError):
			return reject(err.Error(), typeError.Offset)
		case errors.Is(err, io.ErrUnexpectedEOF):
			return reject(err.Error(), int64(len(sourceText)))
		default:
			return simulatorScenario{}, &scenarioSourceDiagnostic{Message: err.Error()}
		}
	}
	var trailing any
	if err := source.Decode(&trailing); err != io.EOF {
		return simulatorScenario{}, &scenarioSourceDiagnostic{Message: "scenario source must contain one JSON object"}
	}
	return scenario, nil
}

func parseScenarioYAML(sourceText string) (simulatorScenario, *scenarioSourceDiagnostic) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(sourceText), &root); err != nil {
		line, column := yamlErrorLineColumn(err)
		return simulatorScenario{}, &scenarioSourceDiagnostic{Message: err.Error(), Line: line, Column: column}
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return simulatorScenario{}, &scenarioSourceDiagnostic{Message: "scenario YAML must be a mapping object", Line: nodeLine(root.Content)}
	}
	if diagnostic := validateScenarioYAMLNode(root.Content[0]); diagnostic != nil {
		return simulatorScenario{}, diagnostic
	}
	value, diagnostic := yamlNodeValue(root.Content[0])
	if diagnostic != nil {
		return simulatorScenario{}, diagnostic
	}
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return simulatorScenario{}, &scenarioSourceDiagnostic{Message: err.Error()}
	}
	return parseScenarioJSON(string(jsonBytes))
}

func validateScenarioYAMLNode(node *yaml.Node) *scenarioSourceDiagnostic {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.AliasNode {
		return &scenarioSourceDiagnostic{Message: "scenario YAML aliases are not supported", Line: node.Line, Column: node.Column}
	}
	if node.Anchor != "" {
		return &scenarioSourceDiagnostic{Message: "scenario YAML anchors are not supported", Line: node.Line, Column: node.Column}
	}
	if node.Kind == yaml.MappingNode {
		seen := map[string]*yaml.Node{}
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind == yaml.ScalarNode {
				if previous := seen[key.Value]; previous != nil {
					return &scenarioSourceDiagnostic{Message: fmt.Sprintf("duplicate YAML key %q", key.Value), Line: key.Line, Column: key.Column}
				}
				seen[key.Value] = key
			}
		}
	}
	for _, child := range node.Content {
		if diagnostic := validateScenarioYAMLNode(child); diagnostic != nil {
			return diagnostic
		}
	}
	return nil
}

func yamlNodeValue(node *yaml.Node) (any, *scenarioSourceDiagnostic) {
	switch node.Kind {
	case yaml.MappingNode:
		result := map[string]any{}
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return nil, &scenarioSourceDiagnostic{Message: "scenario YAML mapping keys must be strings", Line: key.Line, Column: key.Column}
			}
			value, diagnostic := yamlNodeValue(node.Content[index+1])
			if diagnostic != nil {
				return nil, diagnostic
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(node.Content))
		for _, child := range node.Content {
			value, diagnostic := yamlNodeValue(child)
			if diagnostic != nil {
				return nil, diagnostic
			}
			result = append(result, value)
		}
		return result, nil
	case yaml.ScalarNode:
		var value any
		if err := node.Decode(&value); err != nil {
			return nil, &scenarioSourceDiagnostic{Message: err.Error(), Line: node.Line, Column: node.Column}
		}
		return value, nil
	default:
		return nil, &scenarioSourceDiagnostic{Message: "unsupported YAML node in scenario source", Line: node.Line, Column: node.Column}
	}
}

func scenarioToYAML(scenario simulatorScenario) (string, error) {
	jsonBytes, err := json.Marshal(scenario)
	if err != nil {
		return "", err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(jsonBytes, &node); err != nil {
		return "", err
	}
	clearYAMLStyle(&node)
	yamlBytes, err := yaml.Marshal(&node)
	return string(yamlBytes), err
}

func clearYAMLStyle(node *yaml.Node) {
	if node == nil {
		return
	}
	node.Style = 0
	for _, child := range node.Content {
		clearYAMLStyle(child)
	}
}

func sourceOffsetLineColumn(sourceText string, offset int64) (int, int) {
	if offset <= 0 || offset > int64(len(sourceText)) {
		return 0, 0
	}
	before := sourceText[:offset-1]
	line := strings.Count(before, "\n") + 1
	column := len(before) - strings.LastIndex(before, "\n")
	return line, column
}

func yamlErrorLineColumn(err error) (int, int) {
	match := regexp.MustCompile(`line (\d+)(?::|:)`).FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0, 0
	}
	var line int
	_, _ = fmt.Sscanf(match[1], "%d", &line)
	return line, 0
}

func nodeLine(nodes []*yaml.Node) int {
	if len(nodes) == 0 || nodes[0] == nil {
		return 0
	}
	return nodes[0].Line
}
