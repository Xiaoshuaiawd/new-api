package geminibusiness

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/proxy"
	"golang.org/x/sync/singleflight"
)

type Adaptor struct {
}

type Credentials struct {
	ID         string `json:"id"`
	SecureCSes string `json:"secure_c_ses"`
	HostCOses  string `json:"host_c_oses"`
	Csesidx    string `json:"csesidx"`
	ConfigID   string `json:"config_id"`
}

type cachedJWT struct {
	Token     string
	ExpiresAt time.Time
}

type replyPiece struct {
	Text      string
	IsThought bool
}

type imagePayload struct {
	Mime string
	Data string
}

var (
	jwtCache     sync.Map
	jwtGroup     singleflight.Group
	clientCache  sync.Map
	dataURLExp   = regexp.MustCompile(`^data:(image/[^;]+);base64,(.+)$`)
	errStopParse = errors.New("stop_parse")
)

func getGeminiBusinessHttpClient(proxyURL string) (*http.Client, error) {
	// IMPORTANT:
	// - Reuse client/transport to keep connections hot; creating a new Transport per request will
	//   quickly degrade under high RPM (too many TCP/TLS handshakes, goroutines, fd pressure).
	// - Do NOT set client.Timeout here. Use per-request context deadlines instead so the shared client
	//   remains safe under concurrency.
	cacheKey := strings.TrimSpace(proxyURL)
	if v, ok := clientCache.Load(cacheKey); ok {
		if cli, ok := v.(*http.Client); ok && cli != nil {
			return cli, nil
		}
	}

	// Gemini Business 上游为 Google 域名，HTTP/2 通常能更好地复用连接并降低高并发下的连接抖动。
	transport := &http.Transport{
		// Align with the project's shared HTTP client defaults so operators can tune pool size globally.
		MaxIdleConns:        common.RelayMaxIdleConns,
		MaxIdleConnsPerHost: common.RelayMaxIdleConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		// Most other channels use HTTP/2 (service.InitHttpClient sets ForceAttemptHTTP2=true).
		// HTTP/2 multiplexing can significantly reduce connection churn under high RPM.
		ForceAttemptHTTP2: true,
		// 给上游首包/握手设置合理的超时，避免并发时出现长时间卡在连接/首包阶段。
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	if cacheKey != "" {
		parsed, err := url.Parse(cacheKey)
		if err != nil {
			return nil, err
		}
		switch parsed.Scheme {
		case "http", "https":
			transport.Proxy = http.ProxyURL(parsed)
		case "socks5", "socks5h":
			var auth *proxy.Auth
			if parsed.User != nil {
				auth = &proxy.Auth{User: parsed.User.Username()}
				if password, ok := parsed.User.Password(); ok {
					auth.Password = password
				}
			}
			dialer, err := proxy.SOCKS5("tcp", parsed.Host, auth, proxy.Direct)
			if err != nil {
				return nil, err
			}
			transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsed.Scheme)
		}
	}

	cli := &http.Client{Transport: transport}
	actual, _ := clientCache.LoadOrStore(cacheKey, cli)
	if cached, ok := actual.(*http.Client); ok && cached != nil {
		return cached, nil
	}
	return cli, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	// nothing to init
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return discoveryBaseURL + streamAssistPath, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	// 直接复用 OpenAI 请求
	return request, nil
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	oai := geminiToOpenAIRequest(request, info.OriginModelName)
	return oai, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	oa := openai.Adaptor{}
	return oa.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.UpstreamModelName == "" {
		info.UpstreamModelName = info.OriginModelName
	}

	t0 := time.Now()
	body, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("read request body failed: %w", err)
	}

	var openAIReq dto.GeneralOpenAIRequest
	if err := common.Unmarshal(body, &openAIReq); err != nil {
		return nil, fmt.Errorf("parse request failed: %w", err)
	}
	// Gemini Business 上游本身永远是“流式数组输出”，但下游是否需要 SSE 要看请求参数。
	// 这里的 downstreamStream 仅表示“是否以流式返回给客户端”，不要与上游响应形态混淆，
	// 否则会导致非流请求不设置超时（出现你反馈的超时无效问题）。
	downstreamStream := openAIReq.Stream
	if strings.Contains(info.RequestURLPath, "stream") || strings.ToLower(c.Query("alt")) == "sse" {
		downstreamStream = true
	}
	info.IsStream = downstreamStream

	cred, err := parseCredentials(info.ApiKey)
	if err != nil {
		return nil, err
	}

	client, err := getGeminiBusinessHttpClient(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}

	// End-to-end timeout (non-stream only):
	// Gemini Business is a multi-step upstream flow (JWT -> createSession -> streamAssist -> parse body).
	// Using http.Client.Timeout would apply per HTTP call; under load the sum of steps could exceed the
	// configured timeout. We instead enforce a single overall deadline via context.
	ctx := c.Request.Context()
	var cancel context.CancelFunc
	if !downstreamStream && common.RelayTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(common.RelayTimeout)*time.Second)
		defer cancel()
	}

	t1 := time.Now()
	jwt, err := getJWT(client, ctx, cred, info)
	if err != nil {
		return nil, err
	}

	t2 := time.Now()
	session, err := createSession(client, ctx, cred, jwt)
	if err != nil {
		return nil, err
	}

	t3 := time.Now()
	promptText, images, err := extractPayload(c, openAIReq)
	if err != nil {
		return nil, err
	}

	fileIDs := make([]string, 0, len(images))
	for _, img := range images {
		fileID, err := uploadContextFile(client, ctx, cred, jwt, session, img)
		if err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileID)
	}

	t4 := time.Now()
	bodyBytes, err := buildStreamAssistBody(cred, session, info.UpstreamModelName, promptText, fileIDs)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discoveryBaseURL+streamAssistPath, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	for k, v := range getCommonHeaders(jwt) {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if common.DebugEnabled {
		logger.LogInfo(c, fmt.Sprintf("[gemini-business] timings parse=%s jwt=%s createSession=%s payload=%s send=%s total=%s status=%d",
			t1.Sub(t0),
			t2.Sub(t1),
			t3.Sub(t2),
			t4.Sub(t3),
			time.Since(t4),
			time.Since(t0),
			resp.StatusCode,
		))
	}

	// 写入上下文供 DoResponse 使用
	info.SetEstimatePromptTokens(service.CountTextToken(promptText, info.UpstreamModelName))
	c.Set("gb_prompt_text", promptText)
	c.Set("gb_request_created", time.Now().Unix())
	c.Set("gb_credential_id", cred.ID)

	return resp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, apiErr *types.NewAPIError) {
	if resp == nil {
		return nil, types.NewOpenAIError(errors.New("nil response"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	id := helper.GetResponseID(c)
	created := c.GetInt64("gb_request_created")
	if created == 0 {
		created = time.Now().Unix()
	}

	var completionBuilder strings.Builder
	var reasoningBuilder strings.Builder

	if info.IsStream {
		streamMode := detectGeminiStreamMode(c)
		if info.RelayFormat == types.RelayFormatGemini && streamMode == "sse" {
			helper.SetEventStreamHeaders(c)
		} else if info.RelayFormat == types.RelayFormatGemini && streamMode == "ndjson" {
			c.Writer.Header().Set("Content-Type", "application/x-ndjson")
		} else {
			helper.SetEventStreamHeaders(c)
		}

		startResp := helper.GenerateStartEmptyResponse(id, created, info.UpstreamModelName, nil)
		if err := a.writeStreamChunk(c, info, startResp, streamMode); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}

		// 使用低延迟解析器，避免等待对象后的逗号/数组闭合才开始下发
		err := parseJSONArrayStreamLoose(resp.Body, func(obj map[string]any) error {
			if apiErr := detectUpstreamError(obj); apiErr != nil {
				return apiErr
			}
			segments := extractReplyPieces(obj)
			for _, seg := range segments {
				delta := dto.ChatCompletionsStreamResponseChoiceDelta{Role: "assistant"}
				if seg.IsThought {
					delta.SetReasoningContent(seg.Text)
					reasoningBuilder.WriteString(seg.Text)
				} else {
					delta.SetContentString(seg.Text)
					completionBuilder.WriteString(seg.Text)
				}
				chunk := &dto.ChatCompletionsStreamResponse{
					Id:      id,
					Object:  "chat.completion.chunk",
					Created: created,
					Model:   info.UpstreamModelName,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: delta,
						},
					},
				}
				if err := a.writeStreamChunk(c, info, chunk, streamMode); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}

		finishReason := "stop"
		stopResp := helper.GenerateStopResponse(id, created, info.UpstreamModelName, finishReason)
		if err := a.writeStreamChunk(c, info, stopResp, streamMode); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if info.RelayFormat != types.RelayFormatGemini || streamMode == "sse" {
			helper.Done(c)
		}

		responseText := completionBuilder.String() + reasoningBuilder.String()
		usageObj := service.ResponseText2Usage(c, responseText, info.UpstreamModelName, info.GetEstimatePromptTokens())
		if reasoningBuilder.Len() > 0 {
			usageObj.CompletionTokenDetails.ReasoningTokens = service.CountTextToken(reasoningBuilder.String(), info.UpstreamModelName)
		}
		return usageObj, nil
	}

	// 非流式：完整读取并聚合
	// 非流式也使用同一解析器，避免尾部事件导致的额外等待
	err := parseJSONArrayStreamLoose(resp.Body, func(obj map[string]any) error {
		if apiErr := detectUpstreamError(obj); apiErr != nil {
			return apiErr
		}
		segments := extractReplyPieces(obj)
		for _, seg := range segments {
			if seg.IsThought {
				reasoningBuilder.WriteString(seg.Text)
			} else {
				completionBuilder.WriteString(seg.Text)
			}
		}
		// 如果上游明确标记完成，则不必等待数组完全结束（有些场景会继续输出无关事件导致额外等待）
		if completionBuilder.Len() > 0 && isStreamAssistComplete(obj) {
			return errStopParse
		}
		return nil
	})
	if err != nil {
		// For Gemini Business non-stream, timeout should be treated as a channel error so the upper retry loop
		// can auto-disable this channel and quickly switch to others.
		if service.IsTimeoutError(err) {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCode("channel:timeout"), http.StatusTooManyRequests)
		}
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	msg := dto.Message{
		Role:             "assistant",
		Content:          completionBuilder.String(),
		ReasoningContent: reasoningBuilder.String(),
	}
	respObj := dto.OpenAITextResponse{
		Id:      id,
		Object:  "chat.completion",
		Model:   info.UpstreamModelName,
		Created: created,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index:        0,
				Message:      msg,
				FinishReason: "stop",
			},
		},
	}
	respObj.Usage = *service.ResponseText2Usage(c, completionBuilder.String()+reasoningBuilder.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	if reasoningBuilder.Len() > 0 {
		respObj.Usage.CompletionTokenDetails.ReasoningTokens = service.CountTextToken(reasoningBuilder.String(), info.UpstreamModelName)
	}

	var data []byte
	if info.RelayFormat == types.RelayFormatGemini {
		geminiResp := service.ResponseOpenAI2Gemini(&respObj, info)
		data, err = common.Marshal(geminiResp)
	} else {
		data, err = common.Marshal(respObj)
	}
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	c.Header("Content-Type", "application/json")
	service.IOCopyBytesGracefully(c, nil, data)
	return &respObj.Usage, nil
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func parseCredentials(raw string) (Credentials, error) {
	var cred Credentials
	if err := common.Unmarshal([]byte(strings.TrimSpace(raw)), &cred); err != nil {
		return cred, fmt.Errorf("invalid credentials: %w", err)
	}
	if cred.ID == "" || cred.SecureCSes == "" || cred.Csesidx == "" || cred.ConfigID == "" || cred.HostCOses == "" {
		return cred, fmt.Errorf("credentials must include id, secure_c_ses, host_c_oses, csesidx, config_id")
	}
	return cred, nil
}

func getJWT(client *http.Client, ctx context.Context, cred Credentials, info *relaycommon.RelayInfo) (string, error) {
	cacheKey := fmt.Sprintf("gb:%d:%d:%s", info.ChannelId, info.ChannelMultiKeyIndex, cred.ID)
	if val, ok := jwtCache.Load(cacheKey); ok {
		if cached, ok := val.(cachedJWT); ok && cached.ExpiresAt.After(time.Now().Add(10*time.Second)) {
			return cached.Token, nil
		}
	}

	v, err, _ := jwtGroup.Do(cacheKey, func() (any, error) {
		// double-check after acquiring singleflight
		if val, ok := jwtCache.Load(cacheKey); ok {
			if cached, ok := val.(cachedJWT); ok && cached.ExpiresAt.After(time.Now().Add(10*time.Second)) {
				return cached.Token, nil
			}
		}
		token, expiresAt, err := refreshJWT(client, ctx, cred)
		if err != nil {
			return "", err
		}
		jwtCache.Store(cacheKey, cachedJWT{Token: token, ExpiresAt: expiresAt})
		return token, nil
	})
	if err != nil {
		return "", err
	}
	token, _ := v.(string)
	if token == "" {
		return "", errors.New("empty jwt")
	}
	return token, nil
}

func refreshJWT(client *http.Client, ctx context.Context, cred Credentials) (string, time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, businessBaseURL+getXsrfPath, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	q := req.URL.Query()
	q.Set("csesidx", cred.Csesidx)
	req.URL.RawQuery = q.Encode()
	cookie := fmt.Sprintf("__Secure-C_SES=%s", cred.SecureCSes)
	if cred.HostCOses != "" {
		cookie += fmt.Sprintf("; __Host-C_OSES=%s", cred.HostCOses)
	}
	req.Header.Set("cookie", cookie)
	req.Header.Set("user-agent", userAgent)
	req.Header.Set("referer", businessBaseURL+"/")

	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("getoxsrf failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, err
	}
	txt := strings.TrimSpace(string(body))
	if strings.HasPrefix(txt, ")]}'") {
		txt = txt[4:]
	}
	var data struct {
		XsrfToken string `json:"xsrfToken"`
		KeyID     string `json:"keyId"`
	}
	if err := json.Unmarshal([]byte(txt), &data); err != nil {
		return "", time.Time{}, err
	}

	keyBytes, err := decodeXsrfToken(data.XsrfToken)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("decode xsrf token failed: %w", err)
	}
	jwt := createJWT(keyBytes, data.KeyID, cred.Csesidx)
	return jwt, time.Now().Add(4 * time.Minute), nil
}

func createJWT(keyBytes []byte, keyID, csesidx string) string {
	now := time.Now().Unix()
	header := map[string]any{
		"alg": "HS256",
		"typ": "JWT",
		"kid": keyID,
	}
	payload := map[string]any{
		"iss": businessBaseURL,
		"aud": discoveryBaseURL,
		"sub": fmt.Sprintf("csesidx/%s", csesidx),
		"iat": now,
		"exp": now + 300,
		"nbf": now,
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	headerB64 := urlsafeB64Encode(kqEncode(string(headerJSON)))
	payloadB64 := urlsafeB64Encode(kqEncode(string(payloadJSON)))
	message := headerB64 + "." + payloadB64

	mac := hmac.New(sha256.New, keyBytes)
	mac.Write([]byte(message))
	signature := urlsafeB64Encode(mac.Sum(nil))
	return message + "." + signature
}

func decodeXsrfToken(token string) ([]byte, error) {
	tryDecode := func(dec func(string) ([]byte, error), s string) ([]byte, error) {
		return dec(s)
	}

	// 1) url-safe, no padding
	if b, err := tryDecode(base64.RawURLEncoding.DecodeString, token); err == nil {
		return b, nil
	}
	// 2) url-safe with padding
	padded := token
	if m := len(padded) % 4; m != 0 {
		padded += strings.Repeat("=", 4-m)
	}
	if b, err := tryDecode(base64.URLEncoding.DecodeString, padded); err == nil {
		return b, nil
	}
	// 3) std, no padding
	if b, err := tryDecode(base64.RawStdEncoding.DecodeString, token); err == nil {
		return b, nil
	}
	// 4) std with padding
	if b, err := tryDecode(base64.StdEncoding.DecodeString, padded); err == nil {
		return b, nil
	}
	return nil, fmt.Errorf("unsupported xsrf token format")
}

func urlsafeB64Encode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

func kqEncode(s string) []byte {
	buf := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 255 {
			buf = append(buf, byte(r&0xff))
			buf = append(buf, byte(r>>8))
		} else {
			buf = append(buf, byte(r))
		}
	}
	return buf
}

func getCommonHeaders(jwt string) map[string]string {
	return map[string]string{
		"accept": "*/*",
		// 不要手动设置 accept-encoding：
		// Go 的 http.Transport 只有在“自动添加 gzip”时才会自动解压；
		// 若我们手动声明 br/zstd，上游可能返回 Go 无法解压的压缩数据，导致 JSON 解析报错（如 \x1f）。
		"accept-language":    "zh-CN,zh;q=0.9,en;q=0.8",
		"authorization":      "Bearer " + jwt,
		"content-type":       "application/json",
		"origin":             businessBaseURL,
		"referer":            businessBaseURL + "/",
		"user-agent":         userAgent,
		"x-server-timeout":   "1800",
		"sec-ch-ua":          `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`,
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "cross-site",
	}
}

func createSession(client *http.Client, ctx context.Context, cred Credentials, jwt string) (string, error) {
	body := map[string]any{
		"configId": cred.ConfigID,
		"additionalParams": map[string]any{
			"token": "-",
		},
		"createSessionRequest": map[string]any{
			"session": map[string]any{
				"name":        "",
				"displayName": "",
			},
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discoveryBaseURL+createSessionPath, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	for k, v := range getCommonHeaders(jwt) {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create session failed: %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var data struct {
		Session struct {
			Name string `json:"name"`
		} `json:"session"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", err
	}
	if data.Session.Name == "" {
		return "", errors.New("session name empty")
	}
	return data.Session.Name, nil
}

func uploadContextFile(client *http.Client, ctx context.Context, cred Credentials, jwt, session string, img imagePayload) (string, error) {
	ext := guessExtension(img.Mime)
	// 使用纳秒时间戳确保唯一（避免并发下 rand 竞争/数据竞争）
	fileName := fmt.Sprintf("upload_%d.%s", time.Now().UnixNano(), ext)

	body := map[string]any{
		"configId": cred.ConfigID,
		"additionalParams": map[string]any{
			"token": "-",
		},
		"addContextFileRequest": map[string]any{
			"name":         session,
			"fileName":     fileName,
			"mimeType":     img.Mime,
			"fileContents": img.Data,
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discoveryBaseURL+addFilePath, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	for k, v := range getCommonHeaders(jwt) {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload context file failed: %d", resp.StatusCode)
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var data struct {
		AddContextFileResponse struct {
			FileID string `json:"fileId"`
		} `json:"addContextFileResponse"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", err
	}
	if data.AddContextFileResponse.FileID == "" {
		return "", errors.New("missing fileId in response")
	}
	return data.AddContextFileResponse.FileID, nil
}

func buildStreamAssistBody(cred Credentials, session, modelName, text string, fileIDs []string) ([]byte, error) {
	body := map[string]any{
		"configId": cred.ConfigID,
		"additionalParams": map[string]any{
			"token": "-",
		},
		"streamAssistRequest": map[string]any{
			"session": session,
			"query": map[string]any{
				"parts": []map[string]any{
					{"text": text},
				},
			},
			"filter":               "",
			"fileIds":              fileIDs,
			"answerGenerationMode": "NORMAL",
			"toolsSpec": map[string]any{
				"webGroundingSpec":    map[string]any{},
				"toolRegistry":        "default_tool_registry",
				"imageGenerationSpec": map[string]any{},
				"videoGenerationSpec": map[string]any{},
			},
			"languageCode":       "zh-CN",
			"userMetadata":       map[string]any{"timeZone": "Asia/Shanghai"},
			"assistSkippingMode": "REQUEST_ASSIST",
		},
	}
	if mapped, ok := modelMapping[modelName]; ok && mapped != "" {
		body["streamAssistRequest"].(map[string]any)["assistGenerationConfig"] = map[string]any{
			"modelId": mapped,
		}
	}
	return json.Marshal(body)
}

func extractPayload(c *gin.Context, req dto.GeneralOpenAIRequest) (string, []imagePayload, error) {
	text, images := buildFullContextText(req.Messages)
	resolvedImages := make([]imagePayload, 0, len(images))
	for _, img := range images {
		// 支持 data url
		if matches := dataURLExp.FindStringSubmatch(img); len(matches) == 3 {
			resolvedImages = append(resolvedImages, imagePayload{
				Mime: matches[1],
				Data: matches[2],
			})
			continue
		}
		// 尝试下载远程图片
		fileData, err := service.GetFileBase64FromUrl(c, img, "gemini_business_image")
		if err != nil {
			logger.LogError(c, fmt.Sprintf("download image failed: %v", err))
			continue
		}
		resolvedImages = append(resolvedImages, imagePayload{
			Mime: fileData.MimeType,
			Data: fileData.Base64Data,
		})
	}
	return text, resolvedImages, nil
}

func buildFullContextText(messages []dto.Message) (string, []string) {
	var prompt strings.Builder
	var images []string
	for _, msg := range messages {
		role := "User"
		if msg.Role != "" && msg.Role != "user" && msg.Role != "system" {
			role = "Assistant"
		}
		var contentStr string
		if msg.IsStringContent() {
			contentStr = msg.StringContent()
		} else {
			for _, part := range msg.ParseContent() {
				switch part.Type {
				case dto.ContentTypeText:
					contentStr += part.Text
				case dto.ContentTypeImageURL:
					if media := part.GetImageMedia(); media != nil {
						images = append(images, media.Url)
						contentStr += "[图片]"
					}
				}
			}
		}
		prompt.WriteString(fmt.Sprintf("%s: %s\n\n", role, contentStr))
	}
	return prompt.String(), images
}

func guessExtension(mime string) string {
	if mime == "" {
		return "bin"
	}
	if ext := filepath.Ext("file." + strings.TrimPrefix(mime, "image/")); ext != "" {
		return strings.TrimPrefix(ext, ".")
	}
	switch mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return "bin"
	}
}

func parseJSONArrayStream(body io.Reader, handle func(map[string]any) error) error {
	decoder := json.NewDecoder(body)
	// 期望数组开头
	t, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(json.Delim); !ok || delim != '[' {
		return errors.New("response not a JSON array")
	}
	for decoder.More() {
		var obj map[string]any
		if err := decoder.Decode(&obj); err != nil {
			return err
		}
		if err := handle(obj); err != nil {
			if errors.Is(err, errStopParse) {
				return nil
			}
			return err
		}
	}
	return nil
}

// parseJSONArrayStreamLoose 以“低延迟优先”的方式解析 JSON 数组流：
// - 不要求每个对象后面的逗号/数组结束符已到达，就可以尽早产出对象
// - 行为对齐 Python 版本的 brace-level 流解析器
func parseJSONArrayStreamLoose(body io.Reader, handle func(map[string]any) error) error {
	r := bufio.NewReaderSize(body, 32*1024)

	// 1) 找到数组起始符 '['
	for {
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b == '[' {
			break
		}
	}

	var buf bytes.Buffer
	brace := 0
	inString := false
	escapeNext := false

	flushObj := func() error {
		if buf.Len() == 0 {
			return nil
		}
		var obj map[string]any
		if err := common.Unmarshal(buf.Bytes(), &obj); err != nil {
			return err
		}
		buf.Reset()
		inString = false
		escapeNext = false
		if err := handle(obj); err != nil {
			if errors.Is(err, errStopParse) {
				return errStopParse
			}
			return err
		}
		return nil
	}

	for {
		ch, err := r.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				// EOF 时若刚好有完整对象已缓冲，也尽量尝试 flush
				if brace == 0 {
					_ = flushObj()
				}
				return nil
			}
			return err
		}

		if brace == 0 {
			// 忽略对象外的分隔符与空白
			switch ch {
			case '{':
				brace = 1
				buf.Reset()
				buf.WriteByte(ch)
			case ']':
				return nil
			default:
				continue
			}
			continue
		}

		// brace > 0: 在对象内部，逐字节累积
		buf.WriteByte(ch)

		if escapeNext {
			escapeNext = false
			continue
		}
		if ch == '\\' {
			escapeNext = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{':
			brace++
		case '}':
			brace--
			if brace == 0 {
				if err := flushObj(); err != nil {
					if errors.Is(err, errStopParse) {
						return nil
					}
					return err
				}
			}
		}
	}
}

func isStreamAssistComplete(obj map[string]any) bool {
	streamResp, ok := obj["streamAssistResponse"].(map[string]any)
	if !ok {
		return false
	}
	answer, ok := streamResp["answer"].(map[string]any)
	if !ok {
		return false
	}
	// 兼容不同上游字段：有些环境不返回 complete/done/finished，而是 state/status/finishReason。
	for _, k := range []string{"complete", "done", "finished"} {
		if v, ok := answer[k]; ok {
			switch vv := v.(type) {
			case bool:
				return vv
			case string:
				return strings.EqualFold(strings.TrimSpace(vv), "true")
			}
		}
	}
	for _, k := range []string{"state", "status"} {
		if v, ok := answer[k]; ok {
			s := strings.ToUpper(strings.TrimSpace(common.Interface2String(v)))
			if s == "DONE" || s == "COMPLETED" || s == "FINISHED" || s == "COMPLETE" || s == "SUCCEEDED" || s == "SUCCESS" {
				return true
			}
		}
	}
	if v, ok := answer["finishReason"]; ok {
		s := strings.ToUpper(strings.TrimSpace(common.Interface2String(v)))
		if s != "" {
			return true
		}
	}
	return false
}

func extractReplyPieces(obj map[string]any) []replyPiece {
	var pieces []replyPiece
	streamResp, ok := obj["streamAssistResponse"].(map[string]any)
	if !ok {
		return pieces
	}
	answer, ok := streamResp["answer"].(map[string]any)
	if !ok {
		return pieces
	}
	replies, ok := answer["replies"].([]any)
	if !ok {
		return pieces
	}
	for _, r := range replies {
		replyMap, ok := r.(map[string]any)
		if !ok {
			continue
		}
		grounded, _ := replyMap["groundedContent"].(map[string]any)
		content, _ := grounded["content"].(map[string]any)
		text := common.Interface2String(content["text"])
		if text == "" {
			continue
		}
		isThought := false
		if val, ok := content["thought"]; ok {
			switch v := val.(type) {
			case bool:
				isThought = v
			case string:
				isThought = strings.EqualFold(v, "true")
			}
		}
		pieces = append(pieces, replyPiece{
			Text:      text,
			IsThought: isThought,
		})
	}
	return pieces
}

func detectUpstreamError(obj map[string]any) error {
	if errObj, ok := obj["error"].(map[string]any); ok {
		code := common.Interface2String(errObj["code"])
		msg := common.Interface2String(errObj["message"])
		if code == "" {
			code = "upstream_error"
		}
		return fmt.Errorf("%s: %s", code, msg)
	}
	return nil
}

func (a *Adaptor) writeStreamChunk(c *gin.Context, info *relaycommon.RelayInfo, chunk *dto.ChatCompletionsStreamResponse, mode string) error {
	if info.RelayFormat == types.RelayFormatGemini {
		geminiResp := service.StreamResponseOpenAI2Gemini(chunk, info)
		if geminiResp == nil {
			return nil
		}
		data, err := common.Marshal(geminiResp)
		if err != nil {
			return err
		}
		if mode == "ndjson" {
			if _, err := c.Writer.Write(append(data, '\n')); err != nil {
				return err
			}
			return helper.FlushWriter(c)
		}
		return helper.StringData(c, string(data))
	}
	data, err := common.Marshal(chunk)
	if err != nil {
		return err
	}
	return helper.StringData(c, string(data))
}

func detectGeminiStreamMode(c *gin.Context) string {
	accept := strings.ToLower(c.GetHeader("accept"))
	if strings.Contains(accept, "application/x-ndjson") || strings.ToLower(c.Query("alt")) == "ndjson" {
		return "ndjson"
	}
	return "sse"
}

func geminiToOpenAIRequest(req *dto.GeminiChatRequest, modelName string) *dto.GeneralOpenAIRequest {
	messages := make([]dto.Message, 0, len(req.Contents))
	for _, content := range req.Contents {
		role := content.Role
		if role == "" {
			role = "user"
		}
		parts := make([]dto.MediaContent, 0, len(content.Parts))
		for _, part := range content.Parts {
			if part.Text != "" {
				parts = append(parts, dto.MediaContent{
					Type: dto.ContentTypeText,
					Text: part.Text,
				})
			}
			if part.InlineData != nil && part.InlineData.Data != "" {
				dataURL := fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)
				parts = append(parts, dto.MediaContent{
					Type: dto.ContentTypeImageURL,
					ImageUrl: &dto.MessageImageUrl{
						Url:      dataURL,
						Detail:   "high",
						MimeType: part.InlineData.MimeType,
					},
				})
			}
		}
		msg := dto.Message{Role: role}
		if len(parts) == 1 && parts[0].Type == dto.ContentTypeText {
			msg.Content = parts[0].Text
		} else {
			msg.SetMediaContent(parts)
		}
		messages = append(messages, msg)
	}

	return &dto.GeneralOpenAIRequest{
		Model:         modelName,
		Messages:      messages,
		Temperature:   req.GenerationConfig.Temperature,
		TopP:          req.GenerationConfig.TopP,
		MaxTokens:     req.GenerationConfig.MaxOutputTokens,
		Stop:          req.GenerationConfig.StopSequences,
		Seed:          float64(req.GenerationConfig.Seed),
		Stream:        false,
		StreamOptions: nil,
	}
}

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
