package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/google/uuid"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func codexTextTypeByRole(role string) string {
	if strings.EqualFold(strings.TrimSpace(role), "assistant") {
		return "output_text"
	}
	return "input_text"
}

func normalizeCodexInputFilePart(part map[string]any) {
	if part == nil {
		return
	}
	partType := strings.ToLower(strings.TrimSpace(common.Interface2String(part["type"])))
	if partType == "file" {
		part["type"] = "input_file"
		partType = "input_file"
	}
	if partType != "input_file" {
		return
	}

	fileAny, hasFile := part["file"]
	if !hasFile {
		return
	}

	setField := func(key string, val string) {
		if strings.TrimSpace(val) != "" {
			part[key] = strings.TrimSpace(val)
		}
	}

	switch fv := fileAny.(type) {
	case map[string]any:
		setField("file_id", common.Interface2String(fv["file_id"]))
		setField("file_data", common.Interface2String(fv["file_data"]))
		setField("filename", common.Interface2String(fv["filename"]))
		setField("filename", common.Interface2String(fv["file_name"]))
		setField("file_url", common.Interface2String(fv["file_url"]))
		setField("file_url", common.Interface2String(fv["url"]))
	case string:
		s := strings.TrimSpace(fv)
		if strings.HasPrefix(s, "file-") {
			setField("file_id", s)
		} else {
			setField("file_url", s)
		}
	default:
		if b, err := common.Marshal(fileAny); err == nil {
			var fileMap map[string]any
			if common.Unmarshal(b, &fileMap) == nil {
				setField("file_id", common.Interface2String(fileMap["file_id"]))
				setField("file_data", common.Interface2String(fileMap["file_data"]))
				setField("filename", common.Interface2String(fileMap["filename"]))
				setField("filename", common.Interface2String(fileMap["file_name"]))
				setField("file_url", common.Interface2String(fileMap["file_url"]))
				setField("file_url", common.Interface2String(fileMap["url"]))
			}
		}
	}

	delete(part, "file")
}

func normalizeCodexTextPartType(part map[string]any, role string) {
	partType := strings.TrimSpace(common.Interface2String(part["type"]))
	switch partType {
	case "", "text", "input_text", "output_text":
		targetType := codexTextTypeByRole(role)
		part["type"] = targetType
		if _, exists := part["text"]; exists {
			return
		}
		if targetType == "output_text" {
			if alt, ok := part["output_text"]; ok {
				part["text"] = alt
				return
			}
		} else {
			if alt, ok := part["input_text"]; ok {
				part["text"] = alt
				return
			}
		}
		if alt, ok := part["text_value"]; ok {
			part["text"] = alt
		}
	}
}

func normalizeCodexInput(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("[]"), nil
	}

	var inputAny any
	if err := common.Unmarshal(raw, &inputAny); err != nil {
		return nil, err
	}

	var inputs []any
	if arr, ok := inputAny.([]any); ok {
		inputs = arr
	} else {
		inputs = []any{inputAny}
	}

	for i, input := range inputs {
		if input == nil {
			inputs[i] = map[string]any{
				"role":    "user",
				"content": []any{},
			}
			continue
		}

		item, ok := input.(map[string]any)
		if !ok {
			text := common.Interface2String(input)
			if text == "" {
				if b, err := common.Marshal(input); err == nil {
					text = string(b)
				}
			}
			inputs[i] = map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "input_text",
						"text": text,
					},
				},
			}
			continue
		}

		itemType := strings.ToLower(strings.TrimSpace(common.Interface2String(item["type"])))
		// Only message-like inputs should carry role/content.
		// For typed non-message inputs (e.g. function_call_output), keep payload as-is.
		if itemType != "" && itemType != "message" {
			inputs[i] = item
			continue
		}

		role := strings.ToLower(strings.TrimSpace(common.Interface2String(item["role"])))
		if role == "" {
			role = "user"
			item["role"] = role
		}

		content := item["content"]
		switch v := content.(type) {
		case nil:
			item["content"] = []any{}
		case string:
			item["content"] = []any{
				map[string]any{
					"type": codexTextTypeByRole(role),
					"text": v,
				},
			}
		case []any:
			for idx, partAny := range v {
				partMap, ok := partAny.(map[string]any)
				if !ok {
					continue
				}
				normalizeCodexInputFilePart(partMap)
				normalizeCodexTextPartType(partMap, role)
				v[idx] = partMap
			}
			item["content"] = v
		default:
			item["content"] = []any{v}
		}
		inputs[i] = item
	}

	inputRaw, err := common.Marshal(inputs)
	if err != nil {
		return nil, err
	}
	return inputRaw, nil
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("codex channel: endpoint not supported")
}

