package claudecode

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/relay/channel/claude"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
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
	claude.Adaptor
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.Adaptor.Init(info)
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := fmt.Sprintf("%s/v1/messages", info.ChannelBaseUrl)
	// 保留beta参数传递
	if info.IsClaudeBetaQuery {
		baseURL = baseURL + "?beta=true"
	}
	return baseURL, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	// 设置所有硬编码的headers
	req.Set("User-Agent", "User-Agent")
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

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
