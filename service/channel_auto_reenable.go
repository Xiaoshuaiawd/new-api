package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/bytedance/gopkg/util/gopool"
)

type scheduledChannelReenable struct {
	ChannelID  int
	CancelFunc func()
}

var (
	channelReenableTasks = make(map[int]*scheduledChannelReenable)
	channelReenableMutex sync.Mutex
)

func scheduleChannelReenable(channelID int, minutes int) {
	if minutes <= 0 {
		return
	}

	channelReenableMutex.Lock()
	if _, exists := channelReenableTasks[channelID]; exists {
		channelReenableMutex.Unlock()
		return
	}

	done := make(chan struct{})
	task := &scheduledChannelReenable{
		ChannelID: channelID,
		CancelFunc: func() {
			select {
			case <-done:
				// already closed
			default:
				close(done)
			}
		},
	}
	channelReenableTasks[channelID] = task
	channelReenableMutex.Unlock()

	delay := time.Duration(minutes) * time.Minute
	common.SysLog(fmt.Sprintf("渠道 #%d 已设置自动禁用时长 %d 分钟，将在 %s 后尝试自动恢复", channelID, minutes, delay))

	gopool.Go(func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-done:
			return
		case <-timer.C:
		}

		// Remove task record when it fires (best-effort).
		channelReenableMutex.Lock()
		delete(channelReenableTasks, channelID)
		channelReenableMutex.Unlock()

		// Best-effort re-enable: only if still auto-disabled.
		ch, err := model.GetChannelById(channelID, true)
		if err != nil {
			return
		}
		if ch.Status != common.ChannelStatusAutoDisabled {
			return
		}

		// Clear the auto-disabled-until marker and enable.
		ch.ChannelInfo.AutoDisabledUntil = 0
		_ = ch.SaveWithoutKey()
		EnableChannel(ch.Id, "", ch.Name)
	})
}

func cancelChannelReenable(channelID int) {
	channelReenableMutex.Lock()
	task, ok := channelReenableTasks[channelID]
	if ok {
		delete(channelReenableTasks, channelID)
	}
	channelReenableMutex.Unlock()
	if ok && task.CancelFunc != nil {
		task.CancelFunc()
	}
}
