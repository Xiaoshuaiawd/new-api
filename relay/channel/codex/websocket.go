package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func useWebSocketUpstream(info *relaycommon.RelayInfo) bool {
	return info != nil &&
		info.RelayMode == relayconstant.RelayModeResponses &&
		operation_setting.GetGeneralSetting().CodexUpstreamWebSocketEnabled
}

func toWebSocketURL(rawURL string) string {
	switch {
	case strings.HasPrefix(rawURL, "https://"):
		return "wss://" + strings.TrimPrefix(rawURL, "https://")
	case strings.HasPrefix(rawURL, "http://"):
		return "ws://" + strings.TrimPrefix(rawURL, "http://")
	default:
		return rawURL
	}
}

func buildResponsesWebSocketRequestPayload(requestBody io.Reader) ([]byte, error) {
	if requestBody == nil {
		return nil, errors.New("codex websocket request body is nil")
	}

	bodyBytes, err := io.ReadAll(requestBody)
	if err != nil {
		return nil, fmt.Errorf("read websocket request body failed: %w", err)
	}
	if strings.TrimSpace(string(bodyBytes)) == "" {
		return nil, errors.New("codex websocket request body is empty")
	}

	payload := make(map[string]json.RawMessage)
	if err := common.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal websocket request body failed: %w", err)
	}

	payload["type"] = json.RawMessage(`"response.create"`)
	delete(payload, "stream")

	websocketBody, err := common.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal websocket request payload failed: %w", err)
	}
	return websocketBody, nil
}

func buildResponsesWebSocketHeaderTemplate(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo) (http.Header, error) {
	targetHeader := http.Header{}
	if err := a.SetupRequestHeader(c, &targetHeader, info); err != nil {
		return nil, fmt.Errorf("setup websocket request header failed: %w", err)
	}

	headerOverride, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return nil, err
	}
	for key, value := range headerOverride {
		targetHeader.Set(key, value)
	}

	targetHeader.Del("session_id")
	targetHeader.Del("x-client-request-id")
	targetHeader.Del("x-codex-turn-metadata")

	return targetHeader, nil
}

func normalizeResponsesWebSocketEvent(message []byte) ([]byte, string, error) {
	trimmed := strings.TrimSpace(string(message))
	if trimmed == "" {
		return nil, "", nil
	}

	streamResp := dto.ResponsesStreamResponse{}
	if err := common.UnmarshalJsonStr(trimmed, &streamResp); err == nil && strings.TrimSpace(streamResp.Type) != "" {
		if streamResp.Type != "error" {
			return []byte(trimmed), streamResp.Type, nil
		}
	}

	var envelope struct {
		Type  string          `json:"type"`
		Error json.RawMessage `json:"error,omitempty"`
	}
	if err := common.UnmarshalJsonStr(trimmed, &envelope); err != nil {
		return []byte(trimmed), "", nil
	}
	if envelope.Type != "error" {
		return []byte(trimmed), envelope.Type, nil
	}
	if len(envelope.Error) == 0 {
		return []byte(trimmed), envelope.Type, nil
	}

	normalized := fmt.Sprintf(`{"type":"response.failed","response":{"error":%s}}`, string(envelope.Error))
	return []byte(normalized), "response.failed", nil
}

func isTerminalResponsesWebSocketEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.failed", "response.error":
		return true
	default:
		return false
	}
}

func responsesWebSocketEventStatusCode(message []byte) int {
	streamResp := dto.ResponsesStreamResponse{}
	if err := common.Unmarshal(message, &streamResp); err != nil {
		return http.StatusInternalServerError
	}
	if streamResp.Response == nil {
		return http.StatusInternalServerError
	}
	return types.StatusCodeFromOpenAIError(streamResp.Response.GetOpenAIError(), http.StatusInternalServerError)
}

