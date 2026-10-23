package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"neboagency.com/zpr-dashborad/internal/assistant"
)

func validateDirectEditorEdits(source, answer string) error {
	var response struct {
		Edits []struct {
			Before string `json:"before"`
			After  string `json:"after"`
		} `json:"edits"`
	}
	if err := json.Unmarshal([]byte(answer), &response); err != nil {
		return fmt.Errorf("invalid edit JSON")
	}
	type span struct{ start, end int }
	spans := make([]span, 0, len(response.Edits))
	for index, edit := range response.Edits {
		start := strings.Index(source, edit.Before)
		if (edit.Before == "" && source != "") || start < 0 ||
			(edit.Before != "" && strings.Contains(source[start+1:], edit.Before)) {
			return fmt.Errorf("edit %d before text must match exactly one location in the original source; include enough unchanged neighboring text to make it unique", index+1)
		}
		if edit.Before == edit.After {
			return fmt.Errorf("edit %d does not change the source", index+1)
		}
		spans = append(spans, span{start, start + len(edit.Before)})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for index := 1; index < len(spans); index++ {
		if spans[index].start < spans[index-1].end || spans[index].start == spans[index-1].start {
			return fmt.Errorf("edits overlap; combine overlapping replacements into one unique original-source snippet")
		}
	}
	return nil
}

func completeDirectEditorEdits(ctx context.Context, client *assistant.Client, system string, request assistantRequest) (assistantReply, error) {
	messages := append([]assistantMessage(nil), request.Messages...)
	answer, err := client.CompleteEditorEdits(ctx, system, messages, request.Model, request.MaxTokens)
	if err != nil {
		return answer, err
	}
	if validationErr := validateDirectEditorEdits(request.Source, answer.Answer); validationErr != nil {
		remainingTokens := request.MaxTokens - answer.OutputTokens
		if remainingTokens <= 0 {
			return answer, assistant.ErrEditorOutputLimit
		}
		messages = append(messages,
			assistantMessage{Role: "assistant", Content: answer.Answer},
			assistantMessage{Role: "user", Content: "The proposed edits were rejected before application: " + validationErr.Error() +
				". No edits have been applied. Return corrected edits against the unchanged original source, not against your proposed result. Do not change the requested intent."})
		corrected, retryErr := client.CompleteEditorEdits(ctx, system, messages, request.Model, remainingTokens)
		corrected.InputTokens += answer.InputTokens
		corrected.OutputTokens += answer.OutputTokens
		if retryErr != nil {
			return corrected, retryErr
		}
		if validationErr := validateDirectEditorEdits(request.Source, corrected.Answer); validationErr != nil {
			return corrected, fmt.Errorf("%w: %v", assistant.ErrEditorResponse, validationErr)
		}
		return corrected, nil
	}
	return answer, nil
}
