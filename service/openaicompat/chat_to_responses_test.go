package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestChatCompletionsRequestToResponsesRequest_TextContentUsesListParts(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-5-codex",
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: "hello",
			},
			{
				Role:    "assistant",
				Content: "hi",
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	var inputItems []map[string]any
	if err := common.Unmarshal(out.Input, &inputItems); err != nil {
		t.Fatalf("failed to decode input: %v", err)
	}
	if len(inputItems) != 2 {
		t.Fatalf("expected 2 input items, got %d", len(inputItems))
	}

	userParts, ok := inputItems[0]["content"].([]any)
	if !ok || len(userParts) != 1 {
		t.Fatalf("expected user content as single list part, got: %#v", inputItems[0]["content"])
	}
	userPart, ok := userParts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected user part map, got: %#v", userParts[0])
	}
	if userPart["type"] != "input_text" {
		t.Fatalf("expected user part type input_text, got: %#v", userPart["type"])
	}

	assistantParts, ok := inputItems[1]["content"].([]any)
	if !ok || len(assistantParts) != 1 {
		t.Fatalf("expected assistant content as single list part, got: %#v", inputItems[1]["content"])
	}
	assistantPart, ok := assistantParts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected assistant part map, got: %#v", assistantParts[0])
	}
	if assistantPart["type"] != "output_text" {
		t.Fatalf("expected assistant part type output_text, got: %#v", assistantPart["type"])
	}
}

func TestChatCompletionsRequestToResponsesRequest_FileContentUsesInputFileFields(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-5-codex",
		Messages: []dto.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "file",
						"file": map[string]any{
							"file_id": "file-123",
						},
					},
				},
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	var inputItems []map[string]any
	if err := common.Unmarshal(out.Input, &inputItems); err != nil {
		t.Fatalf("failed to decode input: %v", err)
	}
	if len(inputItems) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(inputItems))
	}
	parts, ok := inputItems[0]["content"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("expected single content part, got %#v", inputItems[0]["content"])
	}
	part, ok := parts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected part map, got %#v", parts[0])
	}
	if part["type"] != "input_file" {
		t.Fatalf("expected part type input_file, got %#v", part["type"])
	}
	if part["file_id"] != "file-123" {
		t.Fatalf("expected file_id=file-123, got %#v", part["file_id"])
	}
	if _, exists := part["file"]; exists {
		t.Fatalf("unexpected nested file field in part: %#v", part)
	}
}
