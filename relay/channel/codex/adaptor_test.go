package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func newRelayInfo(mode int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:   mode,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
}

func TestConvertOpenAIResponsesRequest_ReasoningSuffix(t *testing.T) {
	a := &Adaptor{}
	out, err := a.ConvertOpenAIResponsesRequest(nil, newRelayInfo(relayconstant.RelayModeResponses), dto.OpenAIResponsesRequest{
		Model: "gpt-5.2-codex-xhigh",
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req, ok := out.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted type: %T", out)
	}
	if req.Model != "gpt-5.2-codex" {
		t.Fatalf("expected model gpt-5.2-codex, got %q", req.Model)
	}
	if req.Reasoning == nil {
		t.Fatalf("expected reasoning to be set")
	}
	if req.Reasoning.Effort != "xhigh" {
		t.Fatalf("expected effort xhigh, got %q", req.Reasoning.Effort)
	}
	if req.Reasoning.Summary != "auto" {
		t.Fatalf("expected summary auto, got %q", req.Reasoning.Summary)
	}
}

func TestConvertOpenAIResponsesRequest_ReasoningSuffixOverridesExistingReasoning(t *testing.T) {
	a := &Adaptor{}
	out, err := a.ConvertOpenAIResponsesRequest(nil, newRelayInfo(relayconstant.RelayModeResponses), dto.OpenAIResponsesRequest{
		Model: "gpt-5.1-codex-low",
		Reasoning: &dto.Reasoning{
			Effort:  "high",
			Summary: "detailed",
		},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req, ok := out.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted type: %T", out)
	}
	if req.Model != "gpt-5.1-codex" {
		t.Fatalf("expected model gpt-5.1-codex, got %q", req.Model)
	}
	if req.Reasoning == nil {
		t.Fatalf("expected reasoning to be set")
	}
	if req.Reasoning.Effort != "low" {
		t.Fatalf("expected effort low, got %q", req.Reasoning.Effort)
	}
	if req.Reasoning.Summary != "auto" {
		t.Fatalf("expected summary auto, got %q", req.Reasoning.Summary)
	}
}

func TestConvertOpenAIResponsesRequest_NoSuffixKeepsReasoning(t *testing.T) {
	a := &Adaptor{}
	info := newRelayInfo(relayconstant.RelayModeResponses)
	out, err := a.ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.1-codex",
		Reasoning: &dto.Reasoning{
			Effort:  "medium",
			Summary: "detailed",
		},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req, ok := out.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted type: %T", out)
	}
	if req.Model != "gpt-5.1-codex" {
		t.Fatalf("expected model unchanged, got %q", req.Model)
	}
	if req.Reasoning == nil {
		t.Fatalf("expected reasoning to be set")
	}
	if req.Reasoning.Effort != "medium" {
		t.Fatalf("expected effort medium, got %q", req.Reasoning.Effort)
	}
	if req.Reasoning.Summary != "detailed" {
		t.Fatalf("expected summary detailed, got %q", req.Reasoning.Summary)
	}
	if info.ReasoningEffort != "medium" {
		t.Fatalf("expected info reasoning effort medium, got %q", info.ReasoningEffort)
	}
}

func TestConvertOpenAIResponsesRequest_WrapsStringInputForResponses(t *testing.T) {
	adaptor := &Adaptor{}
	maxOutputTokens := uint(128)
	temperature := 0.7
	req := dto.OpenAIResponsesRequest{
		Model:           "gpt-5.4",
		Input:           json.RawMessage(`"Write a short bedtime story about a unicorn."`),
		MaxOutputTokens: &maxOutputTokens,
		Temperature:     &temperature,
	}

	convertedAny, err := adaptor.ConvertOpenAIResponsesRequest(nil, newRelayInfo(relayconstant.RelayModeResponses), req)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	converted, ok := convertedAny.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted type: %T", convertedAny)
	}

	if got := common.GetJsonType(converted.Input); got != "array" {
		t.Fatalf("expected input type array, got %s", got)
	}

	var inputs []dto.Input
	if err := common.Unmarshal(converted.Input, &inputs); err != nil {
		t.Fatalf("failed to unmarshal converted input: %v", err)
	}
	if len(inputs) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(inputs))
	}
	if inputs[0].Role != "user" {
		t.Fatalf("expected role user, got %q", inputs[0].Role)
	}

	var content string
	if err := common.Unmarshal(inputs[0].Content, &content); err != nil {
		t.Fatalf("failed to unmarshal input content: %v", err)
	}
	if content != "Write a short bedtime story about a unicorn." {
		t.Fatalf("unexpected input content: %q", content)
	}

	if got := common.GetJsonType(converted.Instructions); got != "string" {
		t.Fatalf("expected instructions type string, got %s", got)
	}

	var instructions string
	if err := common.Unmarshal(converted.Instructions, &instructions); err != nil {
		t.Fatalf("failed to unmarshal instructions: %v", err)
	}
	if instructions != "" {
		t.Fatalf("expected empty instructions, got %q", instructions)
	}

	if string(converted.Store) != "false" {
		t.Fatalf("expected store=false, got %s", string(converted.Store))
	}
	if converted.MaxOutputTokens != nil {
		t.Fatalf("expected max_output_tokens removed, got %v", *converted.MaxOutputTokens)
	}
	if converted.Temperature != nil {
		t.Fatalf("expected temperature removed, got %v", *converted.Temperature)
	}
}

