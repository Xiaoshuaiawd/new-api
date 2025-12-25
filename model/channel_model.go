package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
)

func (channel *Channel) IsModelDisabled(modelName string) bool {
	if channel == nil || modelName == "" {
		return false
	}
	if channel.ChannelInfo.DisabledModels == nil {
		return false
	}
	_, ok := channel.ChannelInfo.DisabledModels[modelName]
	return ok
}

func DisableChannelModel(channelId int, modelName string, reason string) (bool, error) {
	if channelId <= 0 {
		return false, errors.New("invalid channel id")
	}
	if modelName == "" {
		return false, errors.New("model name is empty")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	channel := &Channel{Id: channelId}
	if err := tx.First(channel, "id = ?", channelId).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	if channel.ChannelInfo.DisabledModels == nil {
		channel.ChannelInfo.DisabledModels = make(map[string]ChannelModelDisabledInfo)
	}
	if _, exists := channel.ChannelInfo.DisabledModels[modelName]; exists {
		tx.Rollback()
		return false, nil
	}

	channel.ChannelInfo.DisabledModels[modelName] = ChannelModelDisabledInfo{
		Reason:       reason,
		DisabledTime: common.GetTimestamp(),
	}

	if err := tx.Model(channel).Update("channel_info", channel.ChannelInfo).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	if err := tx.Model(&Ability{}).
		Where("channel_id = ? AND model = ?", channelId, modelName).
		Select("enabled").
		Update("enabled", false).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	if err := tx.Commit().Error; err != nil {
		return false, err
	}

	if common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return true, nil
}

func EnableChannelModel(channelId int, modelName string) (bool, error) {
	if channelId <= 0 {
		return false, errors.New("invalid channel id")
	}
	if modelName == "" {
		return false, errors.New("model name is empty")
	}

	tx := DB.Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	channel := &Channel{Id: channelId}
	if err := tx.First(channel, "id = ?", channelId).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	if channel.ChannelInfo.DisabledModels == nil {
		tx.Rollback()
		return false, nil
	}
	if _, exists := channel.ChannelInfo.DisabledModels[modelName]; !exists {
		tx.Rollback()
		return false, nil
	}

	delete(channel.ChannelInfo.DisabledModels, modelName)
	if len(channel.ChannelInfo.DisabledModels) == 0 {
		channel.ChannelInfo.DisabledModels = nil
	}

	if err := tx.Model(channel).Update("channel_info", channel.ChannelInfo).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	// Only enable the ability when the channel itself is enabled.
	enableAbility := channel.Status == common.ChannelStatusEnabled
	if err := tx.Model(&Ability{}).
		Where("channel_id = ? AND model = ?", channelId, modelName).
		Select("enabled").
		Update("enabled", enableAbility).Error; err != nil {
		tx.Rollback()
		return false, err
	}

	if err := tx.Commit().Error; err != nil {
		return false, err
	}

	if common.MemoryCacheEnabled {
		InitChannelCache()
	}
	return true, nil
}
