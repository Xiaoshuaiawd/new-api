package types

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

func TestStatusCodeFromOpenAIError(t *testing.T) {
	t.Run("numeric code string", func(t *testing.T) {
		openAIError := &OpenAIError{Code: "429"}
		if got := StatusCodeFromOpenAIError(openAIError, http.StatusInternalServerError); got != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", got)
		}
	})

	t.Run("metadata status code", func(t *testing.T) {
		metadata, err := common.Marshal(map[string]any{"status_code": 401})
		if err != nil {
			t.Fatalf("marshal metadata failed: %v", err)
		}
		openAIError := &OpenAIError{Metadata: metadata}
		if got := StatusCodeFromOpenAIError(openAIError, http.StatusInternalServerError); got != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", got)
		}
	})

	t.Run("message fallback", func(t *testing.T) {
		openAIError := &OpenAIError{Type: "rate_limit_error", Message: "too many requests"}
		if got := StatusCodeFromOpenAIError(openAIError, http.StatusInternalServerError); got != http.StatusTooManyRequests {
			t.Fatalf("expected 429, got %d", got)
		}
	})
}
