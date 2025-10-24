package router

import (
	"github.com/QuantumNous/new-api/metrics"

	"github.com/gin-gonic/gin"
)

func SetMetricsRouter(router *gin.Engine) {
	// 暴露 Prometheus 指标，供外部服务抓取。
	router.GET("/metrics", gin.WrapH(metrics.Handler()))
}