func shouldRecycleResponsesWebSocketConnection(eventType string, statusCode int) bool {
	if !isTerminalResponsesWebSocketEvent(eventType) {
		return false
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusTooManyRequests {
		return false
	}
	return eventType == "response.completed" || eventType == "response.failed" || eventType == "response.error"
}

func bridgeResponsesWebSocketToSSE(c *gin.Context, info *relaycommon.RelayInfo, borrowedConn *borrowedResponsesWebSocketConn, writer *io.PipeWriter) {
	defer writer.Close()

	if borrowedConn == nil || borrowedConn.Conn() == nil {
		_ = writer.CloseWithError(errors.New("codex websocket connection is nil"))
		return
	}

	ctxDone := make(chan struct{})
	go func() {
		select {
		case <-c.Request.Context().Done():
			borrowedConn.Discard()
		case <-ctxDone:
		}
	}()
	defer close(ctxDone)

	for {
		messageType, message, err := borrowedConn.Conn().ReadMessage()
		if err != nil {
			borrowedConn.Discard()
			if c.Request.Context().Err() != nil {
				return
			}
			_ = writer.CloseWithError(fmt.Errorf("read websocket message failed: %w", err))
			return
		}

		if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
			continue
		}

		normalizedMessage, eventType, err := normalizeResponsesWebSocketEvent(message)
		if err != nil {
			borrowedConn.Discard()
			_ = writer.CloseWithError(fmt.Errorf("normalize websocket message failed: %w", err))
			return
		}
		if len(normalizedMessage) == 0 {
			continue
		}

		if _, err = io.WriteString(writer, "data: "+string(normalizedMessage)+"\n\n"); err != nil {
			borrowedConn.Discard()
			return
		}

		if isTerminalResponsesWebSocketEvent(eventType) {
			statusCode := responsesWebSocketEventStatusCode(normalizedMessage)
			if _, err = io.WriteString(writer, "data: [DONE]\n\n"); err != nil {
				borrowedConn.Discard()
				return
			}
			if shouldRecycleResponsesWebSocketConnection(eventType, statusCode) {
				borrowedConn.Release()
			} else {
				borrowedConn.Discard()
				if statusCode == http.StatusUnauthorized || statusCode == http.StatusTooManyRequests {
					CloseResponsesWebSocketPoolsForChannel(info.ChannelId)
				}
			}
			return
		}
	}
}

func doResponsesWebSocketRequest(a *Adaptor, c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get websocket request url failed: %w", err)
	}

	websocketPayload, err := buildResponsesWebSocketRequestPayload(requestBody)
	if err != nil {
		return nil, err
	}

	headerTemplate, err := buildResponsesWebSocketHeaderTemplate(a, c, info)
	if err != nil {
		return nil, err
	}

	signature := buildResponsesWebSocketPoolSignature(fullRequestURL, info.ApiKey, info.ChannelSetting.Proxy)
	pool := getOrCreateResponsesWebSocketPool(info.ChannelId, signature, fullRequestURL, headerTemplate, info.ChannelSetting.Proxy)

	borrowedConn, err := pool.Acquire(c.Request.Context())
	if err != nil {
		var dialErr *responsesWebSocketDialError
		if errors.As(err, &dialErr) && dialErr.response != nil {
			if dialErr.response.StatusCode == http.StatusUnauthorized || dialErr.response.StatusCode == http.StatusTooManyRequests {
				CloseResponsesWebSocketPoolsForChannel(info.ChannelId)
			}
			return dialErr.response, nil
		}
		return nil, fmt.Errorf("acquire websocket connection failed: %w", err)
	}

	if err := borrowedConn.Conn().WriteMessage(websocket.TextMessage, websocketPayload); err != nil {
		borrowedConn.Discard()

		borrowedConn, err = pool.Acquire(c.Request.Context())
		if err != nil {
			var dialErr *responsesWebSocketDialError
			if errors.As(err, &dialErr) && dialErr.response != nil {
				if dialErr.response.StatusCode == http.StatusUnauthorized || dialErr.response.StatusCode == http.StatusTooManyRequests {
					CloseResponsesWebSocketPoolsForChannel(info.ChannelId)
				}
				return dialErr.response, nil
			}
			return nil, fmt.Errorf("reacquire websocket connection failed: %w", err)
		}
		if err = borrowedConn.Conn().WriteMessage(websocket.TextMessage, websocketPayload); err != nil {
			borrowedConn.Discard()
			return nil, fmt.Errorf("write websocket request payload failed: %w", err)
		}
	}

	reader, writer := io.Pipe()
	go bridgeResponsesWebSocketToSSE(c, info, borrowedConn, writer)

	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: reader,
	}, nil
}
