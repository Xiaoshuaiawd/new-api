package middleware

// UpdateChannelStatusMetrics 更新渠道状态的 Prometheus metrics
// 这个函数被 model 包调用，避免循环依赖
func UpdateChannelStatusMetrics(channelID int, channelName string, channelType int, status int) {
	UpdateChannelStatus(channelID, channelName, channelType, status)
}
