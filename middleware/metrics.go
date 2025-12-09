package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// 渠道和模型调用总数（按状态分类）
	modelRequestsTotal *prometheus.CounterVec

	// 渠道和模型调用响应时间
	modelRequestDuration *prometheus.HistogramVec

	// 渠道和模型错误详情（仅失败请求）
	modelRequestErrors *prometheus.CounterVec

	// 渠道在线状态
	channelStatus *prometheus.GaugeVec

	// 当前活跃请求数
	activeRequests *prometheus.GaugeVec

	// Prometheus registry
	registry *prometheus.Registry

	// 初始化锁
	metricsInitOnce sync.Once

	// 是否启用 Prometheus
	prometheusEnabled bool
)

// InitPrometheusMetrics 初始化 Prometheus metrics
func InitPrometheusMetrics() {
	metricsInitOnce.Do(func() {
		// 检查是否启用 Prometheus
		if os.Getenv("PROMETHEUS_ENABLED") != "true" {
			prometheusEnabled = false
			return
		}
		prometheusEnabled = true

		// 创建新的 registry（避免与默认 registry 冲突）
		registry = prometheus.NewRegistry()

		// 获取站点 ID
		siteID := os.Getenv("SITE_ID")
		if siteID == "" {
			siteID = "default"
		}

		// 1. 渠道和模型调用总数
		modelRequestsTotal = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "new_api_model_requests_total",
				Help: "Total number of model requests by channel and model",
			},
			[]string{
				"channel_id",      // 渠道ID
				"channel_name",    // 渠道名称
				"channel_type",    // 渠道类型
				"model_name",      // 模型名称
				"status",          // success|failed
				"error_code",      // HTTP错误码
				"site_id",         // 站点ID
			},
		)

		// 2. 渠道和模型调用响应时间
		modelRequestDuration = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "new_api_model_request_duration_seconds",
				Help: "Model request duration in seconds",
				Buckets: []float64{
					0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0,
				}, // 100ms, 250ms, 500ms, 1s, 2.5s, 5s, 10s, 30s, 1min, 2min, 5min
			},
			[]string{
				"channel_id",
				"channel_name",
				"channel_type",
				"model_name",
				"site_id",
			},
		)

		// 3. 渠道和模型错误详情
		modelRequestErrors = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "new_api_model_request_errors_total",
				Help: "Total number of model request errors with details",
			},
			[]string{
				"channel_id",
				"channel_name",
				"channel_type",
				"model_name",
				"error_code",      // HTTP错误码
				"error_message",   // 错误信息摘要（限制长度）
				"site_id",
			},
		)

		// 4. 渠道在线状态
		channelStatus = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "new_api_channel_status",
				Help: "Channel status: 1=enabled, 2=manually disabled, 3=auto disabled, 4=deleted",
			},
			[]string{
				"channel_id",
				"channel_name",
				"channel_type",
				"status",          // online|offline|testing
				"site_id",
			},
		)

		// 5. 当前活跃请求数
		activeRequests = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "new_api_active_requests",
				Help: "Current number of active requests",
			},
			[]string{
				"channel_id",
				"channel_name",
				"channel_type",
				"model_name",
				"site_id",
			},
		)

		// 注册所有 metrics
		registry.MustRegister(
			modelRequestsTotal,
			modelRequestDuration,
			modelRequestErrors,
			channelStatus,
			activeRequests,
		)
	})
}

// PrometheusHandler 返回 Prometheus HTTP handler
func PrometheusHandler() gin.HandlerFunc {
	if !prometheusEnabled {
		return func(c *gin.Context) {
			c.String(http.StatusNotFound, "Prometheus metrics disabled")
		}
	}

	handler := promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})

	return func(c *gin.Context) {
		handler.ServeHTTP(c.Writer, c.Request)
	}
}

// MetricsContext 请求的 metrics 上下文
type MetricsContext struct {
	ChannelID   int
	ChannelName string
	ChannelType int
	ModelName   string
	StartTime   time.Time
	SiteID      string
}

// RecordRequestStart 记录请求开始
func RecordRequestStart(ctx *MetricsContext) {
	if !prometheusEnabled || ctx == nil {
		return
	}

	ctx.StartTime = time.Now()
	if ctx.SiteID == "" {
		ctx.SiteID = os.Getenv("SITE_ID")
		if ctx.SiteID == "" {
			ctx.SiteID = "default"
		}
	}

	// 增加活跃请求数
	activeRequests.WithLabelValues(
		strconv.Itoa(ctx.ChannelID),
		ctx.ChannelName,
		getChannelTypeName(ctx.ChannelType),
		ctx.ModelName,
		ctx.SiteID,
	).Inc()
}

