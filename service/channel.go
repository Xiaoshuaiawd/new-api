package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

func formatNotifyModelType(channelId int, modelName string) string {
	return fmt.Sprintf("%s_%d_model_%s", dto.NotifyTypeChannelUpdate, channelId, modelName)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	}
}

// DisableChannelModel disables only the current model for this channel.
func DisableChannelModel(channelError types.ChannelError, modelName string, reason string) {
	matchName := ratio_setting.FormatMatchingModelName(modelName)
	if matchName == "" {
		matchName = modelName
	}

	common.SysLog(fmt.Sprintf("通道「%s」（#%d）模型「%s」发生错误，准备禁用该模型，原因：%s", channelError.ChannelName, channelError.ChannelId, matchName, reason))

	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用模型操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	changed, err := model.DisableChannelModel(channelError.ChannelId, matchName, reason)
	if err != nil {
		common.SysLog(fmt.Sprintf("禁用模型失败：通道「%s」（#%d）模型「%s」，错误：%v", channelError.ChannelName, channelError.ChannelId, matchName, err))
		return
	}
	if !changed {
		return
	}

	subject := fmt.Sprintf("通道「%s」（#%d）模型「%s」已被禁用", channelError.ChannelName, channelError.ChannelId, matchName)
	content := fmt.Sprintf("通道「%s」（#%d）模型「%s」已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, matchName, reason)
	NotifyRootUser(formatNotifyModelType(channelError.ChannelId, matchName), subject, content)
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannelModel(channelType int, modelName string, err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil || modelName == "" {
		return false
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if len(operation_setting.AutomaticDisableModelKeywords) == 0 {
		return false
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableModelKeywords, true)
	_ = channelType // reserved for future channel-type specific rules
	return search
}

func ShouldDisableChannel(channelType int, err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		return true
	}
	if types.IsSkipRetryError(err) {
		return false
	}
	if err.StatusCode == http.StatusUnauthorized {
		return true
	}
	if err.StatusCode == http.StatusForbidden {
		switch channelType {
		case constant.ChannelTypeGemini:
			return true
		}
	}
	if (strings.Contains(err.Error(), "StatusCode: 401") ||
		strings.Contains(err.Error(), "invalid aws secret key") ||
		strings.Contains(err.Error(), "StatusCode: 403")) &&
		channelType == constant.ChannelTypeAws {
		return true
	}
	oaiErr := err.ToOpenAIError()
	switch oaiErr.Code {
	case "invalid_api_key":
		return true
	case "account_deactivated":
		return true
	case "billing_not_active":
		return true
	case "pre_consume_token_quota_failed":
		return true
	case "Arrearage":
		return true
	}
	switch oaiErr.Type {
	case "insufficient_quota":
		return true
	case "insufficient_user_quota":
		return true
	// https://docs.anthropic.com/claude/reference/errors
	case "authentication_error":
		return true
	case "permission_error":
		return true
	case "forbidden":
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	search, _ := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	return search
}

func ShouldEnableChannel(newAPIError *types.NewAPIError, status int) bool {
	if !common.AutomaticEnableChannelEnabled {
		return false
	}
	if newAPIError != nil {
		return false
	}
	if status != common.ChannelStatusAutoDisabled {
		return false
	}
	return true
}