func (a *Adaptor) ConvertClaudeRequest(*gin.Context, *relaycommon.RelayInfo, *dto.ClaudeRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/messages endpoint not supported")
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("codex channel: endpoint not supported")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("codex channel: endpoint not supported")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/chat/completions endpoint not supported")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/rerank endpoint not supported")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("codex channel: /v1/embeddings endpoint not supported")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	isCompact := info != nil && info.RelayMode == relayconstant.RelayModeResponsesCompact

	// Codex backend rejects the legacy `user` field.
	request.User = ""

	// Codex backend expects `input` to be a list and assistant text parts to use output_text.
	inputRaw, err := normalizeCodexInput(request.Input)
	if err != nil {
		return nil, err
	}
	request.Input = inputRaw

	if info != nil && info.ChannelSetting.SystemPrompt != "" {
		systemPrompt := info.ChannelSetting.SystemPrompt

		if len(request.Instructions) == 0 {
			if b, err := common.Marshal(systemPrompt); err == nil {
				request.Instructions = b
			} else {
				return nil, err
			}
		} else if info.ChannelSetting.SystemPromptOverride {
			var existing string
			if err := common.Unmarshal(request.Instructions, &existing); err == nil {
				existing = strings.TrimSpace(existing)
				if existing == "" {
					if b, err := common.Marshal(systemPrompt); err == nil {
						request.Instructions = b
					} else {
						return nil, err
					}
				} else {
					if b, err := common.Marshal(systemPrompt + "\n" + existing); err == nil {
						request.Instructions = b
					} else {
						return nil, err
					}
				}
			} else {
				if b, err := common.Marshal(systemPrompt); err == nil {
					request.Instructions = b
				} else {
					return nil, err
				}
			}
		}
	}
	// Codex backend requires the `instructions` field to be present.
	// Keep it consistent with Codex CLI behavior by defaulting to an empty string.
	if len(request.Instructions) == 0 {
		request.Instructions = json.RawMessage(`""`)
	}

	// Codex requires prompt_cache_key to be present. Generate one when missing.
	if len(request.PromptCacheKey) == 0 {
		if b, err := common.Marshal(uuid.New().String()); err == nil {
			request.PromptCacheKey = b
		} else {
			return nil, err
		}
	}

	// If tools are missing, remove tool_choice/parallel_tool_calls.
	// If tools exist and these fields are missing, add defaults.
	if len(request.Tools) == 0 {
		request.ToolChoice = nil
		request.ParallelToolCalls = nil
	} else {
		if len(request.ToolChoice) == 0 {
			if b, err := common.Marshal("auto"); err == nil {
				request.ToolChoice = b
			} else {
				return nil, err
			}
		}
		if len(request.ParallelToolCalls) == 0 {
			request.ParallelToolCalls = json.RawMessage("false")
		}
	}

	// Codex upstream only supports streaming responses.
	if !isCompact {
		request.Stream = true
	}

	if isCompact {
		return request, nil
	}
	// codex: store must be false
	request.Store = json.RawMessage("false")
	// rm max_output_tokens
	request.MaxOutputTokens = 0
	request.Temperature = nil
	request.TopP = nil
	return request, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode != relayconstant.RelayModeResponses && info.RelayMode != relayconstant.RelayModeResponsesCompact {
		return nil, types.NewError(errors.New("codex channel: endpoint not supported"), types.ErrorCodeInvalidRequest)
	}

	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		return openai.OaiResponsesCompactionHandler(c, resp)
	}

	if info.IsStream {
		return openai.OaiResponsesStreamHandler(c, info, resp)
	}
	if resp != nil && isResponsesStream(resp) {
		return responsesStreamToNonStreamHandler(c, info, resp)
	}
	return openai.OaiResponsesHandler(c, info, resp)
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode != relayconstant.RelayModeResponses && info.RelayMode != relayconstant.RelayModeResponsesCompact {
		return "", errors.New("codex channel: only /v1/responses and /v1/responses/compact are supported")
	}
	path := "/backend-api/codex/responses"
	if info.RelayMode == relayconstant.RelayModeResponsesCompact {
		path = "/backend-api/codex/responses/compact"
	}
	return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, path, info.ChannelType), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)

	key := strings.TrimSpace(info.ApiKey)
	if !strings.HasPrefix(key, "{") {
		return errors.New("codex channel: key must be a JSON object")
	}

	oauthKey, err := ParseOAuthKey(key)
	if err != nil {
		return err
	}

	accessToken := strings.TrimSpace(oauthKey.AccessToken)
	accountID := strings.TrimSpace(oauthKey.AccountID)

	if accessToken == "" {
		return errors.New("codex channel: access_token is required")
	}
	if accountID == "" {
		return errors.New("codex channel: account_id is required")
	}

	req.Set("Authorization", "Bearer "+accessToken)
	req.Set("chatgpt-account-id", accountID)

	if req.Get("OpenAI-Beta") == "" {
		req.Set("OpenAI-Beta", "responses=experimental")
	}
	if req.Get("originator") == "" {
		req.Set("originator", "codex_cli_rs")
	}

	// chatgpt.com/backend-api/codex/responses is strict about Content-Type.
	// Clients may omit it or include parameters like `application/json; charset=utf-8`,
	// which can be rejected by the upstream. Force the exact media type.
	req.Set("Content-Type", "application/json")
	if info.RelayMode == relayconstant.RelayModeResponses {
		req.Set("Accept", "text/event-stream")
	} else if info.IsStream {
		req.Set("Accept", "text/event-stream")
	} else if req.Get("Accept") == "" {
		req.Set("Accept", "application/json")
	}

	return nil
}

func isResponsesStream(resp *http.Response) bool {
	if resp == nil || resp.Body == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "text/event-stream") {
		return true
	}

	reader := bufio.NewReader(resp.Body)
	peek, err := reader.Peek(64)
	if err == nil || err == io.EOF {
		trimmed := strings.TrimLeft(string(peek), " \r\n\t")
		if strings.HasPrefix(trimmed, "event:") || strings.HasPrefix(trimmed, "data:") {
			resp.Body = io.NopCloser(reader)
			return true
		}
	}
	resp.Body = io.NopCloser(reader)
	return false
}
