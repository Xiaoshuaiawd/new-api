package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
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

func buildGlobalStickyContextForTest(requestedGroup, userGroup string) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	if userGroup != "" {
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, userGroup)
	}
	SetGlobalStickyScopeContext(ctx, requestedGroup)
	return ctx
}

func buildGlobalStickyMultiKeyContextForTest(requestedGroup, userGroup string, keyIndex int) *gin.Context {
	ctx := buildGlobalStickyContextForTest(requestedGroup, userGroup)
	common.SetContextKey(ctx, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(ctx, constant.ContextKeyChannelMultiKeyIndex, keyIndex)
	return ctx
}

func TestRefreshGlobalStickyChannelIDDoesNotOverwriteDifferentChannel(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"

		entry101 := SetGlobalStickyChannelID(scope, modelName, 101)
		assert.Equal(t, 101, GetGlobalStickyChannelID(scope, modelName))

		// Matching refresh keeps the sticky alive.
		assert.True(t, refreshGlobalStickyChannelEntry(scope, modelName, entry101))
		assert.Equal(t, 101, GetGlobalStickyChannelID(scope, modelName))

		// A stale successful request must not overwrite a newer sticky channel.
		SetGlobalStickyChannelID(scope, modelName, 202)
		assert.False(t, refreshGlobalStickyChannelEntry(scope, modelName, entry101))
		assert.Equal(t, 202, GetGlobalStickyChannelID(scope, modelName))
	})
}

func TestInvalidateGlobalStickyChannelIfMatchOnlyDeletesMatchingChannel(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"

		entry515 := SetGlobalStickyChannelID(scope, modelName, 515)
		assert.Equal(t, 515, GetGlobalStickyChannelID(scope, modelName))

		// A stale failing request must not clear a newer sticky owner.
		assert.False(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, globalChannelStickyEntry{ChannelID: 404, Raw: "404"}))
		assert.Equal(t, 515, GetGlobalStickyChannelID(scope, modelName))

		assert.True(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, entry515))
		assert.Equal(t, 0, GetGlobalStickyChannelID(scope, modelName))
	})
}

func TestGlobalStickyEntryVersionPreventsSameChannelStaleOperations(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"

		oldEntry := SetGlobalStickyChannelID(scope, modelName, 707)
		newEntry := SetGlobalStickyChannelID(scope, modelName, 707)
		require.NotEqual(t, oldEntry.Raw, newEntry.Raw)
		assert.Equal(t, 707, GetGlobalStickyChannelID(scope, modelName))

		// Old in-flight success must not refresh the new generation even when channel id is unchanged.
		assert.False(t, refreshGlobalStickyChannelEntry(scope, modelName, oldEntry))
		assert.Equal(t, 707, GetGlobalStickyChannelID(scope, modelName))

		// Old in-flight failure must not invalidate the new generation either.
		assert.False(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, oldEntry))
		assert.Equal(t, 707, GetGlobalStickyChannelID(scope, modelName))

		// Only the latest generation can mutate the sticky key.
		assert.True(t, refreshGlobalStickyChannelEntry(scope, modelName, newEntry))
		assert.True(t, invalidateGlobalStickyChannelIfMatch(scope, modelName, newEntry))
		assert.Equal(t, 0, GetGlobalStickyChannelID(scope, modelName))
	})
}

func TestRefreshGlobalStickyChannelFromContextSeedsStickyWhenAbsent(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"
		ctx := buildGlobalStickyContextForTest("auto", "vip")

		assert.True(t, RefreshGlobalStickyChannelFromContext(ctx, modelName, 818))
		assert.Equal(t, 818, GetGlobalStickyChannelID(scope, modelName))
	})
}

func TestRefreshGlobalStickyChannelFromContextUpgradesToMultiKeySelection(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"
		SetGlobalStickyChannelID(scope, modelName, 818)
		ctx := buildGlobalStickyMultiKeyContextForTest("auto", "vip", 2)

		assert.True(t, RefreshGlobalStickyChannelFromContext(ctx, modelName, 818))

		entry, ok := getGlobalStickyEntry(scope, modelName)
		require.True(t, ok)
		require.NotNil(t, entry.MultiKeyIndex)
		assert.Equal(t, 2, *entry.MultiKeyIndex)
	})
}

func TestInvalidateGlobalStickyChannelFromContextLoadsEntryWhenNotReadEarlier(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"
		SetGlobalStickyChannelID(scope, modelName, 919)
		ctx := buildGlobalStickyContextForTest("auto", "vip")

		assert.True(t, InvalidateGlobalStickyChannelFromContext(ctx, modelName, 919))
		assert.Equal(t, 0, GetGlobalStickyChannelID(scope, modelName))
	})
}

func TestGlobalChannelActivePoolHonorsMaxActiveChannels(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"
		setting := operation_setting.GetGlobalChannelStickySetting()
		setting.MaxActiveChannels = 2

		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 101))
		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 202))
		assert.False(t, touchGlobalChannelActivePool(scope, modelName, 303))
		assert.ElementsMatch(t, []int{101, 202}, getGlobalChannelActivePoolChannelIDs(scope, modelName))
	})
}

func TestInvalidateGlobalStickyChannelFromContextRemovesActivePoolMember(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"
		setting := operation_setting.GetGlobalChannelStickySetting()
		setting.MaxActiveChannels = 2

		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 919))
		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 1024))
		SetGlobalStickyChannelID(scope, modelName, 919)

		ctx := buildGlobalStickyContextForTest("auto", "vip")
		assert.True(t, InvalidateGlobalStickyChannelFromContext(ctx, modelName, 919))
		assert.ElementsMatch(t, []int{1024}, getGlobalChannelActivePoolChannelIDs(scope, modelName))
		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 2048))
		assert.ElementsMatch(t, []int{1024, 2048}, getGlobalChannelActivePoolChannelIDs(scope, modelName))
	})
}

func TestReplaceOldestGlobalChannelActivePoolMemberKeepsPoolBounded(t *testing.T) {
	withGlobalStickyTestRedis(t, func() {
		scope := ResolveGlobalStickyScope("auto", "vip")
		modelName := "gpt-5"
		setting := operation_setting.GetGlobalChannelStickySetting()
		setting.MaxActiveChannels = 2

		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 101))
		assert.True(t, touchGlobalChannelActivePool(scope, modelName, 202))
		assert.True(t, replaceOldestGlobalChannelActivePoolMember(scope, modelName, 303))
		assert.ElementsMatch(t, []int{202, 303}, getGlobalChannelActivePoolChannelIDs(scope, modelName))
	})
}
