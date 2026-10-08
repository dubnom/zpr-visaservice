package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

type simulatorDesignAssistantRequest struct {
	Scope          string             `json:"scope"`
	OrganizationID string             `json:"organization_id"`
	Scenario       json.RawMessage    `json:"scenario,omitempty"`
	Messages       []assistantMessage `json:"messages"`
	Model          string             `json:"model"`
	MaxTokens      int                `json:"max_tokens"`
}

type simulatorDesignProposal struct {
	Scenario      *simulatorScenario `json:"scenario,omitempty"`
	DirectoryLDIF string             `json:"directory_ldif,omitempty"`
}

type simulatorDesignAssistantResponse struct {
	Answer        string                   `json:"answer"`
	Proposal      *simulatorDesignProposal `json:"proposal,omitempty"`
	ProposalError string                   `json:"proposal_error,omitempty"`
	InputTokens   int                      `json:"input_tokens"`
	OutputTokens  int                      `json:"output_tokens"`
}

type simulatorDesignAssistantModelReply struct {
	Answer   string `json:"answer"`
	Proposal *struct {
		Scenario      json.RawMessage `json:"scenario"`
		DirectoryLDIF string          `json:"directory_ldif"`
	} `json:"proposal"`
}

func handleSimulatorAssistantStatus(w http.ResponseWriter, r *http.Request) {
	if !localEditorRequest(w, r) {
		return
	}
	assistant := newClaudeAssistant()
	writeSimulatorJSON(w, assistantStatus(assistant))
}

func simulatorDesignAssistantHandler(assistant *claudeAssistant) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !localEditorRequest(w, r) {
			return
		}
		if assistant == nil {
			writeWorkspaceError(w, http.StatusServiceUnavailable, "No assistant key is available to this service. Run scripts/configure-assistant.sh, then retry.")
			return
		}
		var request simulatorDesignAssistantRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAssistantBody))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeWorkspaceError(w, http.StatusBadRequest, "invalid assistant request")
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			writeWorkspaceError(w, http.StatusBadRequest, "assistant request contains trailing data")
			return
		}
		if len(request.Messages) == 0 || len(request.Messages) > 20 || len(request.Scenario) > maxScenarioFileSize {
			writeWorkspaceError(w, http.StatusBadRequest, "assistant request is outside the supported size limits")
			return
		}
		messageSize := 0
		for _, message := range request.Messages {
			messageSize += len(message.Content)
			if (message.Role != "user" && message.Role != "assistant") || len(message.Content) > 12000 {
				writeWorkspaceError(w, http.StatusBadRequest, "assistant messages must be bounded user or assistant text")
				return
			}
		}
		if messageSize > maxAssistantBody || request.Messages[len(request.Messages)-1].Role != "user" {
			writeWorkspaceError(w, http.StatusBadRequest, "assistant request is outside the supported size limits")
			return
		}
		if request.Model == "" {
			request.Model = assistant.model
		}
		if request.MaxTokens == 0 {
			request.MaxTokens = 1200
		}
		if !containsString(assistantModels(assistant.model), request.Model) {
			writeWorkspaceError(w, http.StatusBadRequest, "unsupported assistant model")
			return
		}
		if request.MaxTokens != 300 && request.MaxTokens != 600 && request.MaxTokens != 1200 && request.MaxTokens != 2400 {
			writeWorkspaceError(w, http.StatusBadRequest, "unsupported assistant output limit")
			return
		}
		system, proposalKind, err := simulatorDesignAssistantContext(r.Context(), request)
		if err != nil {
			writeWorkspaceError(w, http.StatusBadRequest, err.Error())
			return
		}
		if len(system)+messageSize > maxAssistantBody {
			writeWorkspaceError(w, http.StatusBadRequest, "design context exceeds the supported size limit")
			return
		}
		completion, err := assistant.complete(r.Context(), system, request.Messages, request.Model, request.MaxTokens)
		if err != nil {
			writeWorkspaceError(w, http.StatusBadGateway, "Claude could not complete the design request.")
			return
		}
		modelReply, err := decodeSimulatorDesignReply(completion.Answer)
		if err != nil {
			writeWorkspaceError(w, http.StatusBadGateway, "Claude returned an unreadable design response.")
			return
		}
		response := simulatorDesignAssistantResponse{Answer: modelReply.Answer, InputTokens: completion.InputTokens, OutputTokens: completion.OutputTokens}
		if modelReply.Proposal != nil {
			response.Proposal, response.ProposalError = validateSimulatorDesignProposal(request, proposalKind, modelReply.Proposal)
		}
		writeSimulatorJSON(w, response)
	}
}

