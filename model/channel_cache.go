package model

import (
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

var group2model2channels map[string]map[string][]int // enabled channel
var group2model2priorityBuckets map[string]map[string][]channelPriorityBucket
var channelsIDM map[int]*Channel // all channels (slim, without heavy fields like key/setting/model_mapping)
var channelSyncLock sync.RWMutex

// reverse index: channelID -> list of (group, model) pairs for fast removal
type groupModelKey struct {
	Group string
	Model string
}

var channel2GroupModels map[int][]groupModelKey

// fullChannelCache: on-demand loaded full Channel objects (with key, setting, etc.)
var fullChannelCache map[int]*Channel
var fullChannelCacheLock sync.RWMutex

type channelPriorityBucket struct {
	Priority   int64
	ChannelIDs []int
	SumWeight  int
}

// channelCacheOmitFields are the heavy fields excluded from the slim cache loaded during sync.
// These fields are loaded on-demand via CacheGetFullChannel().
var channelCacheOmitFields = []string{
	"key", "model_mapping", "status_code_mapping",
	"setting", "param_override", "header_override",
	"remark", "other_info", "settings",
}

func InitChannelCache() {
	if !common.MemoryCacheEnabled {
		return
	}
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	// Only load fields needed for routing; heavy fields (key, setting, etc.) are loaded on-demand
	DB.Omit(channelCacheOmitFields...).Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}
	// Only fetch distinct group names instead of loading the entire abilities table
	var groupList []string
	DB.Model(&Ability{}).Where("enabled = ?", true).Distinct().Pluck(commonGroupCol, &groupList)
	newGroup2model2channels := make(map[string]map[string][]int)
	newGroup2model2priorityBuckets := make(map[string]map[string][]channelPriorityBucket)
	newChannel2GroupModels := make(map[int][]groupModelKey)
	for _, group := range groupList {
		newGroup2model2channels[group] = make(map[string][]int)
		newGroup2model2priorityBuckets[group] = make(map[string][]channelPriorityBucket)
	}
	for _, channel := range channels {
		if channel.Status != common.ChannelStatusEnabled {
			continue // skip disabled channels
		}
		groups := strings.Split(channel.Group, ",")
		for _, group := range groups {
			if _, ok := newGroup2model2channels[group]; !ok {
				newGroup2model2channels[group] = make(map[string][]int)
			}
			if _, ok := newGroup2model2priorityBuckets[group]; !ok {
				newGroup2model2priorityBuckets[group] = make(map[string][]channelPriorityBucket)
			}
			models := strings.Split(channel.Models, ",")
			for _, model := range models {
				if _, ok := newGroup2model2channels[group][model]; !ok {
					newGroup2model2channels[group][model] = make([]int, 0)
				}
				newGroup2model2channels[group][model] = append(newGroup2model2channels[group][model], channel.Id)
				// build reverse index
				newChannel2GroupModels[channel.Id] = append(newChannel2GroupModels[channel.Id], groupModelKey{Group: group, Model: model})
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return newChannelId2channel[channels[i]].GetPriority() > newChannelId2channel[channels[j]].GetPriority()
			})
			newGroup2model2channels[group][model] = channels

			// Build priority buckets once during sync, so request-time selection can avoid repeated
			// priority de-dup/sort scans on large channel sets.
			buckets := make([]channelPriorityBucket, 0)
			for _, channelID := range channels {
				channel := newChannelId2channel[channelID]
				priority := channel.GetPriority()
				weight := channel.GetWeight()
				if len(buckets) == 0 || buckets[len(buckets)-1].Priority != priority {
					buckets = append(buckets, channelPriorityBucket{
						Priority:   priority,
						ChannelIDs: []int{channelID},
						SumWeight:  weight,
					})
					continue
				}
				last := len(buckets) - 1
				buckets[last].ChannelIDs = append(buckets[last].ChannelIDs, channelID)
				buckets[last].SumWeight += weight
			}
			newGroup2model2priorityBuckets[group][model] = buckets
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	group2model2priorityBuckets = newGroup2model2priorityBuckets
	channel2GroupModels = newChannel2GroupModels
	for i, channel := range newChannelId2channel {
		if channel.ChannelInfo.IsMultiKey {
			// Key field is not loaded in slim cache, so we cannot call GetKeys() here.
			// Keys will be populated when full channel is loaded on-demand.
			if channel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
				if oldChannel, ok := channelsIDM[i]; ok {
					// 存在旧的渠道，如果是多key且轮询，保留轮询索引信息
					if oldChannel.ChannelInfo.IsMultiKey && oldChannel.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
						channel.ChannelInfo.MultiKeyPollingIndex = oldChannel.ChannelInfo.MultiKeyPollingIndex
					}
				}
			}
		}
	}
	channelsIDM = newChannelId2channel
	channelSyncLock.Unlock()

	// Clear the full channel cache so stale entries are evicted after re-sync
	fullChannelCacheLock.Lock()
	fullChannelCache = make(map[int]*Channel)
	fullChannelCacheLock.Unlock()

	common.SysLog("channels synced from database")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		common.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetRandomSatisfiedChannel(group string, model string, retry int) (*Channel, error) {
	// if memory cache is disabled, get channel directly from database
	if !common.MemoryCacheEnabled {
		return GetChannel(group, model, retry)
	}

	// Select channel ID from slim cache under read lock
	channelSyncLock.RLock()

	// First, try exact model buckets.
	priorityBuckets := group2model2priorityBuckets[group][model]

	// If no buckets found, try normalized model name.
	if len(priorityBuckets) == 0 {
		normalizedModel := ratio_setting.FormatMatchingModelName(model)
		priorityBuckets = group2model2priorityBuckets[group][normalizedModel]
	}

	if len(priorityBuckets) == 0 {
		channelSyncLock.RUnlock()
		return nil, nil
	}

	targetBucketIndex := retry
	if targetBucketIndex >= len(priorityBuckets) {
		targetBucketIndex = len(priorityBuckets) - 1
	}
	targetBucket := priorityBuckets[targetBucketIndex]
	if len(targetBucket.ChannelIDs) == 0 {
		channelSyncLock.RUnlock()
		return nil, errors.New("channel bucket is empty")
	}

	var selectedID int

	if len(targetBucket.ChannelIDs) == 1 {
		selectedID = targetBucket.ChannelIDs[0]
		if _, ok := channelsIDM[selectedID]; !ok {
			channelSyncLock.RUnlock()
			return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", selectedID)
		}
	} else {
		sumWeight := targetBucket.SumWeight

		// smoothing factor and adjustment
		smoothingFactor := 1
		smoothingAdjustment := 0

		if sumWeight == 0 {
			sumWeight = len(targetBucket.ChannelIDs) * 100
			smoothingAdjustment = 100
		} else if sumWeight/len(targetBucket.ChannelIDs) < 10 {
			smoothingFactor = 100
		}

		totalWeight := sumWeight * smoothingFactor
		randomWeight := rand.Intn(totalWeight)

		found := false
		for _, channelId := range targetBucket.ChannelIDs {
			channel, ok := channelsIDM[channelId]
			if !ok {
				channelSyncLock.RUnlock()
				return nil, fmt.Errorf("数据库一致性错误，渠道# %d 不存在，请联系管理员修复", channelId)
			}
			randomWeight -= channel.GetWeight()*smoothingFactor + smoothingAdjustment
			if randomWeight < 0 {
				selectedID = channelId
				found = true
				break
			}
		}
		if !found {
			channelSyncLock.RUnlock()
			return nil, errors.New("channel not found")
		}
	}

	channelSyncLock.RUnlock()

	// Load full channel (with key, setting, etc.) on-demand
	return CacheGetFullChannel(selectedID)
}

