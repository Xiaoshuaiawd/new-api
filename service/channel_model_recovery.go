package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

// ScheduledModelRecovery stores information about scheduled model recovery tasks
type ScheduledModelRecovery struct {
	ChannelID   int
	ModelName   string
	ChannelName string
	CancelFunc  func() // Function to cancel the recovery task
}

var (
	// modelRecoveryTasks tracks all scheduled model recovery tasks
	// Key: "channelID:modelName"
	modelRecoveryTasks = make(map[string]*ScheduledModelRecovery)
	modelRecoveryMutex sync.RWMutex

	// TestChannelModelFunc is a function pointer that will be set by the controller package
	// to avoid circular dependency
	TestChannelModelFunc func(channel *model.Channel, modelName string) bool
)

const (
	// KeywordDisablePrefix marks models disabled by keyword errors (for auto-recovery via testing)
	KeywordDisablePrefix = "[AUTO_KEYWORD]"
	// RecoveryTestInterval is the interval between recovery tests (1 minute)
	RecoveryTestInterval = 60 * time.Second
)

// ScheduleModelRecoveryTest schedules periodic testing for a disabled model
// This is called when a model is disabled due to keyword errors
func ScheduleModelRecoveryTest(channelID int, channelName string, modelName string) {
	key := fmt.Sprintf("%d:%s", channelID, modelName)

	modelRecoveryMutex.Lock()
	// Check if recovery task already exists
	if _, exists := modelRecoveryTasks[key]; exists {
		modelRecoveryMutex.Unlock()
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」的恢复拨测任务已存在，跳过创建", channelID, modelName))
		return
	}

	// Create cancellation context
	done := make(chan struct{})
	cancelFunc := func() {
		close(done)
	}

	recovery := &ScheduledModelRecovery{
		ChannelID:   channelID,
		ModelName:   modelName,
		ChannelName: channelName,
		CancelFunc:  cancelFunc,
	}
	modelRecoveryTasks[key] = recovery
	modelRecoveryMutex.Unlock()

	common.SysLog(fmt.Sprintf("渠道「%s」(#%d) 模型「%s」已启动每分钟自动拨测恢复任务", channelName, channelID, modelName))

	// Start recovery testing goroutine
	gopool.Go(func() {
		ticker := time.NewTicker(RecoveryTestInterval)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				// Task cancelled
				modelRecoveryMutex.Lock()
				delete(modelRecoveryTasks, key)
				modelRecoveryMutex.Unlock()
				common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」的恢复拨测任务已停止", channelID, modelName))
				return
			case <-ticker.C:
				// Perform recovery test
				performModelRecoveryTest(channelID, channelName, modelName, key, done)
			}
		}
	})
}

