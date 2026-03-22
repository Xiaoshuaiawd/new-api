package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetModelRateLimitIdentifier(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name     string
		setup    func(c *gin.Context)
		expected string
	}{
		{
			name: "prefer token id over user id",
			setup: func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("token_id", 42)
				c.Set("token_key", "sk-test")
			},
			expected: "token:42",
		},
		{
			name: "fallback to token key hash",
			setup: func(c *gin.Context) {
				c.Set("token_key", "sk-test")
			},
			expected: "token:" + common.GenerateHMAC("sk-test"),
		},
		{
			name: "fallback to user id",
			setup: func(c *gin.Context) {
				c.Set("id", 9)
			},
			expected: "user:9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			tt.setup(ctx)
			require.Equal(t, tt.expected, getModelRateLimitIdentifier(ctx))
		})
	}
}

func TestMemoryRateLimitHandlerUsesTokenScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
		tokenID, _ := strconv.Atoi(c.GetHeader("X-Token-Id"))
		c.Set("token_id", tokenID)
	})
	router.Use(memoryRateLimitHandler(60, 1, 10))
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	firstTokenA := performRateLimitRequest(router, "1", "")
	require.Equal(t, http.StatusOK, firstTokenA.Code)

	firstTokenB := performRateLimitRequest(router, "2", "")
	require.Equal(t, http.StatusOK, firstTokenB.Code)

	secondTokenA := performRateLimitRequest(router, "1", "")
	require.Equal(t, http.StatusTooManyRequests, secondTokenA.Code)
}

func TestMemoryRateLimitHandlerSuccessWindowIgnoresFailedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	inMemoryRateLimiter = common.InMemoryRateLimiter{}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("id", 1)
		c.Set("token_id", 99)
	})
	router.Use(memoryRateLimitHandler(int64(time.Minute.Seconds()), 0, 1))
	router.GET("/v1/chat/completions", func(c *gin.Context) {
		if c.GetHeader("X-Fail") == "1" {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})

	failed := performRateLimitRequest(router, "99", "1")
	require.Equal(t, http.StatusInternalServerError, failed.Code)

	firstSuccess := performRateLimitRequest(router, "99", "")
	require.Equal(t, http.StatusOK, firstSuccess.Code)

	secondSuccess := performRateLimitRequest(router, "99", "")
	require.Equal(t, http.StatusTooManyRequests, secondSuccess.Code)
}

func performRateLimitRequest(router http.Handler, tokenID string, fail string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	if tokenID != "" {
		req.Header.Set("X-Token-Id", tokenID)
	}
	if fail != "" {
		req.Header.Set("X-Fail", fail)
	}
	router.ServeHTTP(recorder, req)
	return recorder
}
