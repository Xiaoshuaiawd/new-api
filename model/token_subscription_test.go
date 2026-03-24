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
	renewed, err := RenewSubscriptionTokenByID(token.Id, token.UserId)
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

func TestRenewSubscriptionTokenByID_RejectsActiveToken(t *testing.T) {
	truncateTables(t)
	seedTokenTestUser(t, 102)

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

	renewed, err := RenewSubscriptionTokenByID(token.Id, token.UserId)

	require.Error(t, err)
	assert.Nil(t, renewed)
	assert.Contains(t, err.Error(), "暂不支持提前续费")

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, token.Status, stored.Status)
	assert.Equal(t, token.ExpiredTime, stored.ExpiredTime)
	assert.Equal(t, token.RemainQuota, stored.RemainQuota)
	assert.Equal(t, token.UsedQuota, stored.UsedQuota)
}