// CacheGetFullChannel loads a full Channel (with key, setting, etc.) by ID.
// It first checks the fullChannelCache, then falls back to a DB query.
// The result is cached for subsequent requests until the next sync cycle.
func CacheGetFullChannel(id int) (*Channel, error) {
	if !common.MemoryCacheEnabled {
		return GetChannelById(id, true)
	}

	// Fast path: check the full channel cache
	fullChannelCacheLock.RLock()
	if c, ok := fullChannelCache[id]; ok {
		fullChannelCacheLock.RUnlock()
		return c, nil
	}
	fullChannelCacheLock.RUnlock()

	// Verify the channel exists in slim cache
	channelSyncLock.RLock()
	slim, exists := channelsIDM[id]
	channelSyncLock.RUnlock()
	if !exists {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}

	// Slow path: load from DB
	fullChannel, err := GetChannelById(id, true)
	if err != nil {
		return nil, err
	}

	// Preserve the polling index from the slim cache (maintained across syncs)
	if slim.ChannelInfo.IsMultiKey && slim.ChannelInfo.MultiKeyMode == constant.MultiKeyModePolling {
		fullChannel.ChannelInfo.MultiKeyPollingIndex = slim.ChannelInfo.MultiKeyPollingIndex
	}
	if fullChannel.ChannelInfo.IsMultiKey {
		fullChannel.Keys = fullChannel.GetKeys()
	}

	// Cache it
	fullChannelCacheLock.Lock()
	fullChannelCache[id] = fullChannel
	fullChannelCacheLock.Unlock()

	return fullChannel, nil
}

