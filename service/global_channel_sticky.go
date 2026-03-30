package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	globalChannelStickyKeyPrefix = "new-api:global_channel_sticky:v1"
	// ginKeyGlobalChannelStickyUsed 标记本次请求是否走了全局渠道粘性
	ginKeyGlobalChannelStickyUsed  = "global_channel_sticky_used"
	ginKeyGlobalChannelStickyScope = "global_channel_sticky_scope"
	ginKeyGlobalChannelStickyEntry = "global_channel_sticky_entry"
)

const (
	globalChannelStickyRefreshLua = `
local current = redis.call('GET', KEYS[1])
if current ~= ARGV[1] then
	return 0
end
local ttl = tonumber(ARGV[2])
if ttl and ttl > 0 then
	redis.call('SET', KEYS[1], ARGV[1], 'EX', ttl)
else
	redis.call('SET', KEYS[1], ARGV[1])
end
return 1
`
	globalChannelStickyDeleteIfMatchLua = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
	return redis.call('DEL', KEYS[1])
end
return 0
	`
)

type globalChannelStickyEntry struct {
	ChannelID int
	Version   string
	Raw       string
}

func newGlobalChannelStickyEntry(channelID int) globalChannelStickyEntry {
	entry := globalChannelStickyEntry{
		ChannelID: channelID,
		Version:   uuid.NewString(),
	}
	entry.Raw = encodeGlobalChannelStickyEntry(entry)
	return entry
}

func encodeGlobalChannelStickyEntry(entry globalChannelStickyEntry) string {
	if entry.ChannelID <= 0 {
		return ""
	}
	raw := strconv.Itoa(entry.ChannelID)
	if strings.TrimSpace(entry.Version) == "" {
		return raw
	}
	return raw + "|" + strings.TrimSpace(entry.Version)
}

func decodeGlobalChannelStickyEntry(raw string) (globalChannelStickyEntry, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return globalChannelStickyEntry{}, false
	}
	parts := strings.SplitN(raw, "|", 2)
	channelID, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || channelID <= 0 {
		return globalChannelStickyEntry{}, false
	}
	entry := globalChannelStickyEntry{
		ChannelID: channelID,
		Raw:       raw,
	}
	if len(parts) == 2 {
		entry.Version = strings.TrimSpace(parts[1])
	}
	return entry, true
}

// ResolveGlobalStickyScope returns the sticky scope used as the Redis key dimension.
// For auto group we include userGroup so different user-group auto pools do not fight each other.
func ResolveGlobalStickyScope(requestedGroup, userGroup string) string {
	requestedGroup = strings.TrimSpace(requestedGroup)
	userGroup = strings.TrimSpace(userGroup)
	if requestedGroup == "" {
		return ""
	}
	if requestedGroup != "auto" {
		return requestedGroup
	}
	if userGroup == "" {
		return requestedGroup
	}
	return requestedGroup + "@" + userGroup
}

func SetGlobalStickyScopeContext(c *gin.Context, requestedGroup string) string {
	if c == nil {
		return ""
	}
	scope := ResolveGlobalStickyScope(requestedGroup, common.GetContextKeyString(c, constant.ContextKeyUserGroup))
	if scope != "" {
		c.Set(ginKeyGlobalChannelStickyScope, scope)
	}
	return scope
}

func getGlobalStickyScopeContext(c *gin.Context) string {
	if c == nil {
		return ""
	}
	anyScope, ok := c.Get(ginKeyGlobalChannelStickyScope)
	if !ok {
		return ""
	}
	scope, _ := anyScope.(string)
	return strings.TrimSpace(scope)
}

func getOrInitGlobalStickyScope(c *gin.Context, requestedGroup string) string {
	if scope := getGlobalStickyScopeContext(c); scope != "" {
		return scope
	}
	return SetGlobalStickyScopeContext(c, requestedGroup)
}

func setGlobalStickyEntryContext(c *gin.Context, entry globalChannelStickyEntry) {
	if c == nil {
		return
	}
	raw := strings.TrimSpace(entry.Raw)
	if raw == "" {
		raw = encodeGlobalChannelStickyEntry(entry)
	}
	if raw == "" {
		return
	}
	c.Set(ginKeyGlobalChannelStickyEntry, raw)
}

func getGlobalStickyEntryContext(c *gin.Context) (globalChannelStickyEntry, bool) {
	if c == nil {
		return globalChannelStickyEntry{}, false
	}
	anyEntry, ok := c.Get(ginKeyGlobalChannelStickyEntry)
	if !ok {
		return globalChannelStickyEntry{}, false
	}
	raw, _ := anyEntry.(string)
	return decodeGlobalChannelStickyEntry(raw)
}

// globalChannelStickyRedisKey 生成 Redis key
// key 格式: new-api:global_channel_sticky:v1:{scope}:{model}
func globalChannelStickyRedisKey(scope, model string) string {
	return fmt.Sprintf("%s:%s:%s", globalChannelStickyKeyPrefix, scope, model)
}

func globalChannelStickyTTLSeconds() int {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil {
		return 0
	}
	return setting.TTLSeconds
}

func applyGlobalStickySelectedGroup(c *gin.Context, requestedGroup, selectedGroup string) {
	if c == nil {
		return
	}
	if strings.TrimSpace(requestedGroup) != "auto" {
		return
	}
	selectedGroup = strings.TrimSpace(selectedGroup)
	if selectedGroup == "" {
		return
	}
	common.SetContextKey(c, constant.ContextKeyAutoGroup, selectedGroup)
}

func resolveGlobalStickySelectedChannel(requestedGroup, userGroup, modelName string, channelID int) (*model.Channel, string, bool) {
	if channelID <= 0 {
		return nil, "", false
	}
	channel, err := model.CacheGetChannel(channelID)
	if err != nil || channel == nil || channel.Status != common.ChannelStatusEnabled {
		return nil, "", false
	}

	requestedGroup = strings.TrimSpace(requestedGroup)
	if requestedGroup == "auto" {
		autoGroups := GetUserAutoGroup(userGroup)
		for _, group := range autoGroups {
			if model.IsChannelEnabledForGroupModel(group, modelName, channel.Id) {
				return channel, group, true
			}
		}
		return nil, "", false
	}
	if model.IsChannelEnabledForGroupModel(requestedGroup, modelName, channel.Id) {
		return channel, requestedGroup, true
	}
	return nil, "", false
}

// GetGlobalStickyChannelID 从 Redis 中获取当前活跃渠道 ID。
// 返回 0 表示尚无活跃渠道（需要正常选渠道流程）。
func getGlobalStickyEntry(scope, model string) (globalChannelStickyEntry, bool) {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return globalChannelStickyEntry{}, false
	}
	if !common.RedisEnabled || common.RDB == nil {
		return globalChannelStickyEntry{}, false
	}
	key := globalChannelStickyRedisKey(scope, model)
	val, err := common.RDB.Get(context.Background(), key).Result()
	if err != nil {
		// key 不存在或 Redis 错误均视为无活跃渠道
		return globalChannelStickyEntry{}, false
	}
	return decodeGlobalChannelStickyEntry(val)
}

// GetGlobalStickyChannelID 从 Redis 中获取当前活跃渠道 ID。
// 返回 0 表示尚无活跃渠道（需要正常选渠道流程）。
func GetGlobalStickyChannelID(scope, model string) int {
	entry, ok := getGlobalStickyEntry(scope, model)
	if !ok {
		return 0
	}
	return entry.ChannelID
}

func TryGetGlobalStickyChannel(c *gin.Context, requestedGroup, modelName string) (*model.Channel, string, bool) {
	scope := getOrInitGlobalStickyScope(c, requestedGroup)
	if scope == "" {
		return nil, "", false
	}
	entry, ok := getGlobalStickyEntry(scope, modelName)
	if !ok {
		return nil, "", false
	}
	channel, selectedGroup, ok := resolveGlobalStickySelectedChannel(
		requestedGroup,
		common.GetContextKeyString(c, constant.ContextKeyUserGroup),
		modelName,
		entry.ChannelID,
	)
	if !ok {
		return nil, "", false
	}
	applyGlobalStickySelectedGroup(c, requestedGroup, selectedGroup)
	setGlobalStickyEntryContext(c, entry)
	c.Set(ginKeyGlobalChannelStickyUsed, true)
	return channel, selectedGroup, true
}

// SetGlobalStickyChannelIDWithLock 使用 SETNX 原子写入活跃渠道，避免并发竞态。
// 返回 true 表示写入成功，false 表示已有其他请求写入。
func SetGlobalStickyChannelIDWithLock(scope, model string, channelID int) (globalChannelStickyEntry, bool) {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return globalChannelStickyEntry{}, false
	}
	if !common.RedisEnabled || common.RDB == nil {
		return globalChannelStickyEntry{}, false
	}
	if channelID <= 0 {
		return globalChannelStickyEntry{}, false
	}
	entry := newGlobalChannelStickyEntry(channelID)
	key := globalChannelStickyRedisKey(scope, model)
	ttl := time.Duration(setting.TTLSeconds) * time.Second
	if setting.TTLSeconds <= 0 {
		ttl = 0
	}

	// 使用 SetNX 原子操作：只有 key 不存在时才写入
	var success bool
	var err error
	if ttl > 0 {
		success, err = common.RDB.SetNX(context.Background(), key, entry.Raw, ttl).Result()
	} else {
		success, err = common.RDB.SetNX(context.Background(), key, entry.Raw, 0).Result()
	}
	if err != nil {
		common.SysError(fmt.Sprintf("global channel sticky setnx failed: key=%s, err=%v", key, err))
		return globalChannelStickyEntry{}, false
	}
	return entry, success
}

// SetGlobalStickyChannelID 将指定渠道设为全局活跃渠道，写入 Redis（覆盖模式）。
func SetGlobalStickyChannelID(scope, model string, channelID int) globalChannelStickyEntry {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return globalChannelStickyEntry{}
	}
	if !common.RedisEnabled || common.RDB == nil {
		return globalChannelStickyEntry{}
	}
	if channelID <= 0 {
		return globalChannelStickyEntry{}
	}
	entry := newGlobalChannelStickyEntry(channelID)
	key := globalChannelStickyRedisKey(scope, model)
	ttl := time.Duration(setting.TTLSeconds) * time.Second
	if setting.TTLSeconds <= 0 {
		ttl = 0
	}
	var err error
	if ttl > 0 {
		err = common.RDB.Set(context.Background(), key, entry.Raw, ttl).Err()
	} else {
		err = common.RDB.Set(context.Background(), key, entry.Raw, 0).Err()
	}
	if err != nil {
		common.SysError(fmt.Sprintf("global channel sticky set failed: key=%s, err=%v", key, err))
	}
	return entry
}

// RefreshGlobalStickyChannelFromContext refreshes the sticky TTL if the current sticky value is unchanged.
// When the key has already been switched to a different channel, this call becomes a no-op so old in-flight
// requests do not clobber a newer sticky decision.
func RefreshGlobalStickyChannelFromContext(c *gin.Context, model string, channelID int) bool {
	scope := getGlobalStickyScopeContext(c)
	entry, ok := getGlobalStickyEntryContext(c)
	if !ok {
		return false
	}
	if channelID > 0 && entry.ChannelID != channelID {
		return false
	}
	return refreshGlobalStickyChannelEntry(scope, model, entry)
}

func refreshGlobalStickyChannelEntry(scope, model string, entry globalChannelStickyEntry) bool {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return false
	}
	if !common.RedisEnabled || common.RDB == nil {
		return false
	}
	scope = strings.TrimSpace(scope)
	raw := strings.TrimSpace(entry.Raw)
	if raw == "" {
		raw = encodeGlobalChannelStickyEntry(entry)
	}
	if scope == "" || strings.TrimSpace(model) == "" || raw == "" {
		return false
	}

	key := globalChannelStickyRedisKey(scope, model)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := common.RDB.Eval(
		ctx,
		globalChannelStickyRefreshLua,
		[]string{key},
		raw,
		strconv.Itoa(globalChannelStickyTTLSeconds()),
	).Int()
	if err != nil {
		common.SysError(fmt.Sprintf("global channel sticky refresh failed: key=%s, err=%v", key, err))
		return false
	}
	return res == 1
}

// GetOrSelectGlobalStickyChannel 原子获取或选择全局粘性渠道（带重试等待）。
// 用于并发场景：如果 Redis 为空，尝试选新渠道并用 SETNX 写入；
// 如果写入失败（其他请求已写入），则等待并重新读取。
func GetOrSelectGlobalStickyChannel(c *gin.Context, requestedGroup, modelName string, selectFunc func() (*model.Channel, string, error)) (*model.Channel, string, error) {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		// 功能未启用，直接调用选择函数
		channel, selectGroup, err := selectFunc()
		if err == nil && channel != nil {
			applyGlobalStickySelectedGroup(c, requestedGroup, selectGroup)
		}
		return channel, selectGroup, err
	}
	if !common.RedisEnabled || common.RDB == nil {
		channel, selectGroup, err := selectFunc()
		if err == nil && channel != nil {
			applyGlobalStickySelectedGroup(c, requestedGroup, selectGroup)
		}
		return channel, selectGroup, err
	}
	scope := getOrInitGlobalStickyScope(c, requestedGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)

	// 先尝试读取现有渠道
	if channelID := GetGlobalStickyChannelID(scope, modelName); channelID > 0 {
		channel, selectGroup, ok := resolveGlobalStickySelectedChannel(requestedGroup, userGroup, modelName, channelID)
		if ok {
			applyGlobalStickySelectedGroup(c, requestedGroup, selectGroup)
			c.Set(ginKeyGlobalChannelStickyUsed, true)
			return channel, selectGroup, nil
		}
	}

	// Redis 为空或渠道失效，选新渠道
	channel, selectGroup, err := selectFunc()
	if err != nil || channel == nil {
		return channel, selectGroup, err
	}

	// 尝试用 SETNX 写入（原子操作）
	if entry, ok := SetGlobalStickyChannelIDWithLock(scope, modelName, channel.Id); ok {
		// 写入成功，我是第一个
		applyGlobalStickySelectedGroup(c, requestedGroup, selectGroup)
		setGlobalStickyEntryContext(c, entry)
		return channel, selectGroup, nil
	}

	// 写入失败，说明其他请求已写入，短暂等待后重新读取几次。
	for i := 0; i < 3; i++ {
		time.Sleep(time.Duration(i+1) * 20 * time.Millisecond)
		if entry, ok := getGlobalStickyEntry(scope, modelName); ok {
			newChannel, stickyGroup, ok := resolveGlobalStickySelectedChannel(requestedGroup, userGroup, modelName, entry.ChannelID)
			if ok {
				applyGlobalStickySelectedGroup(c, requestedGroup, stickyGroup)
				setGlobalStickyEntryContext(c, entry)
				c.Set(ginKeyGlobalChannelStickyUsed, true)
				return newChannel, stickyGroup, nil
			}
		}
	}

	// 兜底：仍然使用自己选出的渠道
	applyGlobalStickySelectedGroup(c, requestedGroup, selectGroup)
	return channel, selectGroup, nil
}

// InvalidateGlobalStickyChannelFromContext removes the sticky entry only when it still points
// to the failing channel. This avoids deleting a newer sticky channel chosen by another request.
func InvalidateGlobalStickyChannelFromContext(c *gin.Context, model string, channelID int) bool {
	scope := getGlobalStickyScopeContext(c)
	entry, ok := getGlobalStickyEntryContext(c)
	if !ok {
		return false
	}
	if channelID > 0 && entry.ChannelID != channelID {
		return false
	}
	return invalidateGlobalStickyChannelIfMatch(scope, model, entry)
}

func invalidateGlobalStickyChannelIfMatch(scope, model string, entry globalChannelStickyEntry) bool {
	setting := operation_setting.GetGlobalChannelStickySetting()
	if setting == nil || !setting.Enabled {
		return false
	}
	if !common.RedisEnabled || common.RDB == nil {
		return false
	}
	scope = strings.TrimSpace(scope)
	raw := strings.TrimSpace(entry.Raw)
	if raw == "" {
		raw = encodeGlobalChannelStickyEntry(entry)
	}
	if scope == "" || strings.TrimSpace(model) == "" || raw == "" {
		return false
	}

	key := globalChannelStickyRedisKey(scope, model)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := common.RDB.Eval(
		ctx,
		globalChannelStickyDeleteIfMatchLua,
		[]string{key},
		raw,
	).Int()
	if err != nil {
		common.SysError(fmt.Sprintf("global channel sticky invalidate failed: key=%s, err=%v", key, err))
		return false
	}
	return res > 0
}

// IsGlobalChannelStickyTriggerCode 判断状态码是否应触发渠道切换
// 当前仅 429 和 401 触发
func IsGlobalChannelStickyTriggerCode(statusCode int) bool {
	return statusCode == 429 || statusCode == 401
}
