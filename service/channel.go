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
	"github.com/QuantumNous/new-api/types"
)

func formatNotifyType(channelId int, status int) string {
	return fmt.Sprintf("%s_%d_%d", dto.NotifyTypeChannelUpdate, channelId, status)
}

// disable & notify
func DisableChannel(channelError types.ChannelError, reason string) {
	common.SysLog(fmt.Sprintf("通道「%s」（#%d）发生错误，准备禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason))
	common.SysLog(fmt.Sprintf("通道信息 - AutoBan: %v, ModelName: %s", channelError.AutoBan, channelError.ModelName))

	// 检查是否启用自动禁用功能
	if !channelError.AutoBan {
		common.SysLog(fmt.Sprintf("通道「%s」（#%d）未启用自动禁用功能，跳过禁用操作", channelError.ChannelName, channelError.ChannelId))
		return
	}

	// 检查是否启用了"失败时拆分模型禁用"功能
	common.SysLog(fmt.Sprintf("检查失败时拆分模型禁用功能 - DisableModelOnFailureEnabled: %v", common.DisableModelOnFailureEnabled))

	if common.DisableModelOnFailureEnabled && channelError.ModelName != "" {
		// 只禁用当前模型而不是整个渠道
		common.SysLog(fmt.Sprintf("启用了失败时拆分模型禁用，准备禁用模型「%s」", channelError.ModelName))
		success := model.DisableChannelModel(channelError.ChannelId, channelError.ModelName, reason)
		if success {
			subject := fmt.Sprintf("通道「%s」（#%d）的模型「%s」已被禁用", channelError.ChannelName, channelError.ChannelId, channelError.ModelName)
			content := fmt.Sprintf("通道「%s」（#%d）的模型「%s」已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, channelError.ModelName, reason)
			NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
		} else {
			common.SysError(fmt.Sprintf("禁用模型「%s」失败", channelError.ModelName))
		}
		return
	}

	// 否则，按原逻辑禁用整个渠道
	common.SysLog(fmt.Sprintf("准备禁用整个渠道「%s」（#%d）", channelError.ChannelName, channelError.ChannelId))

	success := model.UpdateChannelStatus(channelError.ChannelId, channelError.UsingKey, common.ChannelStatusAutoDisabled, reason)
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被禁用", channelError.ChannelName, channelError.ChannelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被禁用，原因：%s", channelError.ChannelName, channelError.ChannelId, reason)
		NotifyRootUser(formatNotifyType(channelError.ChannelId, common.ChannelStatusAutoDisabled), subject, content)
	} else {
		common.SysError(fmt.Sprintf("禁用渠道「%s」（#%d）失败", channelError.ChannelName, channelError.ChannelId))
	}
}

func EnableChannel(channelId int, usingKey string, channelName string) {
	success := model.UpdateChannelStatus(channelId, usingKey, common.ChannelStatusEnabled, "")
	if success {
		subject := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		content := fmt.Sprintf("通道「%s」（#%d）已被启用", channelName, channelId)
		NotifyRootUser(formatNotifyType(channelId, common.ChannelStatusEnabled), subject, content)
	}
}

func ShouldDisableChannel(channelType int, err *types.NewAPIError) bool {
	if !common.AutomaticDisableChannelEnabled {
		common.SysLog("自动禁用通道功能未启用")
		return false
	}
	if err == nil {
		return false
	}
	if types.IsChannelError(err) {
		common.SysLog(fmt.Sprintf("检测到通道错误，应该禁用"))
		return true
	}
	if types.IsSkipRetryError(err) {
		common.SysLog(fmt.Sprintf("检测到跳过重试错误，不禁用"))
		return false
	}
	if operation_setting.ShouldDisableByStatusCode(err.StatusCode) {
		common.SysLog(fmt.Sprintf("状态码 %d 匹配自动禁用规则", err.StatusCode))
		return true
	}
	//if err.StatusCode == http.StatusUnauthorized {
	//	return true
	//}
	if err.StatusCode == http.StatusForbidden {
		switch channelType {
		case constant.ChannelTypeGemini:
			return true
		}
	}
	oaiErr := err.ToOpenAIError()

	// 记录错误信息用于调试
	common.SysLog(fmt.Sprintf("检查错误代码和类型 - Code: %s, Type: %s, Message: %s", oaiErr.Code, oaiErr.Type, err.Error()))

	switch oaiErr.Code {
	case "invalid_api_key":
		common.SysLog("检测到 invalid_api_key 错误")
		return true
	case "account_deactivated":
		common.SysLog("检测到 account_deactivated 错误")
		return true
	case "billing_not_active":
		common.SysLog("检测到 billing_not_active 错误")
		return true
	case "pre_consume_token_quota_failed":
		common.SysLog("检测到 pre_consume_token_quota_failed 错误")
		return true
	case "Arrearage":
		common.SysLog("检测到 Arrearage 错误")
		return true
	case "rate_limit_exceeded": // 速率限制错误
		common.SysLog("检测到 rate_limit_exceeded 错误")
		return true
	}
	switch oaiErr.Type {
	case "insufficient_quota":
		common.SysLog("检测到 insufficient_quota 错误")
		return true
	case "insufficient_user_quota":
		common.SysLog("检测到 insufficient_user_quota 错误")
		return true
	// https://docs.anthropic.com/claude/reference/errors
	case "authentication_error":
		common.SysLog("检测到 authentication_error 错误")
		return true
	case "permission_error":
		common.SysLog("检测到 permission_error 错误")
		return true
	case "forbidden":
		common.SysLog("检测到 forbidden 错误")
		return true
	}

	lowerMessage := strings.ToLower(err.Error())
	common.SysLog(fmt.Sprintf("检查关键词匹配 - 错误消息（小写）: %s", lowerMessage))
	common.SysLog(fmt.Sprintf("自动禁用关键词列表: %v", operation_setting.AutomaticDisableKeywords))

	search, matchedWords := AcSearch(lowerMessage, operation_setting.AutomaticDisableKeywords, true)
	if search {
		common.SysLog(fmt.Sprintf("匹配到自动禁用关键词: %v", matchedWords))
	} else {
		common.SysLog("未匹配到任何自动禁用关键词")
	}
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