// RecordRequestEnd 记录请求结束
func RecordRequestEnd(ctx *MetricsContext, statusCode int, errorMessage string) {
	if !prometheusEnabled || ctx == nil {
		return
	}

	if ctx.SiteID == "" {
		ctx.SiteID = os.Getenv("SITE_ID")
		if ctx.SiteID == "" {
			ctx.SiteID = "default"
		}
	}

	// 减少活跃请求数
	activeRequests.WithLabelValues(
		strconv.Itoa(ctx.ChannelID),
		ctx.ChannelName,
		getChannelTypeName(ctx.ChannelType),
		ctx.ModelName,
		ctx.SiteID,
	).Dec()

	// 记录响应时间
	duration := time.Since(ctx.StartTime).Seconds()
	modelRequestDuration.WithLabelValues(
		strconv.Itoa(ctx.ChannelID),
		ctx.ChannelName,
		getChannelTypeName(ctx.ChannelType),
		ctx.ModelName,
		ctx.SiteID,
	).Observe(duration)

	// 判断请求是否成功
	status := "success"
	if statusCode >= 400 {
		status = "failed"
	}

	// 记录请求总数
	modelRequestsTotal.WithLabelValues(
		strconv.Itoa(ctx.ChannelID),
		ctx.ChannelName,
		getChannelTypeName(ctx.ChannelType),
		ctx.ModelName,
		status,
		strconv.Itoa(statusCode),
		ctx.SiteID,
	).Inc()

	// 如果是失败请求，记录错误详情
	if status == "failed" {
		// 限制错误信息长度
		truncatedErrorMessage := truncateString(errorMessage, 200)
		modelRequestErrors.WithLabelValues(
			strconv.Itoa(ctx.ChannelID),
			ctx.ChannelName,
			getChannelTypeName(ctx.ChannelType),
			ctx.ModelName,
			strconv.Itoa(statusCode),
			truncatedErrorMessage,
			ctx.SiteID,
		).Inc()
	}
}

// UpdateChannelStatus 更新渠道状态
func UpdateChannelStatus(channelID int, channelName string, channelType int, status int) {
	if !prometheusEnabled {
		return
	}

	siteID := os.Getenv("SITE_ID")
	if siteID == "" {
		siteID = "default"
	}

	statusName := getChannelStatusName(status)

	// 先清除该渠道的所有状态
	channelIDStr := strconv.Itoa(channelID)
	channelTypeName := getChannelTypeName(channelType)

	for _, s := range []string{"online", "manually_disabled", "auto_disabled", "deleted"} {
		channelStatus.DeleteLabelValues(
			channelIDStr,
			channelName,
			channelTypeName,
			s,
			siteID,
		)
	}

	// 设置当前状态
	channelStatus.WithLabelValues(
		channelIDStr,
		channelName,
		channelTypeName,
		statusName,
		siteID,
	).Set(float64(status))
}

// getChannelTypeName 获取渠道类型名称
func getChannelTypeName(channelType int) string {
	// 根据项目中的渠道类型常量映射
	channelTypeMap := map[int]string{
		1:  "openai",
		2:  "api2d",
		3:  "azure",
		4:  "closeai",
		5:  "openai-sb",
		6:  "openai-max",
		7:  "ohmygpt",
		8:  "custom",
		9:  "ali",
		10: "baidu",
		11: "zhipu",
		12: "xunfei",
		13: "360",
		14: "moonshot",
		15: "claude",
		16: "gemini",
		17: "palm",
		18: "tencent",
		19: "aws",
		20: "mistral",
		21: "groq",
		22: "ollama",
		23: "vertexai",
		24: "cohere",
		25: "siliconflow",
		26: "lingyiwanwu",
		27: "deepseek",
		28: "cloudflare",
		29: "coze",
		30: "dify",
		31: "minimax",
		32: "perplexity",
		33: "replicate",
		34: "xai",
		35: "mokaai",
		36: "jina",
		37: "openrouter",
		38: "xinference",
		39: "baidu_v2",
		40: "zhipu_4v",
	}

	if name, ok := channelTypeMap[channelType]; ok {
		return name
	}
	return fmt.Sprintf("type_%d", channelType)
}

// getChannelStatusName 获取渠道状态名称
func getChannelStatusName(status int) string {
	switch status {
	case 1:
		return "online"
	case 2:
		return "manually_disabled"
	case 3:
		return "auto_disabled"
	case 4:
		return "deleted"
	default:
		return "unknown"
	}
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	// 移除换行符和多余空格
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.Join(strings.Fields(s), " ")

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// IsPrometheusEnabled 检查是否启用了 Prometheus
func IsPrometheusEnabled() bool {
	return prometheusEnabled
}