func CacheGetChannel(id int) (*Channel, error) {
	return CacheGetFullChannel(id)
}

func CacheGetChannelInfo(id int) (*ChannelInfo, error) {
	if !common.MemoryCacheEnabled {
		channel, err := GetChannelById(id, true)
		if err != nil {
			return nil, err
		}
		return &channel.ChannelInfo, nil
	}
	// ChannelInfo is available in the slim cache, no need to load full channel
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	c, ok := channelsIDM[id]
	if !ok {
		return nil, fmt.Errorf("渠道# %d，已不存在", id)
	}
	return &c.ChannelInfo, nil
}

func CacheUpdateChannelStatus(id int, status int) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel, ok := channelsIDM[id]; ok {
		channel.Status = status
	}
	if status != common.ChannelStatusEnabled {
		// Use reverse index for fast removal instead of scanning all groups/models
		gms, ok := channel2GroupModels[id]
		if !ok {
			return
		}
		channelWeight := 0
		if channel, ok := channelsIDM[id]; ok {
			channelWeight = channel.GetWeight()
		}
		for _, gm := range gms {
			// Remove from group2model2channels
			channels := group2model2channels[gm.Group][gm.Model]
			for i, channelId := range channels {
				if channelId == id {
					group2model2channels[gm.Group][gm.Model] = append(channels[:i], channels[i+1:]...)
					break
				}
			}
			// Remove from priority buckets
			buckets := group2model2priorityBuckets[gm.Group][gm.Model]
			if len(buckets) == 0 {
				continue
			}
			newBuckets := make([]channelPriorityBucket, 0, len(buckets))
			for _, bucket := range buckets {
				removeIdx := -1
				for i, channelID := range bucket.ChannelIDs {
					if channelID == id {
						removeIdx = i
						break
					}
				}
				if removeIdx >= 0 {
					bucket.ChannelIDs = append(bucket.ChannelIDs[:removeIdx], bucket.ChannelIDs[removeIdx+1:]...)
					bucket.SumWeight -= channelWeight
					if bucket.SumWeight < 0 {
						bucket.SumWeight = 0
					}
				}
				if len(bucket.ChannelIDs) > 0 {
					newBuckets = append(newBuckets, bucket)
				}
			}
			group2model2priorityBuckets[gm.Group][gm.Model] = newBuckets
		}
		// Remove from reverse index
		delete(channel2GroupModels, id)
	}

	// Also invalidate the full channel cache entry
	fullChannelCacheLock.Lock()
	delete(fullChannelCache, id)
	fullChannelCacheLock.Unlock()
}

func CacheUpdateChannel(channel *Channel) {
	if !common.MemoryCacheEnabled {
		return
	}
	channelSyncLock.Lock()
	defer channelSyncLock.Unlock()
	if channel == nil {
		return
	}
	channelsIDM[channel.Id] = channel

	// Also update the full channel cache
	fullChannelCacheLock.Lock()
	fullChannelCache[channel.Id] = channel
	fullChannelCacheLock.Unlock()
}