// performModelRecoveryTest performs a single recovery test for a disabled model
func performModelRecoveryTest(channelID int, channelName string, modelName string, taskKey string, done chan struct{}) {
	// Get the channel
	channel, err := model.CacheGetChannel(channelID)
	if err != nil {
		common.SysLog(fmt.Sprintf("恢复拨测失败：无法获取渠道 #%d，错误：%v", channelID, err))
		return
	}

	// Check if the model is still disabled
	if !channel.IsModelDisabled(modelName) {
		// Model is no longer disabled, stop the recovery task
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」已不再禁用，停止恢复拨测任务", channelID, modelName))
		CancelModelRecoveryTest(channelID, modelName)
		return
	}

	// Check if the disable reason still has the keyword prefix
	if channel.ChannelInfo.DisabledModels != nil {
		if disabledInfo, exists := channel.ChannelInfo.DisabledModels[modelName]; exists {
			// Only continue testing if the disable reason starts with the keyword prefix
			if len(disabledInfo.Reason) < len(KeywordDisablePrefix) ||
				disabledInfo.Reason[:len(KeywordDisablePrefix)] != KeywordDisablePrefix {
				common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」禁用原因已变更（可能被手动禁用或 RPM 限流），停止恢复拨测任务", channelID, modelName))
				CancelModelRecoveryTest(channelID, modelName)
				return
			}
		}
	}

	common.SysLog(fmt.Sprintf("开始拨测渠道 #%d 模型「%s」", channelID, modelName))

	// Import controller package functions (we'll use the testChannel function)
	// Note: We can't directly import controller here due to circular dependency
	// Instead, we'll need to expose a testing function in the service layer

	// For now, we'll create a simple test request
	// This will be refined later to use the actual testChannel function
	success := testChannelModel(channel, modelName)

	if success {
		// Test passed, enable the model
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」拨测成功，准备恢复启用", channelID, modelName))

		enabled, err := model.EnableChannelModel(channelID, modelName)
		if err != nil {
			common.SysLog(fmt.Sprintf("自动恢复模型失败：渠道 #%d 模型「%s」，错误：%v", channelID, modelName, err))
			return
		}

		if enabled {
			common.SysLog(fmt.Sprintf("渠道「%s」(#%d) 模型「%s」拨测成功，已自动恢复启用", channelName, channelID, modelName))
			NotifyRootUser(formatNotifyModelType(channelID, modelName),
				fmt.Sprintf("通道「%s」（#%d）模型「%s」已自动恢复", channelName, channelID, modelName),
				fmt.Sprintf("通道「%s」（#%d）模型「%s」拨测成功，已自动恢复启用", channelName, channelID, modelName))

			// Stop the recovery task
			CancelModelRecoveryTest(channelID, modelName)
		}
	} else {
		common.SysLog(fmt.Sprintf("渠道 #%d 模型「%s」拨测失败，将在 1 分钟后重试", channelID, modelName))
	}
}

// testChannelModel performs a test on a channel model using the injected test function
func testChannelModel(channel *model.Channel, modelName string) bool {
	if TestChannelModelFunc == nil {
		common.SysLog("警告：TestChannelModelFunc 未设置，无法执行模型拨测")
		return false
	}
	return TestChannelModelFunc(channel, modelName)
}

// CancelModelRecoveryTest cancels a scheduled model recovery test
func CancelModelRecoveryTest(channelID int, modelName string) {
	key := fmt.Sprintf("%d:%s", channelID, modelName)

	modelRecoveryMutex.Lock()
	defer modelRecoveryMutex.Unlock()

	if recovery, exists := modelRecoveryTasks[key]; exists {
		recovery.CancelFunc()
		delete(modelRecoveryTasks, key)
		common.SysLog(fmt.Sprintf("已取消渠道 #%d 模型「%s」的恢复拨测任务", channelID, modelName))
	}
}

// GetActiveRecoveryTasks returns the list of active recovery tasks (for monitoring/debugging)
func GetActiveRecoveryTasks() []ScheduledModelRecovery {
	modelRecoveryMutex.RLock()
	defer modelRecoveryMutex.RUnlock()

	tasks := make([]ScheduledModelRecovery, 0, len(modelRecoveryTasks))
	for _, task := range modelRecoveryTasks {
		tasks = append(tasks, *task)
	}
	return tasks
}

// IsKeywordDisabledModel checks if a model was disabled by keyword error
func IsKeywordDisabledModel(disableReason string) bool {
	if disableReason == "" {
		return false
	}
	return strings.HasPrefix(disableReason, KeywordDisablePrefix)
}

// InitModelRecoveryTasks initializes recovery tasks for all currently disabled models with keyword prefix
// This should be called on application startup
func InitModelRecoveryTasks() {
	common.SysLog("初始化模型恢复拨测任务...")

	channels, err := model.GetAllChannels(0, 0, true, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("初始化模型恢复拨测任务失败：无法获取渠道列表，错误：%v", err))
		return
	}

	taskCount := 0
	for _, channel := range channels {
		if channel.ChannelInfo.DisabledModels == nil {
			continue
		}

		for modelName, disabledInfo := range channel.ChannelInfo.DisabledModels {
			// Only schedule recovery for keyword-disabled models
			if IsKeywordDisabledModel(disabledInfo.Reason) {
				ScheduleModelRecoveryTest(channel.Id, channel.Name, modelName)
				taskCount++
			}
		}
	}

	common.SysLog(fmt.Sprintf("已初始化 %d 个模型恢复拨测任务", taskCount))
}
