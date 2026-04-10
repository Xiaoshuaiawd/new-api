package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForSelectedChannelUsesPreferredMultiKeyIndex(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	common.SetContextKey(ctx, constant.ContextKeyChannelPreferredMultiKeyIndex, 2)

	channel := &model.Channel{
		Id:   1001,
		Name: "multi-key-test",
		Key:  "key-a\nkey-b\nkey-c",
		ChannelInfo: model.ChannelInfo{
			IsMultiKey:   true,
			MultiKeySize: 3,
			MultiKeyMode: constant.MultiKeyModePolling,
		},
	}

	err := SetupContextForSelectedChannel(ctx, channel, "gpt-5")
	require.Nil(t, err)
	require.True(t, common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey))
	require.Equal(t, 2, common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	require.Equal(t, "key-c", common.GetContextKeyString(ctx, constant.ContextKeyChannelKey))
}
