package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	logger.LogInfo(c, "MES: GeminiTextGenerationHandler start, path="+info.RequestURLPath)
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	// 解析为 Gemini 原生响应格式
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if len(geminiResponse.Candidates) == 0 && geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", *geminiResponse.PromptFeedback.BlockReason))
	}

	// 计算使用量（基于 UsageMetadata）
	usage := dto.Usage{
		PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount + geminiResponse.UsageMetadata.ThoughtsTokenCount,
		TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
	}

	usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount
	usage.PromptTokensDetails.CachedTokens = geminiResponse.UsageMetadata.CachedContentTokenCount

	for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
		if detail.Modality == "AUDIO" {
			usage.PromptTokensDetails.AudioTokens = detail.TokenCount
		} else if detail.Modality == "TEXT" {
			usage.PromptTokensDetails.TextTokens = detail.TokenCount
		}
	}

	// 添加缓存 token 详情，但不计入配额计费
	if len(geminiResponse.UsageMetadata.CacheTokensDetails) > 0 {
		usage.CacheTokensDetails = make([]dto.CacheTokensDetails, len(geminiResponse.UsageMetadata.CacheTokensDetails))
		for i, detail := range geminiResponse.UsageMetadata.CacheTokensDetails {
			usage.CacheTokensDetails[i] = dto.CacheTokensDetails{
				Modality:   detail.Modality,
				TokenCount: detail.TokenCount,
			}
		}
	}

	// 添加缓存内容 token 总数
	if geminiResponse.UsageMetadata.CachedContentTokenCount > 0 {
		usage.CachedContentTokenCount = geminiResponse.UsageMetadata.CachedContentTokenCount
	}

	// 以 Gemini 原生格式写入 MES
	var respMap map[string]interface{}
	if raw, errMarshal := common.Marshal(geminiResponse); errMarshal == nil {
		_ = common.Unmarshal(raw, &respMap)
	}
	if respMap == nil {
		respMap = make(map[string]interface{})
	}
	respMap["usage"] = map[string]interface{}{
		"prompt_tokens":     usage.PromptTokens,
		"completion_tokens": usage.CompletionTokens,
		"total_tokens":      usage.TotalTokens,
	}
	// 异步写入，避免阻塞主流程
	helper.SaveMESWithGenericResponseAsync(c, info, respMap)

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func NativeGeminiEmbeddingHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	usage := service.ResponseText2Usage(c, "", info.UpstreamModelName, info.GetEstimatePromptTokens())

	if info.IsGeminiBatchEmbedding {
		var geminiResponse dto.GeminiBatchEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	} else {
		var geminiResponse dto.GeminiEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return usage, nil
}

func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	logger.LogInfo(c, "MES: GeminiTextGenerationStreamHandler start, path="+info.RequestURLPath)
	id := helper.GetResponseID(c)
	createAt := common.GetTimestamp()
	helper.SetEventStreamHeaders(c)

	usage, aggregatedText, imageCount, err := geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		err := helper.StringData(c, data)
		if err != nil {
			logger.LogError(c, "failed to write stream data: "+err.Error())
			return false
		}
		info.SendResponseCount++
		return true
	})

	if info.SendResponseCount == 0 {
		return nil, types.NewOpenAIError(errors.New("no response received from Gemini API"), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}

	if imageCount != 0 && usage.CompletionTokens == 0 {
		usage.CompletionTokens = imageCount * 258
	}

	if usage.CompletionTokens == 0 {
		if len(aggregatedText) > 0 {
			usage = service.ResponseText2Usage(c, aggregatedText, info.UpstreamModelName, info.GetEstimatePromptTokens())
		} else {
			usage = &dto.Usage{}
		}
	}

	streamResp := map[string]interface{}{
		"stream": true,
		"text":   aggregatedText,
		"usage": map[string]interface{}{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		},
		"response_id": id,
		"created":     createAt,
		"model":       info.UpstreamModelName,
	}
	// 异步写入，避免阻塞主流程
	helper.SaveMESWithGenericResponseAsync(c, info, streamResp)

	return usage, err
}
