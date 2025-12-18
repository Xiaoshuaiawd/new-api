package codex

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ChannelName = "Codex"
)

type Adaptor struct {
	openai.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.Adaptor.Init(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/v1/responses", info.ChannelBaseUrl), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// 生成UUID用于conversation_id和session_id
	conversationID := uuid.New().String()
	sessionID := conversationID // 与conversation_id保持一致

	// 先删除所有现有的headers，确保完全硬编码
	for key := range *req {
		req.Del(key)
	}

	// 设置所有硬编码的headers
	req.Set("User-Agent", "codex_cli_rs/0.73.0 (Mac OS 15.3.0; arm64) Apple_Terminal/455")
	req.Set("Accept", "text/event-stream")
	req.Set("Content-Type", "application/json")
	req.Set("conversation_id", conversationID)
	req.Set("session_id", sessionID)
	req.Set("Authorization", info.ApiKey)
	req.Set("originator", "codex_cli_rs")

	// 将conversation_id存储到context中供后续使用
	c.Set("codex_conversation_id", conversationID)

	return nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// 获取conversation_id
	conversationID, exists := c.Get("codex_conversation_id")
	if !exists {
		return nil, errors.New("conversation_id not found in context")
	}

	// 替换prompt_cache_key为conversation_id
	request.PromptCacheKey = []byte(fmt.Sprintf(`"%s"`, conversationID))

	return a.Adaptor.ConvertOpenAIResponsesRequest(c, info, request)
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	// 获取conversation_id
	conversationID, exists := c.Get("codex_conversation_id")
	if !exists {
		return nil, errors.New("conversation_id not found in context")
	}

	// 替换prompt_cache_key为conversation_id
	request.PromptCacheKey = conversationID.(string)

	return a.Adaptor.ConvertOpenAIRequest(c, info, request)
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	return a.Adaptor.DoResponse(c, resp, info)
}

func (a *Adaptor) GetModelList() []string {
	return []string{
		"gpt-4o",
		"gpt-4o-mini",
		"o1",
		"o1-mini",
		"o3-mini",
	}
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
