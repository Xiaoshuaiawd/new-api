package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTokenTestUser(t *testing.T, id int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "token_test_user",
		Role:     common.RoleAdminUser,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, DB.Create(user).Error)
}

func TestRenewSubscriptionTokenByID_RenewsExpiredToken(t *testing.T) {
	truncateTables(t)
	seedTokenTestUser(t, 101)

	now := time.Now().Unix()
	token := &Token{
		Id:                1001,
		UserId:            101,
		Key:               "renew-expired-token",
		Name:              "expired-sub-token",
		Status:            common.TokenStatusExpired,
		AccessedTime:      now - 120,
		ExpiredTime:       now - 60,
		RemainQuota:       0,
		UsedQuota:         3000,
		PlanId:            1,
		PlanTitle:         "Monthly Plan",
		ActivationTime:    now - 30*24*3600,
		PlanDurationUnit:  SubscriptionDurationDay,
		PlanDurationValue: 30,
		PlanAmountTotal:   3000,
		PlanResetPeriod:   SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(token).Error)

	before := GetDBTimestamp()
	renewed, err := RenewSubscriptionTokenByID(token.Id, token.UserId, 0)
	after := GetDBTimestamp()

	require.NoError(t, err)
	require.NotNil(t, renewed)
	assert.Equal(t, common.TokenStatusEnabled, renewed.Status)
	assert.GreaterOrEqual(t, renewed.ActivationTime, before)
	assert.LessOrEqual(t, renewed.ActivationTime, after)
	assert.Greater(t, renewed.ExpiredTime, after)
	assert.Equal(t, 3000, renewed.RemainQuota)
	assert.Equal(t, 0, renewed.UsedQuota)
	assert.Equal(t, int64(0), renewed.NextResetTime)

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, renewed.Status, stored.Status)
	assert.Equal(t, renewed.ActivationTime, stored.ActivationTime)
	assert.Equal(t, renewed.ExpiredTime, stored.ExpiredTime)
	assert.Equal(t, renewed.RemainQuota, stored.RemainQuota)
	assert.Equal(t, renewed.UsedQuota, stored.UsedQuota)
}

func TestRenewSubscriptionTokenByID_QueuesActiveTokenRenewal(t *testing.T) {
	truncateTables(t)
	seedTokenTestUser(t, 102)

	plan := &SubscriptionPlan{
		Id:                      202,
		Title:                   "Queued Pro Plan",
		DurationUnit:            SubscriptionDurationDay,
		DurationValue:           15,
		TotalAmount:             6000,
		QuotaResetPeriod:        SubscriptionResetNever,
		QuotaResetCustomSeconds: 0,
	}
	require.NoError(t, DB.Create(plan).Error)

	now := time.Now().Unix()
	token := &Token{
		Id:                1002,
		UserId:            102,
		Key:               "renew-active-token",
		Name:              "active-sub-token",
		Status:            common.TokenStatusEnabled,
		AccessedTime:      now - 60,
		ExpiredTime:       now + 24*3600,
		RemainQuota:       2000,
		UsedQuota:         1000,
		PlanId:            2,
		PlanTitle:         "Active Plan",
		ActivationTime:    now - 12*3600,
		PlanDurationUnit:  SubscriptionDurationDay,
		PlanDurationValue: 30,
		PlanAmountTotal:   3000,
		PlanResetPeriod:   SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(token).Error)

	renewed, err := RenewSubscriptionTokenByID(token.Id, token.UserId, plan.Id)

	require.NoError(t, err)
	require.NotNil(t, renewed)
	assert.Equal(t, token.PlanId, renewed.PlanId)
	assert.Equal(t, token.PlanTitle, renewed.PlanTitle)
	assert.Equal(t, token.ActivationTime, renewed.ActivationTime)
	assert.Equal(t, token.ExpiredTime, renewed.ExpiredTime)
	assert.Equal(t, token.RemainQuota, renewed.RemainQuota)
	assert.Equal(t, token.UsedQuota, renewed.UsedQuota)
	assert.Equal(t, 1, renewed.RenewalQueuedCount)
	assert.Equal(t, plan.Id, renewed.NextRenewalPlanId)
	assert.Equal(t, plan.Title, renewed.NextRenewalPlanTitle)

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, token.Status, stored.Status)
	assert.Equal(t, token.ExpiredTime, stored.ExpiredTime)
	assert.Equal(t, token.RemainQuota, stored.RemainQuota)
	assert.Equal(t, token.UsedQuota, stored.UsedQuota)
	assert.NotEmpty(t, stored.RenewalPlanQueue)
	stored.syncRenewalQueueState(nil)
	assert.Equal(t, 1, stored.RenewalQueuedCount)
	assert.Equal(t, plan.Title, stored.NextRenewalPlanTitle)
}

