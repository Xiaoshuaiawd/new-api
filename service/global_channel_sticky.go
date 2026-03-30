package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	globalChannelStickyKeyPrefix = "new-api:global_channel_sticky:v1"
	// ginKeyGlobalChannelStickyUsed 标记本次请求是否走了全局渠道粘性
	ginKeyGlobalChannelStickyUsed = "global_channel_sticky_used"
)

// globalChannelStickyRedisKey 生成 Redis key
// key 格式: new-api:global_channel_sticky:v1:{group}:{model}
func globalChannelStickyRedisKey(group, model string) string {
	return fmt.Sprintf("%s:%s:%s", globalChannelStickyKeyPrefix, group, model)
}

// GetGlobalStickyChannelID 从 Redis 中获取当前活跃渠道 ID。
// 返回 0 表示尚无活跃渠道（需要正常选渠道流程）。
func GetGlobalStickyChannelID(group, model string) int {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return 0
	}
	if !common.RedisEnabled || common.RDB == nil {
		return 0
	}
	key := globalChannelStickyRedisKey(group, model)
	val, err := common.RDB.Get(context.Background(), key).Result()
	if err != nil {
		// key 不存在或 Redis 错误均视为无活跃渠道
		return 0
	}
	id, err := strconv.Atoi(val)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// SetGlobalStickyChannelID 将指定渠道设为全局活跃渠道，写入 Redis。
func SetGlobalStickyChannelID(group, model string, channelID int) {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return
	}
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	if channelID <= 0 {
		return
	}
	key := globalChannelStickyRedisKey(group, model)
	ttl := time.Duration(setting.TTLSeconds) * time.Second
	if setting.TTLSeconds <= 0 {
		ttl = 0 // 永不过期
	}
	var err error
	if ttl > 0 {
		err = common.RDB.Set(context.Background(), key, strconv.Itoa(channelID), ttl).Err()
	} else {
		err = common.RDB.Set(context.Background(), key, strconv.Itoa(channelID), 0).Err()
	}
	if err != nil {
		common.SysError(fmt.Sprintf("global channel sticky set failed: key=%s, err=%v", key, err))
	}
}

// InvalidateGlobalStickyChannel 将当前活跃渠道从 Redis 中删除（触发条件：429 或 401）。
// 删除后下一次请求会触发重新选渠道，并将新渠道写入 Redis。
func InvalidateGlobalStickyChannel(group, model string) {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return
	}
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	key := globalChannelStickyRedisKey(group, model)
	if err := common.RDB.Del(context.Background(), key).Err(); err != nil {
		common.SysError(fmt.Sprintf("global channel sticky invalidate failed: key=%s, err=%v", key, err))
	}
}

// IsGlobalChannelStickyTriggerCode 判断状态码是否应触发渠道切换
// 当前仅 429 和 401 触发
func IsGlobalChannelStickyTriggerCode(statusCode int) bool {
	return statusCode == 429 || statusCode == 401
}
