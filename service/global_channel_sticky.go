package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

// SetGlobalStickyChannelIDWithLock 使用 SETNX 原子写入活跃渠道，避免并发竞态。
// 返回 true 表示写入成功，false 表示已有其他请求写入。
func SetGlobalStickyChannelIDWithLock(group, model string, channelID int) bool {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return false
	}
	if !common.RedisEnabled || common.RDB == nil {
		return false
	}
	if channelID <= 0 {
		return false
	}
	key := globalChannelStickyRedisKey(group, model)
	ttl := time.Duration(setting.TTLSeconds) * time.Second
	if setting.TTLSeconds <= 0 {
		ttl = 0
	}

	// 使用 SetNX 原子操作：只有 key 不存在时才写入
	var success bool
	var err error
	if ttl > 0 {
		success, err = common.RDB.SetNX(context.Background(), key, strconv.Itoa(channelID), ttl).Result()
	} else {
		success, err = common.RDB.SetNX(context.Background(), key, strconv.Itoa(channelID), 0).Result()
	}
	if err != nil {
		common.SysError(fmt.Sprintf("global channel sticky setnx failed: key=%s, err=%v", key, err))
		return false
	}
	return success
}

// SetGlobalStickyChannelID 将指定渠道设为全局活跃渠道，写入 Redis（覆盖模式，用于刷新 TTL）。
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
		ttl = 0
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

// GetOrSelectGlobalStickyChannel 原子获取或选择全局粘性渠道（带重试等待）。
// 用于并发场景：如果 Redis 为空，尝试选新渠道并用 SETNX 写入；
// 如果写入失败（其他请求已写入），则等待并重新读取。
func GetOrSelectGlobalStickyChannel(group, modelName string, selectFunc func() (*model.Channel, string, error)) (*model.Channel, string, error) {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		// 功能未启用，直接调用选择函数
		return selectFunc()
	}
	if !common.RedisEnabled || common.RDB == nil {
		return selectFunc()
	}

	// 先尝试读取现有渠道
	if channelID := GetGlobalStickyChannelID(group, modelName); channelID > 0 {
		channel, err := model.CacheGetChannel(channelID)
		if err == nil && channel != nil && channel.Status == common.ChannelStatusEnabled {
			if group == "auto" {
				// auto group 需要验证渠道是否在可用分组中
				// 这里简化处理，直接返回，由调用方验证
				return channel, group, nil
			}
			if model.IsChannelEnabledForGroupModel(group, modelName, channel.Id) {
				return channel, group, nil
			}
		}
	}

	// Redis 为空或渠道失效，选新渠道
	channel, selectGroup, err := selectFunc()
	if err != nil || channel == nil {
		return channel, selectGroup, err
	}

	// 尝试用 SETNX 写入（原子操作）
	if SetGlobalStickyChannelIDWithLock(selectGroup, modelName, channel.Id) {
		// 写入成功，我是第一个
		return channel, selectGroup, nil
	}

	// 写入失败，说明其他请求已写入，等待 50ms 后重新读取
	time.Sleep(50 * time.Millisecond)
	if channelID := GetGlobalStickyChannelID(group, modelName); channelID > 0 {
		newChannel, err := model.CacheGetChannel(channelID)
		if err == nil && newChannel != nil && newChannel.Status == common.ChannelStatusEnabled {
			// 使用其他请求选出的渠道
			return newChannel, selectGroup, nil
		}
	}

	// 兜底：仍然使用自己选出的渠道
	return channel, selectGroup, nil
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
