// metrics 包负责注册和导出 Prometheus 指标。
package metrics

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	rollingWindow    = time.Minute
	rollingBucket    = 5 * time.Second
	rollingBucketNum = int(rollingWindow / rollingBucket)
)

var (
	// API 级别请求次数与延迟直方图
	apiRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "app_api_request_total",
			Help: "Total number of API requests grouped by path, method and status.",
		},
		[]string{"path", "method", "status"},
	)

	apiRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "app_api_request_duration_seconds",
			Help:    "Latency distribution for API requests.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 8), // 50ms ~ 6.4s
		},
		[]string{"path", "method"},
	)

	// 渠道成功/失败计数器
	channelRequestTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "channel_request_total",
			Help: "Total number of downstream channel requests grouped by status.",
		},
		[]string{"channel", "status"},
	)

	channelErrorTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "channel_error_total",
			Help: "Total number of downstream channel errors grouped by status code and error type.",
		},
		[]string{"channel", "channel_id", "model", "status_code", "error_type", "detail"},
	)

	channelErrorEventTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "channel_error_event_total",
			Help: "Individual downstream channel error events with occurrence timestamp.",
		},
		[]string{"channel", "channel_id", "model", "status_code", "error_type", "detail", "event_time", "event_id"},
	)

	// 渠道调用延迟直方图
	channelLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "channel_latency_seconds",
			Help:    "Latency distribution for downstream channel calls.",
			Buckets: prometheus.ExponentialBuckets(0.05, 2, 8),
		},
		[]string{"channel"},
	)

	// 渠道 Token 统计（Prompt/Completion/Total）
	channelTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "channel_tokens_total",
			Help: "Total number of tokens consumed per channel grouped by token type.",
		},
		[]string{"channel", "token_type"},
	)

	// channel_rpm/channel_tpm 通过本地滑动窗口维护 1 分钟总量
	channelRPM = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "channel_rpm",
			Help: "Rolling one-minute requests per minute per channel.",
		},
		[]string{"channel"},
	)

	channelTPM = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "channel_tpm",
			Help: "Rolling one-minute tokens per minute per channel.",
		},
		[]string{"channel"},
	)
)

// Handler exposes the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// ObserveAPIRequest records API level metrics.
func ObserveAPIRequest(path, method string, status int, duration time.Duration) {
	statusCode := strconv.Itoa(status)
	apiRequestTotal.WithLabelValues(path, method, statusCode).Inc()
	apiRequestDuration.WithLabelValues(path, method).Observe(duration.Seconds())
}

// ObserveChannelSuccess records metrics for a successful downstream call.
func ObserveChannelSuccess(channel string, duration time.Duration) {
	label := normalizeChannelLabel(channel)
	channelRequestTotal.WithLabelValues(label, "success").Inc()
	channelLatency.WithLabelValues(label).Observe(duration.Seconds())
	rollingStoreInstance.add(label, 1, 0)
}

// ObserveChannelError 记录渠道请求失败的次数、耗时及错误详情
func ObserveChannelError(channel string, channelID int, model string, statusCode int, errType string, detail string, duration time.Duration) {
	label := normalizeChannelLabel(channel)
	channelRequestTotal.WithLabelValues(label, "error").Inc()
	channelLatency.WithLabelValues(label).Observe(duration.Seconds())
	channelErrorTotal.WithLabelValues(
		label,
		strconv.Itoa(channelID),
		model,
		strconv.Itoa(statusCode),
		errType,
		sanitizeErrorDetail(detail),
	).Inc()
	rollingStoreInstance.add(label, 1, 0)
}

// RecordChannelErrorEvent 单独记录一次错误事件的发生时间
func RecordChannelErrorEvent(channel string, channelID int, model string, statusCode int, errType string, detail string, eventID string, eventTime time.Time) {
	channelErrorEventTotal.WithLabelValues(
		normalizeChannelLabel(channel),
		strconv.Itoa(channelID),
		model,
		strconv.Itoa(statusCode),
		errType,
		sanitizeErrorDetail(detail),
		eventTime.UTC().Format(time.RFC3339Nano),
		eventID,
	).Inc()
}

