package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestResponsesStreamToChatNonStreamHandler_FallbackWithoutCompletedEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	streamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"model":"gpt-5-codex","created_at":1730000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello "}`,
		`data: {"type":"response.output_text.delta","delta":"world"}`,
		`data: [DONE]`,
	}, "\n")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
		},
		Body: io.NopCloser(strings.NewReader(streamBody)),
	}

	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5-codex",
		},
	}

	usage, err := responsesStreamToChatNonStreamHandler(ctx, info, resp)
	if err != nil {
		t.Fatalf("responsesStreamToChatNonStreamHandler returned error: %v", err)
	}
	if usage == nil {
		t.Fatalf("expected usage, got nil")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"chat.completion"`) {
		t.Fatalf("expected chat completion response, got: %s", body)
	}
	if !strings.Contains(body, `"content":"hello world"`) {
		t.Fatalf("expected merged output text in response, got: %s", body)
	}
}
