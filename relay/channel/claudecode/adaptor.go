package claudecode

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	ChannelName = "ClaudeCode"
)

var ModelList = []string{
	"claude-3-5-sonnet-20241022",
	"claude-3-5-sonnet-20240620",
	"claude-3-opus-20240229",
	"claude-3-sonnet-20240229",
	"claude-3-haiku-20240307",
	"claude-3-5-haiku-20241022",
	"claude-sonnet-4-5-20250929",
}

type Adaptor struct {
	RequestMode int
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// 使用claude的RequestMode逻辑
	claudeAdaptor := &claude.Adaptor{}
	claudeAdaptor.Init(info)
	a.RequestMode = claudeAdaptor.RequestMode
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// 直接使用info.RequestURLPath，它已经包含了beta参数（如果有的话）
	baseURL := fmt.Sprintf("%s%s", info.ChannelBaseUrl, info.RequestURLPath)
	return baseURL, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// 先删除所有现有的headers，确保完全硬编码
	for key := range *req {
		req.Del(key)
	}

	// 完全硬编码所有headers
	req.Set("User-Agent", "claude-cli/2.0.13 (external, cli)")
	req.Set("Accept", "application/json")
	req.Set("X-Stainless-Retry-Count", "0")
	req.Set("X-Stainless-Timeout", "600")
	req.Set("X-Stainless-Lang", "js")
	req.Set("X-Stainless-Package-Version", "0.60.0")
	req.Set("X-Stainless-OS", "MacOS")
	req.Set("X-Stainless-Arch", "arm64")
	req.Set("X-Stainless-Runtime", "node")
	req.Set("X-Stainless-Runtime-Version", "v22.14.0")
	req.Set("anthropic-dangerous-direct-browser-access", "true")
	req.Set("anthropic-version", "2023-06-01")
	req.Set("x-app", "cli")
	req.Set("Authorization", info.ApiKey)
	req.Set("anthropic-beta", "fine-grained-tool-streaming-2025-05-14")
	req.Set("x-stainless-helper-method", "stream")
	req.Set("accept-language", "*")
	req.Set("sec-fetch-mode", "cors")
	req.Set("Content-Type", "application/json")

	return nil
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	// 使用claude的转换逻辑
	claudeAdaptor := &claude.Adaptor{RequestMode: a.RequestMode}
	return claudeAdaptor.ConvertOpenAIRequest(c, info, request)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// 使用claude的响应处理逻辑，根据是否流式选择对应的处理器
	if info.IsStream {
		return claude.ClaudeStreamHandler(c, resp, info, a.RequestMode)
	} else {
		return claude.ClaudeHandler(c, resp, info, a.RequestMode)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
