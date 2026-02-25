package codex

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestConvertOpenAIResponsesRequest_NormalizesUserAndInput(t *testing.T) {
	adaptor := &Adaptor{}
	request := dto.OpenAIResponsesRequest{
		Model: "gpt-5-codex",
		User:  "legacy-user-id",
		Input: json.RawMessage(`{"role":"assistant","content":"hello"}`),
	}

	convertedAny, err := adaptor.ConvertOpenAIResponsesRequest(nil, nil, request)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	converted, ok := convertedAny.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("expected dto.OpenAIResponsesRequest, got %T", convertedAny)
	}

	if converted.User != "" {
		t.Fatalf("expected user to be stripped, got %q", converted.User)
	}

	var inputItems []map[string]any
	if err := common.Unmarshal(converted.Input, &inputItems); err != nil {
		t.Fatalf("failed to decode normalized input: %v", err)
	}
	if len(inputItems) != 1 {
		t.Fatalf("expected 1 normalized input item, got %d", len(inputItems))
	}
	if inputItems[0]["role"] != "assistant" {
		t.Fatalf("expected role assistant, got %#v", inputItems[0]["role"])
	}

	contentParts, ok := inputItems[0]["content"].([]any)
	if !ok || len(contentParts) != 1 {
		t.Fatalf("expected single content part list, got %#v", inputItems[0]["content"])
	}
	part, ok := contentParts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected part map, got %#v", contentParts[0])
	}
	if part["type"] != "output_text" {
		t.Fatalf("expected assistant text type output_text, got %#v", part["type"])
	}
}
