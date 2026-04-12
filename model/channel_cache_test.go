package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRandomSatisfiedChannelWithAllowedIDsUsesAllowedSet(t *testing.T) {
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2Model2Channels := group2model2channels
	oldGroup2Model2PriorityBuckets := group2model2priorityBuckets
	oldChannelsIDM := channelsIDM
	oldChannel2GroupModels := channel2GroupModels
	oldFullChannelCache := fullChannelCache

	common.MemoryCacheEnabled = true
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		group2model2channels = oldGroup2Model2Channels
		group2model2priorityBuckets = oldGroup2Model2PriorityBuckets
		channelsIDM = oldChannelsIDM
		channel2GroupModels = oldChannel2GroupModels
		fullChannelCache = oldFullChannelCache
	})

	channel1 := &Channel{Id: 1, Status: common.ChannelStatusEnabled, Weight: common.GetPointer(uint(10))}
	channel2 := &Channel{Id: 2, Status: common.ChannelStatusEnabled, Weight: common.GetPointer(uint(20))}
	channel3 := &Channel{Id: 3, Status: common.ChannelStatusEnabled, Weight: common.GetPointer(uint(30))}

	group2model2channels = map[string]map[string][]int{
		"default": {
			"gpt-5": {1, 2, 3},
		},
	}
	group2model2priorityBuckets = map[string]map[string][]channelPriorityBucket{
		"default": {
			"gpt-5": {
				{
					Priority:   100,
					ChannelIDs: []int{1, 2, 3},
					SumWeight:  60,
				},
			},
		},
	}
	channelsIDM = map[int]*Channel{
		1: channel1,
		2: channel2,
		3: channel3,
	}
	channel2GroupModels = map[int][]groupModelKey{
		1: {{Group: "default", Model: "gpt-5"}},
		2: {{Group: "default", Model: "gpt-5"}},
		3: {{Group: "default", Model: "gpt-5"}},
	}
	fullChannelCache = map[int]*Channel{
		1: channel1,
		2: channel2,
		3: channel3,
	}

	for i := 0; i < 20; i++ {
		channel, err := GetRandomSatisfiedChannelWithAllowedIDs("default", "gpt-5", 0, []int{2, 3})
		require.NoError(t, err)
		require.NotNil(t, channel)
		assert.Contains(t, []int{2, 3}, channel.Id)
	}

	channel, err := GetRandomSatisfiedChannelWithAllowedIDs("default", "gpt-5", 0, []int{4})
	require.NoError(t, err)
	assert.Nil(t, channel)
}