func TestRenewSubscriptionTokenByID_UsesSelectedPlan(t *testing.T) {
	truncateTables(t)
	seedTokenTestUser(t, 103)

	plan := &SubscriptionPlan{
		Id:                      201,
		Title:                   "Pro Plan",
		DurationUnit:            SubscriptionDurationDay,
		DurationValue:           7,
		TotalAmount:             9000,
		QuotaResetPeriod:        SubscriptionResetDaily,
		QuotaResetCustomSeconds: 0,
	}
	require.NoError(t, DB.Create(plan).Error)

	now := time.Now().Unix()
	token := &Token{
		Id:                1003,
		UserId:            103,
		Key:               "renew-plan-token",
		Name:              "plan-switch-token",
		Status:            common.TokenStatusExpired,
		AccessedTime:      now - 60,
		ExpiredTime:       now - 1,
		RemainQuota:       0,
		UsedQuota:         1000,
		PlanId:            3,
		PlanTitle:         "Old Plan",
		ActivationTime:    now - 3*24*3600,
		PlanDurationUnit:  SubscriptionDurationDay,
		PlanDurationValue: 3,
		PlanAmountTotal:   1000,
		PlanResetPeriod:   SubscriptionResetNever,
	}
	require.NoError(t, DB.Create(token).Error)

	renewed, err := RenewSubscriptionTokenByID(token.Id, token.UserId, plan.Id)

	require.NoError(t, err)
	require.NotNil(t, renewed)
	assert.Equal(t, plan.Id, renewed.PlanId)
	assert.Equal(t, plan.Title, renewed.PlanTitle)
	assert.Equal(t, plan.DurationUnit, renewed.PlanDurationUnit)
	assert.Equal(t, plan.DurationValue, renewed.PlanDurationValue)
	assert.Equal(t, int(plan.TotalAmount), renewed.PlanAmountTotal)
	assert.Equal(t, plan.QuotaResetPeriod, renewed.PlanResetPeriod)
	assert.Equal(t, int(plan.TotalAmount), renewed.RemainQuota)
	assert.Equal(t, 0, renewed.UsedQuota)
	assert.Greater(t, renewed.NextResetTime, renewed.ActivationTime)
}

func TestResetDueSubscriptionTokens_ResetsQuota(t *testing.T) {
	truncateTables(t)
	seedTokenTestUser(t, 104)

	now := time.Now().Unix()
	token := &Token{
		Id:                1004,
		UserId:            104,
		Key:               "due-reset-token",
		Name:              "due-reset-token",
		Status:            common.TokenStatusExhausted,
		AccessedTime:      now - 3600,
		ActivationTime:    now - 48*3600,
		ExpiredTime:       now + 48*3600,
		LastResetTime:     now - 24*3600,
		NextResetTime:     now - 60,
		RemainQuota:       0,
		UsedQuota:         5000,
		PlanId:            4,
		PlanTitle:         "Daily Reset Plan",
		PlanDurationUnit:  SubscriptionDurationDay,
		PlanDurationValue: 7,
		PlanAmountTotal:   5000,
		PlanResetPeriod:   SubscriptionResetDaily,
	}
	require.NoError(t, DB.Create(token).Error)

	resetCount, err := ResetDueSubscriptionTokens(10)

	require.NoError(t, err)
	assert.Equal(t, 1, resetCount)

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, common.TokenStatusEnabled, stored.Status)
	assert.Equal(t, 5000, stored.RemainQuota)
	assert.Equal(t, 0, stored.UsedQuota)
	assert.Greater(t, stored.LastResetTime, token.LastResetTime)
	assert.Greater(t, stored.NextResetTime, now)
}

func TestEnsureSubscriptionTokenReady_ActivatesQueuedRenewalAfterExpiry(t *testing.T) {
	truncateTables(t)
	seedTokenTestUser(t, 105)

	plan := &SubscriptionPlan{
		Id:                      203,
		Title:                   "Follow-up Plan",
		DurationUnit:            SubscriptionDurationDay,
		DurationValue:           7,
		TotalAmount:             7000,
		QuotaResetPeriod:        SubscriptionResetDaily,
		QuotaResetCustomSeconds: 0,
	}
	require.NoError(t, DB.Create(plan).Error)

	now := time.Now().Unix()
	expiredAt := now - 3600
	token := &Token{
		Id:                1005,
		UserId:            105,
		Key:               "queued-renewal-token",
		Name:              "queued-renewal-token",
		Status:            common.TokenStatusEnabled,
		AccessedTime:      now - 7200,
		ActivationTime:    now - 30*24*3600,
		ExpiredTime:       expiredAt,
		RemainQuota:       0,
		UsedQuota:         3000,
		PlanId:            5,
		PlanTitle:         "Legacy Plan",
		PlanDurationUnit:  SubscriptionDurationDay,
		PlanDurationValue: 30,
		PlanAmountTotal:   3000,
		PlanResetPeriod:   SubscriptionResetNever,
	}
	queueSnapshot, err := buildRenewalSnapshotFromPlan(plan, now-24*3600)
	require.NoError(t, err)
	require.NoError(t, token.setRenewalPlanQueue([]tokenRenewalSnapshot{*queueSnapshot}))
	require.NoError(t, DB.Create(token).Error)

	stored, err := GetTokenById(token.Id)
	require.NoError(t, err)
	require.NotNil(t, stored)

	ready, err := ensureSubscriptionTokenReady(stored)
	require.NoError(t, err)
	require.NotNil(t, ready)
	assert.Equal(t, plan.Id, ready.PlanId)
	assert.Equal(t, plan.Title, ready.PlanTitle)
	assert.Equal(t, expiredAt, ready.ActivationTime)
	assert.Greater(t, ready.ExpiredTime, now)
	assert.Equal(t, int(plan.TotalAmount), ready.RemainQuota)
	assert.Equal(t, 0, ready.UsedQuota)
	assert.Equal(t, 0, ready.RenewalQueuedCount)
	assert.Empty(t, ready.NextRenewalPlanTitle)

	var latest Token
	require.NoError(t, DB.First(&latest, token.Id).Error)
	latest.syncRenewalQueueState(nil)
	assert.Equal(t, plan.Id, latest.PlanId)
	assert.Equal(t, expiredAt, latest.ActivationTime)
	assert.Greater(t, latest.NextResetTime, now)
	assert.Empty(t, latest.RenewalPlanQueue)
}
