package controller

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

const maxIssuedSubscriptionTokensPerRequest = 100

type AdminIssueSubscriptionTokensRequest struct {
	Name               string  `json:"name"`
	PlanId             int     `json:"plan_id"`
	Group              string  `json:"group"`
	CrossGroupRetry    bool    `json:"cross_group_retry"`
	ModelLimitsEnabled bool    `json:"model_limits_enabled"`
	ModelLimits        string  `json:"model_limits"`
	AllowIps           *string `json:"allow_ips"`
	TokenCount         int     `json:"token_count"`
}

type RenewSubscriptionTokenRequest struct {
	PlanId int `json:"plan_id"`
}

func buildIssuedTokenName(baseName string, fallback string, key string, useSuffix bool) string {
	name := strings.TrimSpace(baseName)
	if name == "" {
		name = fallback
	}
	if name == "" {
		name = "subscription-token"
	}
	if !useSuffix {
		return name
	}
	suffix := key
	if len(suffix) > 6 {
		suffix = suffix[:6]
	}
	return fmt.Sprintf("%s-%s", name, suffix)
}

func GetAllTokens(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	tokens, err := model.GetAllUserTokens(userId, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	total, _ := model.CountUserTokens(userId)
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tokens)
	common.ApiSuccess(c, pageInfo)
	return
}

func SearchTokens(c *gin.Context) {
	userId := c.GetInt("id")
	keyword := c.Query("keyword")
	token := c.Query("token")

	pageInfo := common.GetPageQuery(c)

	tokens, total, err := model.SearchUserTokens(userId, keyword, token, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(tokens)
	common.ApiSuccess(c, pageInfo)
	return
}

func GetToken(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, err := model.GetTokenByIds(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    token,
	})
	return
}

func GetTokenStatus(c *gin.Context) {
	tokenId := c.GetInt("token_id")
	userId := c.GetInt("id")
	token, err := model.GetTokenByIds(tokenId, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}
	c.JSON(http.StatusOK, gin.H{
		"object":          "credit_summary",
		"total_granted":   token.RemainQuota,
		"total_used":      0, // not supported currently
		"total_available": token.RemainQuota,
		"expires_at":      expiredAt * 1000,
	})
}

func GetTokenUsage(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "No Authorization header",
		})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Invalid Bearer token",
		})
		return
	}
	tokenKey := parts[1]

	token, err := model.GetTokenByKey(strings.TrimPrefix(tokenKey, "sk-"), false)
	if err != nil {
		common.SysError("failed to get token by key: " + err.Error())
		common.ApiErrorI18n(c, i18n.MsgTokenGetInfoFailed)
		return
	}

	expiredAt := token.ExpiredTime
	if expiredAt == -1 {
		expiredAt = 0
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    true,
		"message": "ok",
		"data": gin.H{
			"object":               "token_usage",
			"name":                 token.Name,
			"total_granted":        token.RemainQuota + token.UsedQuota,
			"total_used":           token.UsedQuota,
			"total_available":      token.RemainQuota,
			"unlimited_quota":      token.UnlimitedQuota,
			"model_limits":         token.GetModelLimitsMap(),
			"model_limits_enabled": token.ModelLimitsEnabled,
			"expires_at":           expiredAt,
		},
	})
}

