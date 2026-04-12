package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// GlobalChannelStickySetting 全局渠道粘性配置
// 启用后，同一 group+model 的所有请求会被路由到同一个渠道，
// 只有当该渠道返回 429 或 401 时才切换到下一个渠道。
type GlobalChannelStickySetting struct {
	// Enabled 是否启用全局渠道粘性
	Enabled bool `json:"enabled"`
	// TTLSeconds 活跃渠道在 Redis 中的 TTL（秒），0 表示不过期
	// 建议设置一个合理值（如 3600），避免渠道长期不更新
	TTLSeconds int `json:"ttl_seconds"`
	// MaxActiveChannels 最大同时活跃渠道数，0 表示不限制
	// 启用后随机选路只会在最多 N 个活跃渠道范围内扩散。
	MaxActiveChannels int `json:"max_active_channels"`
}

var globalChannelStickySetting = GlobalChannelStickySetting{
	Enabled:           false,
	TTLSeconds:        3600,
	MaxActiveChannels: 0,
}

func init() {
	config.GlobalConfig.Register("global_channel_sticky_setting", &globalChannelStickySetting)
}

func GetGlobalChannelStickySetting() *GlobalChannelStickySetting {
	return &globalChannelStickySetting
}

func IsGlobalChannelStickyEnabled() bool {
	return globalChannelStickySetting.Enabled
}