func simulatorDesignAssistantContext(ctx context.Context, request simulatorDesignAssistantRequest) (string, string, error) {
	manifest, organization, err := manifestForOrganization(request.OrganizationID)
	if err != nil {
		return "", "", errors.New("unknown organization profile")
	}
	switch request.Scope {
	case "scenario":
		if len(request.Scenario) == 0 || !json.Valid(request.Scenario) {
			return "", "", errors.New("a valid scenario draft is required")
		}
		var scenario simulatorScenario
		if err := json.Unmarshal(request.Scenario, &scenario); err != nil {
			return "", "", errors.New("scenario draft could not be read")
		}
		scenario.OrganizationID = request.OrganizationID
		contextData, err := json.Marshal(struct {
			Organization simulatorOrganization `json:"organization"`
			Machines     []simulatorMachine    `json:"machines"`
			Components   []simulatorComponent  `json:"components"`
			Scenario     simulatorScenario     `json:"scenario"`
		}{organization, manifest.Machines, manifest.Components, scenario})
		if err != nil {
			return "", "", err
		}
		return simulatorScenarioAssistantSystem(string(contextData)), "scenario", nil
	case "organization":
		store, err := openSimulatorWorkspace(request.OrganizationID)
		if err != nil {
			return "", "", errors.New("organization workspace is unavailable")
		}
		defer store.Close()
		artifact, err := ensureWorkspaceDirectorySeed(ctx, store, request.OrganizationID, organization)
		if err != nil {
			return "", "", errors.New("organization directory seed is unavailable")
		}
		var document simulatorDirectoryDocument
		if err := json.Unmarshal(artifact.Content, &document); err != nil {
			return "", "", errors.New("organization directory draft could not be read")
		}
		contextData, err := json.Marshal(struct {
			Organization  simulatorOrganization `json:"organization"`
			DirectoryLDIF string                `json:"directory_ldif"`
		}{organization, document.LDIF})
		if err != nil {
			return "", "", err
		}
		return simulatorOrganizationAssistantSystem(string(contextData)), "organization", nil
	default:
		return "", "", errors.New("assistant scope must be scenario or organization")
	}
}

func simulatorScenarioAssistantSystem(contextData string) string {
	return scenarioAssistantSkill + "\n\n<scenario-design-context>\n" + contextData + "\n</scenario-design-context>"
}

func simulatorOrganizationAssistantSystem(contextData string) string {
	return organizationAssistantSkill + "\n\n<organization-design-context>\n" + contextData + "\n</organization-design-context>"
}

func decodeSimulatorDesignReply(content string) (simulatorDesignAssistantModelReply, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```json"), "```"))
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```"), "```"))
	}
	var result simulatorDesignAssistantModelReply
	decoder := json.NewDecoder(strings.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return simulatorDesignAssistantModelReply{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return simulatorDesignAssistantModelReply{}, errors.New("design response contains trailing data")
	}
	if strings.TrimSpace(result.Answer) == "" {
		return simulatorDesignAssistantModelReply{}, errors.New("design response has no answer")
	}
	return result, nil
}

func validateSimulatorDesignProposal(request simulatorDesignAssistantRequest, proposalKind string, candidate *struct {
	Scenario      json.RawMessage `json:"scenario"`
	DirectoryLDIF string          `json:"directory_ldif"`
}) (*simulatorDesignProposal, string) {
	if proposalKind == "scenario" && len(candidate.Scenario) > 0 && string(candidate.Scenario) != "null" {
		var scenario simulatorScenario
		decoder := json.NewDecoder(strings.NewReader(string(candidate.Scenario)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&scenario); err != nil {
			return nil, "Claude's scenario proposal was not valid scenario JSON."
		}
		var current simulatorScenario
		_ = json.Unmarshal(request.Scenario, &current)
		if current.ID != "" && scenario.ID != current.ID {
			return nil, "Claude changed the scenario ID; keep the current ID and try again."
		}
		scenario.OrganizationID = request.OrganizationID
		manifest, _, err := manifestForOrganization(request.OrganizationID)
		if err != nil || validateSimulatorScenario(scenario, manifest) != nil {
			return nil, "Claude's scenario proposal did not pass simulator validation."
		}
		return &simulatorDesignProposal{Scenario: &scenario}, ""
	}
	if proposalKind == "organization" && candidate.DirectoryLDIF != "" {
		_, organization, err := manifestForOrganization(request.OrganizationID)
		if err != nil || validateDirectoryLDIF(candidate.DirectoryLDIF, organization.Directory.BaseDN) != nil {
			return nil, "Claude's LDIF proposal did not pass directory validation."
		}
		return &simulatorDesignProposal{DirectoryLDIF: candidate.DirectoryLDIF}, ""
	}
	return nil, ""
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