func AddToken(c *gin.Context) {
	token := model.Token{}
	err := c.ShouldBindJSON(&token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	userId := c.GetInt("id")
	var plan *model.SubscriptionPlan
	if token.PlanId > 0 {
		if !model.IsAdmin(userId) {
			common.ApiErrorMsg(c, "仅管理员可创建订阅型令牌")
			return
		}
		plan, err = model.GetSubscriptionPlanById(token.PlanId)
		if err != nil || plan == nil {
			common.ApiErrorMsg(c, "订阅套餐不存在")
			return
		}
	} else if !token.UnlimitedQuota {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	// 检查用户令牌数量是否已达上限
	maxTokens := operation_setting.GetMaxUserTokens()
	count, err := model.CountUserTokens(userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if int(count) >= maxTokens {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": fmt.Sprintf("已达到最大令牌数量限制 (%d)", maxTokens),
		})
		return
	}
	key, err := common.GenerateKey()
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
		common.SysLog("failed to generate token key: " + err.Error())
		return
	}
	cleanToken := model.Token{
		UserId:             userId,
		Name:               token.Name,
		Key:                key,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        token.ExpiredTime,
		RemainQuota:        token.RemainQuota,
		UnlimitedQuota:     token.UnlimitedQuota,
		ModelLimitsEnabled: token.ModelLimitsEnabled,
		ModelLimits:        token.ModelLimits,
		AllowIps:           token.AllowIps,
		Group:              token.Group,
		CrossGroupRetry:    token.CrossGroupRetry,
	}
	if plan != nil {
		if err := model.FillTokenPlanSnapshot(&cleanToken, plan); err != nil {
			common.ApiError(c, err)
			return
		}
		if strings.TrimSpace(cleanToken.Name) == "" {
			cleanToken.Name = plan.Title
		}
		cleanToken.AccessedTime = 0
	}
	err = cleanToken.Insert()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func AdminIssueSubscriptionTokens(c *gin.Context) {
	var req AdminIssueSubscriptionTokensRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.PlanId <= 0 {
		common.ApiErrorMsg(c, "订阅套餐不能为空")
		return
	}
	if len(req.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if req.TokenCount <= 0 {
		req.TokenCount = 1
	}
	if req.TokenCount > maxIssuedSubscriptionTokensPerRequest {
		common.ApiErrorMsg(c, fmt.Sprintf("单次最多创建 %d 个订阅型令牌", maxIssuedSubscriptionTokensPerRequest))
		return
	}
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil || plan == nil {
		common.ApiErrorMsg(c, "订阅套餐不存在")
		return
	}

	issuedTokens := make([]gin.H, 0, req.TokenCount)
	userId := c.GetInt("id")
	for i := 0; i < req.TokenCount; i++ {
		key, keyErr := common.GenerateKey()
		if keyErr != nil {
			common.ApiErrorI18n(c, i18n.MsgTokenGenerateFailed)
			return
		}
		cleanToken := model.Token{
			UserId:             userId,
			Name:               buildIssuedTokenName(req.Name, plan.Title, key, req.TokenCount > 1),
			Key:                key,
			CreatedTime:        common.GetTimestamp(),
			AccessedTime:       0,
			ModelLimitsEnabled: req.ModelLimitsEnabled && req.ModelLimits != "",
			ModelLimits:        req.ModelLimits,
			AllowIps:           req.AllowIps,
			Group:              req.Group,
			CrossGroupRetry:    req.CrossGroupRetry,
		}
		if err := model.FillTokenPlanSnapshot(&cleanToken, plan); err != nil {
			common.ApiError(c, err)
			return
		}
		if err := cleanToken.Insert(); err != nil {
			common.ApiError(c, err)
			return
		}
		issuedTokens = append(issuedTokens, gin.H{
			"id":      cleanToken.Id,
			"name":    cleanToken.Name,
			"key":     "sk-" + cleanToken.Key,
			"plan_id": cleanToken.PlanId,
		})
	}
	common.ApiSuccess(c, gin.H{
		"plan":   plan,
		"tokens": issuedTokens,
	})
}

func DeleteToken(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	userId := c.GetInt("id")
	err := model.DeleteTokenById(id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func UpdateToken(c *gin.Context) {
	userId := c.GetInt("id")
	statusOnly := c.Query("status_only")
	token := model.Token{}
	err := c.ShouldBindJSON(&token)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(token.Name) > 50 {
		common.ApiErrorI18n(c, i18n.MsgTokenNameTooLong)
		return
	}
	if !token.UnlimitedQuota && token.PlanId <= 0 {
		if token.RemainQuota < 0 {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaNegative)
			return
		}
		maxQuotaValue := int((1000000000 * common.QuotaPerUnit))
		if token.RemainQuota > maxQuotaValue {
			common.ApiErrorI18n(c, i18n.MsgTokenQuotaExceedMax, map[string]any{"Max": maxQuotaValue})
			return
		}
	}
	cleanToken, err := model.GetTokenByIds(token.Id, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if token.Status == common.TokenStatusEnabled {
		if cleanToken.Status == common.TokenStatusExpired && cleanToken.ExpiredTime <= common.GetTimestamp() && cleanToken.ExpiredTime != -1 {
			common.ApiErrorI18n(c, i18n.MsgTokenExpiredCannotEnable)
			return
		}
		if cleanToken.Status == common.TokenStatusExhausted && cleanToken.RemainQuota <= 0 && !cleanToken.UnlimitedQuota {
			common.ApiErrorI18n(c, i18n.MsgTokenExhaustedCannotEable)
			return
		}
	}
	if statusOnly != "" {
		cleanToken.Status = token.Status
	} else {
		cleanToken.Name = token.Name
		cleanToken.ModelLimitsEnabled = token.ModelLimitsEnabled
		cleanToken.ModelLimits = token.ModelLimits
		cleanToken.AllowIps = token.AllowIps
		cleanToken.Group = token.Group
		cleanToken.CrossGroupRetry = token.CrossGroupRetry
		if cleanToken.PlanId <= 0 {
			// If you add more fields, please also update token.Update()
			cleanToken.ExpiredTime = token.ExpiredTime
			cleanToken.RemainQuota = token.RemainQuota
			cleanToken.UnlimitedQuota = token.UnlimitedQuota
		}
	}
	err = cleanToken.Update()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    cleanToken,
	})
}

func RenewSubscriptionToken(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		common.ApiErrorMsg(c, "仅管理员可续费订阅型令牌")
		return
	}
	var req RenewSubscriptionTokenRequest
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			common.ApiError(c, err)
			return
		}
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	userId := c.GetInt("id")
	token, err := model.RenewSubscriptionTokenByID(id, userId, req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, token)
}

func DeleteSubscriptionTokenRenewal(c *gin.Context) {
	if c.GetInt("role") < common.RoleAdminUser {
		common.ApiErrorMsg(c, "仅管理员可删除订阅型令牌的待续费项")
		return
	}
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		common.ApiErrorMsg(c, "无效的ID")
		return
	}
	queueIndex, err := strconv.Atoi(c.Param("index"))
	if err != nil || queueIndex < 0 {
		common.ApiErrorMsg(c, "无效的待续费项")
		return
	}
	userId := c.GetInt("id")
	token, err := model.RemoveSubscriptionTokenRenewalByID(id, userId, queueIndex)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, token)
}

type TokenBatch struct {
	Ids []int `json:"ids"`
}

func DeleteTokenBatch(c *gin.Context) {
	tokenBatch := TokenBatch{}
	if err := c.ShouldBindJSON(&tokenBatch); err != nil || len(tokenBatch.Ids) == 0 {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	userId := c.GetInt("id")
	count, err := model.BatchDeleteTokens(tokenBatch.Ids, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    count,
	})
}
