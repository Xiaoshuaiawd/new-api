package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withGlobalStickyTestRedis(t *testing.T, fn func()) {
	t.Helper()

	mr, err := miniredis.Run()
	require.NoError(t, err)

	oldRDB := common.RDB
	oldRedisEnabled := common.RedisEnabled
	oldSetting := *operation_setting.GetGlobalChannelStickySetting()

	common.RDB = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	common.RedisEnabled = true
	*operation_setting.GetGlobalChannelStickySetting() = operation_setting.GlobalChannelStickySetting{
		Enabled:    true,
		TTLSeconds: 3600,
	}

	t.Cleanup(func() {
		if common.RDB != nil {
			_ = common.RDB.Close()
		}
		common.RDB = oldRDB
		common.RedisEnabled = oldRedisEnabled
		*operation_setting.GetGlobalChannelStickySetting() = oldSetting
		mr.Close()
	})

	fn()
}

func TestResolveGlobalStickyScope(t *testing.T) {
	assert.Equal(t, "default", ResolveGlobalStickyScope("default", "vip"))
	assert.Equal(t, "auto@vip", ResolveGlobalStickyScope("auto", "vip"))
	assert.Equal(t, "auto", ResolveGlobalStickyScope("auto", ""))
	assert.Equal(t, "", ResolveGlobalStickyScope("", "vip"))
}

func TestRefreshGlobalStickyChannelIDDoesNotOverwriteDifferentChannel(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"

		SetGlobalStickyChannelID(scope, modelName, 101)
		assert.Equal(t, 101, GetGlobalStickyChannelID(scope, modelName))

		// Matching refresh keeps the sticky alive.
		assert.True(t, refreshGlobalStickyChannelID(scope, modelName, 101))
		assert.Equal(t, 101, GetGlobalStickyChannelID(scope, modelName))

		// A stale successful request must not overwrite a newer sticky channel.
		SetGlobalStickyChannelID(scope, modelName, 202)
		assert.False(t, refreshGlobalStickyChannelID(scope, modelName, 101))
		assert.Equal(t, 202, GetGlobalStickyChannelID(scope, modelName))

		// If the key is absent, the first successful request may adopt it again.
		assert.True(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, 202))
		assert.Equal(t, 0, GetGlobalStickyChannelID(scope, modelName))
		assert.True(t, refreshGlobalStickyChannelID(scope, modelName, 303))
		assert.Equal(t, 303, GetGlobalStickyChannelID(scope, modelName))
	})
}

func TestInvalidateGlobalStickyChannelIfMatchOnlyDeletesMatchingChannel(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"

		SetGlobalStickyChannelID(scope, modelName, 515)
		assert.Equal(t, 515, GetGlobalStickyChannelID(scope, modelName))

		// A stale failing request must not clear a newer sticky owner.
		assert.False(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, 404))
		assert.Equal(t, 515, GetGlobalStickyChannelID(scope, modelName))

		assert.True(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, 515))
		assert.Equal(t, 0, GetGlobalStickyChannelID(scope, modelName))
	})
}