func TestConvertOpenAIResponsesRequest_KeepArrayInputForResponses(t *testing.T) {
	adaptor := &Adaptor{}
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-5.4",
		Input: json.RawMessage(`[{"role":"user","content":"hi"}]`),
	}

	convertedAny, err := adaptor.ConvertOpenAIResponsesRequest(nil, newRelayInfo(relayconstant.RelayModeResponses), req)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	converted, ok := convertedAny.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted type: %T", convertedAny)
	}

	if string(converted.Input) != string(req.Input) {
		t.Fatalf("expected input unchanged, got %s", string(converted.Input))
	}
}

func TestConvertOpenAIResponsesRequest_CompactKeepsStringInput(t *testing.T) {
	adaptor := &Adaptor{}
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-5.4",
		Input: json.RawMessage(`"compact input"`),
	}

	convertedAny, err := adaptor.ConvertOpenAIResponsesRequest(nil, newRelayInfo(relayconstant.RelayModeResponsesCompact), req)
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	converted, ok := convertedAny.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("unexpected converted type: %T", convertedAny)
	}

	if got := common.GetJsonType(converted.Input); got != "string" {
		t.Fatalf("expected compact input to remain string, got %s", got)
	}
}

func TestGetRequestURL_UsesWebSocketWhenEnabled(t *testing.T) {
	adaptor := &Adaptor{}
	info := newRelayInfo(relayconstant.RelayModeResponses)
	info.ChannelBaseUrl = "https://chatgpt.com"
	original := operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled
	operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled = true
	defer func() {
		operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled = original
	}()

	requestURL, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	if requestURL != "wss://chatgpt.com/backend-api/codex/responses" {
		t.Fatalf("unexpected websocket request url: %s", requestURL)
	}
}

func TestGetRequestURL_CompactStillUsesHTTP(t *testing.T) {
	adaptor := &Adaptor{}
	info := newRelayInfo(relayconstant.RelayModeResponsesCompact)
	info.ChannelBaseUrl = "https://chatgpt.com"
	original := operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled
	operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled = true
	defer func() {
		operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled = original
	}()

	requestURL, err := adaptor.GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}

	if requestURL != "https://chatgpt.com/backend-api/codex/responses/compact" {
		t.Fatalf("unexpected compact request url: %s", requestURL)
	}
}

func TestBuildResponsesWebSocketRequestPayload_AddsCreateEvent(t *testing.T) {
	payload, err := buildResponsesWebSocketRequestPayload(strings.NewReader(`{"model":"gpt-5.4","stream":true,"input":"hi"}`))
	if err != nil {
		t.Fatalf("buildResponsesWebSocketRequestPayload returned error: %v", err)
	}

	var payloadMap map[string]json.RawMessage
	if err := common.Unmarshal(payload, &payloadMap); err != nil {
		t.Fatalf("failed to unmarshal websocket payload: %v", err)
	}

	var eventType string
	if err := common.Unmarshal(payloadMap["type"], &eventType); err != nil {
		t.Fatalf("failed to unmarshal websocket event type: %v", err)
	}
	if eventType != "response.create" {
		t.Fatalf("expected response.create, got %q", eventType)
	}
	if _, exists := payloadMap["stream"]; exists {
		t.Fatalf("expected stream field removed from websocket payload")
	}
	if got := common.GetJsonType(payloadMap["input"]); got != "string" {
		t.Fatalf("expected input preserved, got %s", got)
	}
}

func TestNormalizeResponsesWebSocketEvent_ErrorBecomesFailed(t *testing.T) {
	normalized, eventType, err := normalizeResponsesWebSocketEvent([]byte(`{"type":"error","error":{"message":"boom","type":"server_error"}}`))
	if err != nil {
		t.Fatalf("normalizeResponsesWebSocketEvent returned error: %v", err)
	}
	if eventType != "response.failed" {
		t.Fatalf("expected response.failed event type, got %q", eventType)
	}

	var payloadMap map[string]json.RawMessage
	if err := common.Unmarshal(normalized, &payloadMap); err != nil {
		t.Fatalf("failed to unmarshal normalized event: %v", err)
	}

	var normalizedType string
	if err := common.Unmarshal(payloadMap["type"], &normalizedType); err != nil {
		t.Fatalf("failed to unmarshal normalized type: %v", err)
	}
	if normalizedType != "response.failed" {
		t.Fatalf("expected normalized type response.failed, got %q", normalizedType)
	}
	if _, exists := payloadMap["response"]; !exists {
		t.Fatalf("expected normalized response payload")
	}
}
