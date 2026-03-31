package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalcNextResetTime_DailyUsesShanghaiMidnight(t *testing.T) {
	t.Setenv("SUBSCRIPTION_RESET_TIMEZONE", "Asia/Shanghai")

	plan := &SubscriptionPlan{
		QuotaResetPeriod: SubscriptionResetDaily,
	}

	base := time.Date(2026, 4, 1, 16, 30, 0, 0, time.UTC) // 2026-04-02 00:30:00 +0800
	nextReset := calcNextResetTime(base, plan, 0)

	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	expected := time.Date(2026, 4, 3, 0, 0, 0, 0, loc).Unix()
	assert.Equal(t, expected, nextReset)
}

func TestCalcNextResetTime_WeeklyUsesShanghaiMondayMidnight(t *testing.T) {
	t.Setenv("SUBSCRIPTION_RESET_TIMEZONE", "Asia/Shanghai")

	plan := &SubscriptionPlan{
		QuotaResetPeriod: SubscriptionResetWeekly,
	}

	base := time.Date(2026, 4, 3, 18, 0, 0, 0, time.UTC) // 2026-04-04 02:00:00 +0800, Saturday
	nextReset := calcNextResetTime(base, plan, 0)

	loc, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	expected := time.Date(2026, 4, 6, 0, 0, 0, 0, loc).Unix()
	assert.Equal(t, expected, nextReset)
}