// ObserveChannelTokens records token consumption for a channel.
func ObserveChannelTokens(channel string, promptTokens, completionTokens, totalTokens int) {
	label := normalizeChannelLabel(channel)
	if promptTokens > 0 {
		channelTokensTotal.WithLabelValues(label, "prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		channelTokensTotal.WithLabelValues(label, "completion").Add(float64(completionTokens))
	}
	if totalTokens > 0 {
		channelTokensTotal.WithLabelValues(label, "total").Add(float64(totalTokens))
		rollingStoreInstance.add(label, 0, totalTokens)
	}
}

func normalizeChannelLabel(channel string) string {
	if channel == "" {
		return "unknown"
	}
	return channel
}

type rollingBucketEntry struct {
	start    time.Time
	requests float64
	tokens   float64
}

type rollingSeries struct {
	buckets [rollingBucketNum]rollingBucketEntry
}

func newRollingSeries() *rollingSeries {
	return &rollingSeries{}
}

func (s *rollingSeries) add(now time.Time, requests, tokens float64) {
	// 保持 (channel, bucketStart) 的最新桶，并复用旧桶
	bucketStart := now.Truncate(rollingBucket)
	targetIdx := -1
	oldestIdx := -1
	for i := range s.buckets {
		bucket := &s.buckets[i]
		if !bucket.start.IsZero() && bucket.start.Equal(bucketStart) {
			targetIdx = i
			break
		}
		if oldestIdx == -1 {
			oldestIdx = i
			continue
		}
		if s.buckets[oldestIdx].start.IsZero() || bucket.start.IsZero() || bucket.start.Before(s.buckets[oldestIdx].start) {
			oldestIdx = i
		}
	}

	if targetIdx == -1 {
		if oldestIdx == -1 {
			oldestIdx = 0
		}
		targetIdx = oldestIdx
		s.buckets[targetIdx] = rollingBucketEntry{
			start:    bucketStart,
			requests: 0,
			tokens:   0,
		}
	}

	if requests != 0 {
		s.buckets[targetIdx].requests += requests
	}
	if tokens != 0 {
		s.buckets[targetIdx].tokens += tokens
	}
}

func (s *rollingSeries) totals(now time.Time) (requests, tokens float64, lastActive time.Time) {
	cutoff := now.Add(-rollingWindow)
	for i := range s.buckets {
		bucket := s.buckets[i]
		if bucket.start.After(cutoff) {
			requests += bucket.requests
			tokens += bucket.tokens
			if bucket.start.After(lastActive) {
				lastActive = bucket.start
			}
		}
	}
	return
}

type rollingStore struct {
	mu    sync.Mutex
	store map[string]*rollingSeries
}

var rollingStoreInstance = newRollingStore()

func newRollingStore() *rollingStore {
	rs := &rollingStore{
		store: make(map[string]*rollingSeries),
	}
	// 后台定时刷新 Gauge，及时清理失效渠道
	go rs.loop()
	return rs
}

func (rs *rollingStore) add(channel string, requests, tokens int) {
	if channel == "" {
		channel = "unknown"
	}
	now := time.Now()

	rs.mu.Lock()
	series, ok := rs.store[channel]
	if !ok {
		series = newRollingSeries()
		rs.store[channel] = series
	}
	series.add(now, float64(requests), float64(tokens))
	reqSum, tokenSum, lastActive := series.totals(now)
	rs.mu.Unlock()

	if now.Sub(lastActive) > rollingWindow {
		channelRPM.DeleteLabelValues(channel)
		channelTPM.DeleteLabelValues(channel)
		return
	}

	channelRPM.WithLabelValues(channel).Set(reqSum)
	channelTPM.WithLabelValues(channel).Set(tokenSum)
}

func (rs *rollingStore) loop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		rs.mu.Lock()
		for channel, series := range rs.store {
			reqSum, tokenSum, lastActive := series.totals(now)
			if lastActive.IsZero() || now.Sub(lastActive) > rollingWindow {
				delete(rs.store, channel)
				channelRPM.DeleteLabelValues(channel)
				channelTPM.DeleteLabelValues(channel)
				continue
			}
			channelRPM.WithLabelValues(channel).Set(reqSum)
			channelTPM.WithLabelValues(channel).Set(tokenSum)
		}
		rs.mu.Unlock()
	}
}

var requestIDPattern = regexp.MustCompile(`\(request id:[^)]+\)`)

func sanitizeErrorDetail(detail string) string {
	detail = requestIDPattern.ReplaceAllString(detail, "")
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return "unknown"
	}
	if utf8.RuneCountInString(detail) > 80 {
		runes := []rune(detail)
		detail = string(runes[:80]) + "…"
	}
	return detail
}
