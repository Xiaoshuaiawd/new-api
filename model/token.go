package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Token struct {
	Id                   int                     `json:"id"`
	UserId               int                     `json:"user_id" gorm:"index"`
	Key                  string                  `json:"key" gorm:"type:char(48);uniqueIndex"`
	Status               int                     `json:"status" gorm:"default:1"`
	Name                 string                  `json:"name" gorm:"index" `
	CreatedTime          int64                   `json:"created_time" gorm:"bigint"`
	AccessedTime         int64                   `json:"accessed_time" gorm:"bigint"`
	ExpiredTime          int64                   `json:"expired_time" gorm:"bigint;default:-1"` // -1 means never expired
	RemainQuota          int                     `json:"remain_quota" gorm:"default:0"`
	UnlimitedQuota       bool                    `json:"unlimited_quota"`
	ModelLimitsEnabled   bool                    `json:"model_limits_enabled"`
	ModelLimits          string                  `json:"model_limits" gorm:"type:text"`
	AllowIps             *string                 `json:"allow_ips" gorm:"default:''"`
	UsedQuota            int                     `json:"used_quota" gorm:"default:0"` // used quota
	Group                string                  `json:"group" gorm:"default:''"`
	CrossGroupRetry      bool                    `json:"cross_group_retry"` // 跨分组重试，仅auto分组有效
	PlanId               int                     `json:"plan_id" gorm:"type:int;default:0;index"`
	PlanTitle            string                  `json:"plan_title" gorm:"type:varchar(128);default:''"`
	ActivationTime       int64                   `json:"activation_time" gorm:"bigint;default:0"`
	NextResetTime        int64                   `json:"next_reset_time" gorm:"type:bigint;default:0;index"`
	LastResetTime        int64                   `json:"last_reset_time" gorm:"type:bigint;default:0"`
	PlanDurationUnit     string                  `json:"plan_duration_unit" gorm:"type:varchar(16);default:''"`
	PlanDurationValue    int                     `json:"plan_duration_value" gorm:"type:int;default:0"`
	PlanCustomSeconds    int64                   `json:"plan_custom_seconds" gorm:"type:bigint;default:0"`
	PlanAmountTotal      int                     `json:"plan_amount_total" gorm:"type:int;default:0"`
	PlanResetPeriod      string                  `json:"plan_reset_period" gorm:"type:varchar(16);default:''"`
	PlanResetSeconds     int64                   `json:"plan_reset_seconds" gorm:"type:bigint;default:0"`
	RenewalPlanQueue     string                  `json:"-" gorm:"type:text"`
	RenewalQueuedCount   int                     `json:"renewal_queued_count" gorm:"-"`
	NextRenewalPlanId    int                     `json:"next_renewal_plan_id" gorm:"-"`
	NextRenewalPlanTitle string                  `json:"next_renewal_plan_title" gorm:"-"`
	RenewalQueue         []TokenRenewalQueueItem `json:"renewal_queue" gorm:"-" redis:"-"`
	DeletedAt            gorm.DeletedAt          `gorm:"index"`
}

type TokenRenewalQueueItem struct {
	QueueIndex             int    `json:"queue_index"`
	PlanId                 int    `json:"plan_id"`
	PlanTitle              string `json:"plan_title"`
	PlanDurationUnit       string `json:"duration_unit"`
	PlanDurationValue      int    `json:"duration_value"`
	PlanCustomSeconds      int64  `json:"custom_seconds"`
	PlanAmountTotal        int    `json:"total_amount"`
	PlanResetPeriod        string `json:"quota_reset_period"`
	PlanResetCustomSeconds int64  `json:"quota_reset_custom_seconds"`
	QueuedAt               int64  `json:"queued_at"`
}

type tokenRenewalSnapshot struct {
	PlanId                 int    `json:"plan_id"`
	PlanTitle              string `json:"plan_title"`
	PlanDurationUnit       string `json:"duration_unit"`
	PlanDurationValue      int    `json:"duration_value"`
	PlanCustomSeconds      int64  `json:"custom_seconds"`
	PlanAmountTotal        int    `json:"total_amount"`
	PlanResetPeriod        string `json:"quota_reset_period"`
	PlanResetCustomSeconds int64  `json:"quota_reset_custom_seconds"`
	QueuedAt               int64  `json:"queued_at"`
}

func (token *Token) Clean() {
	token.Key = ""
}

func (token *Token) IsSubscriptionToken() bool {
	return token != nil && token.PlanId > 0
}

func (token *Token) AfterFind(tx *gorm.DB) error {
	token.syncRenewalQueueState(nil)
	return nil
}

func (token *Token) currentRenewalSnapshot(queuedAt int64) *tokenRenewalSnapshot {
	if token == nil || token.PlanId <= 0 {
		return nil
	}
	return &tokenRenewalSnapshot{
		PlanId:                 token.PlanId,
		PlanTitle:              token.PlanTitle,
		PlanDurationUnit:       token.PlanDurationUnit,
		PlanDurationValue:      token.PlanDurationValue,
		PlanCustomSeconds:      token.PlanCustomSeconds,
		PlanAmountTotal:        token.PlanAmountTotal,
		PlanResetPeriod:        token.PlanResetPeriod,
		PlanResetCustomSeconds: token.PlanResetSeconds,
		QueuedAt:               queuedAt,
	}
}

func buildRenewalSnapshotFromPlan(plan *SubscriptionPlan, queuedAt int64) (*tokenRenewalSnapshot, error) {
	tmp := &Token{}
	if err := FillTokenPlanSnapshot(tmp, plan); err != nil {
		return nil, err
	}
	return tmp.currentRenewalSnapshot(queuedAt), nil
}

func (snapshot *tokenRenewalSnapshot) toPlanSnapshot() *SubscriptionPlan {
	if snapshot == nil || snapshot.PlanId <= 0 {
		return nil
	}
	return &SubscriptionPlan{
		Id:                      snapshot.PlanId,
		Title:                   snapshot.PlanTitle,
		DurationUnit:            snapshot.PlanDurationUnit,
		DurationValue:           snapshot.PlanDurationValue,
		CustomSeconds:           snapshot.PlanCustomSeconds,
		TotalAmount:             int64(snapshot.PlanAmountTotal),
		QuotaResetPeriod:        snapshot.PlanResetPeriod,
		QuotaResetCustomSeconds: snapshot.PlanResetCustomSeconds,
	}
}

func (snapshot *tokenRenewalSnapshot) toQueueItem(queueIndex int) TokenRenewalQueueItem {
	if snapshot == nil {
		return TokenRenewalQueueItem{}
	}
	return TokenRenewalQueueItem{
		QueueIndex:             queueIndex,
		PlanId:                 snapshot.PlanId,
		PlanTitle:              snapshot.PlanTitle,
		PlanDurationUnit:       snapshot.PlanDurationUnit,
		PlanDurationValue:      snapshot.PlanDurationValue,
		PlanCustomSeconds:      snapshot.PlanCustomSeconds,
		PlanAmountTotal:        snapshot.PlanAmountTotal,
		PlanResetPeriod:        snapshot.PlanResetPeriod,
		PlanResetCustomSeconds: snapshot.PlanResetCustomSeconds,
		QueuedAt:               snapshot.QueuedAt,
	}
}

func applyRenewalSnapshotToToken(token *Token, snapshot tokenRenewalSnapshot) {
	if token == nil {
		return
	}
	token.PlanId = snapshot.PlanId
	token.PlanTitle = snapshot.PlanTitle
	token.PlanDurationUnit = snapshot.PlanDurationUnit
	token.PlanDurationValue = snapshot.PlanDurationValue
	token.PlanCustomSeconds = snapshot.PlanCustomSeconds
	token.PlanAmountTotal = snapshot.PlanAmountTotal
	token.PlanResetPeriod = snapshot.PlanResetPeriod
	token.PlanResetSeconds = snapshot.PlanResetCustomSeconds
}

func (token *Token) getRenewalPlanQueue() ([]tokenRenewalSnapshot, error) {
	if token == nil || strings.TrimSpace(token.RenewalPlanQueue) == "" {
		return nil, nil
	}
	var queue []tokenRenewalSnapshot
	if err := common.UnmarshalJsonStr(token.RenewalPlanQueue, &queue); err != nil {
		return nil, fmt.Errorf("invalid token renewal queue: %w", err)
	}
	return queue, nil
}

func (token *Token) setRenewalPlanQueue(queue []tokenRenewalSnapshot) error {
	if token == nil {
		return errors.New("token is nil")
	}
	if len(queue) == 0 {
		token.RenewalPlanQueue = ""
		token.syncRenewalQueueState(queue)
		return nil
	}
	data, err := common.Marshal(queue)
	if err != nil {
		return err
	}
	token.RenewalPlanQueue = string(data)
	token.syncRenewalQueueState(queue)
	return nil
}

func (token *Token) syncRenewalQueueState(queue []tokenRenewalSnapshot) {
	if token == nil {
		return
	}
	token.RenewalQueuedCount = 0
	token.NextRenewalPlanId = 0
	token.NextRenewalPlanTitle = ""
	token.RenewalQueue = nil
	if queue == nil {
		var err error
		queue, err = token.getRenewalPlanQueue()
		if err != nil {
			common.SysLog("failed to parse token renewal queue: " + err.Error())
			return
		}
	}
	if len(queue) == 0 {
		return
	}
	token.RenewalQueue = make([]TokenRenewalQueueItem, 0, len(queue))
	for idx := range queue {
		token.RenewalQueue = append(token.RenewalQueue, queue[idx].toQueueItem(idx))
	}
	token.RenewalQueuedCount = len(queue)
	token.NextRenewalPlanId = queue[0].PlanId
	token.NextRenewalPlanTitle = queue[0].PlanTitle
}

func FillTokenPlanSnapshot(token *Token, plan *SubscriptionPlan) error {
	if token == nil {
		return errors.New("token is nil")
	}
	if plan == nil || plan.Id <= 0 {
		return errors.New("invalid plan")
	}
	maxQuotaValue := int64(1000000000 * common.QuotaPerUnit)
	if plan.TotalAmount < 0 {
		return errors.New("plan total amount cannot be negative")
	}
	if plan.TotalAmount > maxQuotaValue {
		return fmt.Errorf("plan total amount exceeds max token quota: %d", maxQuotaValue)
	}
	token.PlanId = plan.Id
	token.PlanTitle = plan.Title
	token.PlanDurationUnit = plan.DurationUnit
	token.PlanDurationValue = plan.DurationValue
	token.PlanCustomSeconds = plan.CustomSeconds
	token.PlanAmountTotal = int(plan.TotalAmount)
	token.PlanResetPeriod = NormalizeResetPeriod(plan.QuotaResetPeriod)
	token.PlanResetSeconds = plan.QuotaResetCustomSeconds
	token.ActivationTime = 0
	token.LastResetTime = 0
	token.NextResetTime = 0
	token.ExpiredTime = 0
	token.UnlimitedQuota = plan.TotalAmount == 0
	token.RemainQuota = int(plan.TotalAmount)
	token.UsedQuota = 0
	return nil
}

func (token *Token) toPlanSnapshot() *SubscriptionPlan {
	if token == nil || token.PlanId <= 0 {
		return nil
	}
	return &SubscriptionPlan{
		Id:                      token.PlanId,
		Title:                   token.PlanTitle,
		DurationUnit:            token.PlanDurationUnit,
		DurationValue:           token.PlanDurationValue,
		CustomSeconds:           token.PlanCustomSeconds,
		TotalAmount:             int64(token.PlanAmountTotal),
		QuotaResetPeriod:        token.PlanResetPeriod,
		QuotaResetCustomSeconds: token.PlanResetSeconds,
	}
}

func applySubscriptionTokenRuntime(token *Token, plan *SubscriptionPlan, startUnix int64, accessUnix int64) error {
	if token == nil {
		return errors.New("token is nil")
	}
	if plan == nil {
		return errors.New("plan is nil")
	}
	if startUnix <= 0 {
		return errors.New("invalid start time")
	}
	start := time.Unix(startUnix, 0)
	endUnix, err := calcPlanEndTime(start, plan)
	if err != nil {
		return err
	}
	nextReset := calcNextResetTime(start, plan, endUnix)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = startUnix
	}
	token.Status = common.TokenStatusEnabled
	token.ActivationTime = startUnix
	token.ExpiredTime = endUnix
	token.AccessedTime = accessUnix
	token.LastResetTime = lastReset
	token.NextResetTime = nextReset
	token.UnlimitedQuota = plan.TotalAmount == 0
	token.RemainQuota = int(plan.TotalAmount)
	token.UsedQuota = 0
	return nil
}

func activateSubscriptionTokenTx(tx *gorm.DB, token *Token, now int64) error {
	if tx == nil || token == nil || !token.IsSubscriptionToken() || token.ActivationTime > 0 {
		return nil
	}
	plan := token.toPlanSnapshot()
	if plan == nil {
		return errors.New("subscription plan snapshot is missing")
	}
	if err := applySubscriptionTokenRuntime(token, plan, now, now); err != nil {
		return err
	}
	return tx.Save(token).Error
}

func maybeActivateQueuedRenewalTokensTx(tx *gorm.DB, token *Token, now int64) error {
	if tx == nil || token == nil || !token.IsSubscriptionToken() || token.ActivationTime == 0 {
		return nil
	}
	queue, err := token.getRenewalPlanQueue()
	if err != nil {
		return err
	}
	if len(queue) == 0 {
		token.syncRenewalQueueState(queue)
		return nil
	}
	updated := false
	for token.ExpiredTime > 0 && token.ExpiredTime <= now && len(queue) > 0 {
		nextSnapshot := queue[0]
		startUnix := token.ExpiredTime
		if startUnix < nextSnapshot.QueuedAt {
			startUnix = nextSnapshot.QueuedAt
		}
		if startUnix > now {
			break
		}
		plan := nextSnapshot.toPlanSnapshot()
		if plan == nil {
			return errors.New("queued renewal plan snapshot is missing")
		}
		applyRenewalSnapshotToToken(token, nextSnapshot)
		if err := applySubscriptionTokenRuntime(token, plan, startUnix, now); err != nil {
			return err
		}
		queue = queue[1:]
		updated = true
	}
	if err := token.setRenewalPlanQueue(queue); err != nil {
		return err
	}
	if !updated {
		return nil
	}
	return tx.Save(token).Error
}

func maybeResetSubscriptionTokenTx(tx *gorm.DB, token *Token, now int64) error {
	if tx == nil || token == nil || !token.IsSubscriptionToken() || token.ActivationTime == 0 {
		return nil
	}
	if token.NextResetTime <= 0 || token.NextResetTime > now {
		return nil
	}
	plan := token.toPlanSnapshot()
	if plan == nil || NormalizeResetPeriod(plan.QuotaResetPeriod) == SubscriptionResetNever {
		return nil
	}
	baseUnix := token.LastResetTime
	if baseUnix <= 0 {
		baseUnix = token.ActivationTime
	}
	base := time.Unix(baseUnix, 0)
	next := calcNextResetTime(base, plan, token.ExpiredTime)
	advanced := false
	for next > 0 && next <= now {
		advanced = true
		base = time.Unix(next, 0)
		next = calcNextResetTime(base, plan, token.ExpiredTime)
	}
	if !advanced {
		if token.NextResetTime == 0 && next > 0 {
			token.LastResetTime = base.Unix()
			token.NextResetTime = next
			return tx.Save(token).Error
		}
		return nil
	}
	token.LastResetTime = base.Unix()
	token.NextResetTime = next
	token.UsedQuota = 0
	if !token.UnlimitedQuota && token.PlanAmountTotal > 0 {
		token.RemainQuota = token.PlanAmountTotal
	}
	if token.Status == common.TokenStatusExhausted {
		token.Status = common.TokenStatusEnabled
	}
	return tx.Save(token).Error
}

func ensureSubscriptionTokenReady(token *Token) (*Token, error) {
	if token == nil || !token.IsSubscriptionToken() {
		return token, nil
	}
	now := GetDBTimestamp()
	needsActivation := token.ActivationTime == 0
	needsReset := token.ActivationTime > 0 && token.NextResetTime > 0 && token.NextResetTime <= now
	needsRenewalSwitch := token.ActivationTime > 0 && token.ExpiredTime > 0 && token.ExpiredTime <= now && strings.TrimSpace(token.RenewalPlanQueue) != ""
	if !needsActivation && !needsReset && !needsRenewalSwitch {
		return token, nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var locked Token
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("id = ?", token.Id).First(&locked).Error; err != nil {
			return err
		}
		if !locked.IsSubscriptionToken() {
			*token = locked
			return nil
		}
		if locked.ActivationTime == 0 {
			if err := activateSubscriptionTokenTx(tx, &locked, now); err != nil {
				return err
			}
		}
		if err := maybeActivateQueuedRenewalTokensTx(tx, &locked, now); err != nil {
			return err
		}
		if err := maybeResetSubscriptionTokenTx(tx, &locked, now); err != nil {
			return err
		}
		locked.syncRenewalQueueState(nil)
		*token = locked
		return nil
	})
	if err != nil {
		return nil, err
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			if err := cacheSetToken(*token); err != nil {
				common.SysLog("failed to refresh subscription token cache: " + err.Error())
			}
		})
	}
	return token, nil
}

func ResetDueSubscriptionTokens(limit int) (int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var tokens []Token
	if err := DB.Where("plan_id > 0 AND activation_time > 0 AND ((next_reset_time > 0 AND next_reset_time <= ?) OR (expired_time > 0 AND expired_time <= ? AND renewal_plan_queue <> ''))", now, now).
		Order("expired_time asc").
		Order("next_reset_time asc").
		Limit(limit).
		Find(&tokens).Error; err != nil {
		return 0, err
	}
	if len(tokens) == 0 {
		return 0, nil
	}
	resetCount := 0
	for _, token := range tokens {
		tokenCopy := token
		updated := false
		err := DB.Transaction(func(tx *gorm.DB) error {
			var locked Token
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("id = ? AND plan_id > 0 AND activation_time > 0 AND ((next_reset_time > 0 AND next_reset_time <= ?) OR (expired_time > 0 AND expired_time <= ? AND renewal_plan_queue <> ''))", tokenCopy.Id, now, now).
				First(&locked).Error; err != nil {
				return nil
			}
			if err := maybeActivateQueuedRenewalTokensTx(tx, &locked, now); err != nil {
				return err
			}
			if err := maybeResetSubscriptionTokenTx(tx, &locked, now); err != nil {
				return err
			}
			locked.syncRenewalQueueState(nil)
			tokenCopy = locked
			updated = true
			resetCount++
			return nil
		})
		if err != nil {
			return resetCount, err
		}
		if updated && common.RedisEnabled {
			cacheToken := tokenCopy
			gopool.Go(func() {
				if err := cacheSetToken(cacheToken); err != nil {
					common.SysLog("failed to refresh due subscription token cache: " + err.Error())
				}
			})
		}
	}
	return resetCount, nil
}

func validateSubscriptionTokenRenewal(token *Token, _ int64) error {
	if token == nil {
		return errors.New("token is nil")
	}
	if !token.IsSubscriptionToken() {
		return errors.New("该令牌不是订阅型令牌")
	}
	return nil
}

func prepareSubscriptionTokenRenewalPlanTx(tx *gorm.DB, token *Token, planId int, queuedAt int64) (*tokenRenewalSnapshot, error) {
	if token == nil {
		return nil, errors.New("token is nil")
	}
	if planId > 0 {
		plan, err := getSubscriptionPlanByIdTx(tx, planId)
		if err != nil {
			return nil, err
		}
		return buildRenewalSnapshotFromPlan(plan, queuedAt)
	}
	snapshot := token.currentRenewalSnapshot(queuedAt)
	if snapshot == nil {
		return nil, errors.New("subscription plan snapshot is missing")
	}
	return snapshot, nil
}

func RenewSubscriptionTokenByID(id int, userId int, planId int) (*Token, error) {
	if id <= 0 || userId <= 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	var renewed Token
	now := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var locked Token
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND user_id = ?", id, userId).
			First(&locked).Error; err != nil {
			return err
		}
		if err := validateSubscriptionTokenRenewal(&locked, now); err != nil {
			return err
		}
		snapshot, err := prepareSubscriptionTokenRenewalPlanTx(tx, &locked, planId, now)
		if err != nil {
			return err
		}
		queue, err := locked.getRenewalPlanQueue()
		if err != nil {
			return err
		}
		queue = append(queue, *snapshot)
		if err := locked.setRenewalPlanQueue(queue); err != nil {
			return err
		}
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if err := maybeActivateQueuedRenewalTokensTx(tx, &locked, now); err != nil {
			return err
		}
		if err := maybeResetSubscriptionTokenTx(tx, &locked, now); err != nil {
			return err
		}
		locked.syncRenewalQueueState(nil)
		renewed = locked
		return nil
	})
	if err != nil {
		return nil, err
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			if err := cacheSetToken(renewed); err != nil {
				common.SysLog("failed to refresh renewed subscription token cache: " + err.Error())
			}
		})
	}
	return &renewed, nil
}

func RemoveSubscriptionTokenRenewalByID(id int, userId int, queueIndex int) (*Token, error) {
	if id <= 0 || userId <= 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	if queueIndex < 0 {
		return nil, errors.New("无效的待续费项")
	}
	var updated Token
	now := GetDBTimestamp()
	err := DB.Transaction(func(tx *gorm.DB) error {
		var locked Token
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND user_id = ?", id, userId).
			First(&locked).Error; err != nil {
			return err
		}
		if !locked.IsSubscriptionToken() {
			return errors.New("该令牌不是订阅型令牌")
		}
		queue, err := locked.getRenewalPlanQueue()
		if err != nil {
			return err
		}
		if len(queue) == 0 {
			return errors.New("当前没有待续费套餐")
		}
		if queueIndex >= len(queue) {
			return errors.New("待续费项不存在")
		}
		queue = append(queue[:queueIndex], queue[queueIndex+1:]...)
		if err := locked.setRenewalPlanQueue(queue); err != nil {
			return err
		}
		if err := tx.Save(&locked).Error; err != nil {
			return err
		}
		if err := maybeActivateQueuedRenewalTokensTx(tx, &locked, now); err != nil {
			return err
		}
		if err := maybeResetSubscriptionTokenTx(tx, &locked, now); err != nil {
			return err
		}
		locked.syncRenewalQueueState(nil)
		updated = locked
		return nil
	})
	if err != nil {
		return nil, err
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			if err := cacheSetToken(updated); err != nil {
				common.SysLog("failed to refresh token renewal queue cache: " + err.Error())
			}
		})
	}
	return &updated, nil
}

func (token *Token) GetIpLimits() []string {
	// delete empty spaces
	//split with \n
	ipLimits := make([]string, 0)
	if token.AllowIps == nil {
		return ipLimits
	}
	cleanIps := strings.ReplaceAll(*token.AllowIps, " ", "")
	if cleanIps == "" {
		return ipLimits
	}
	ips := strings.Split(cleanIps, "\n")
	for _, ip := range ips {
		ip = strings.TrimSpace(ip)
		ip = strings.ReplaceAll(ip, ",", "")
		if ip != "" {
			ipLimits = append(ipLimits, ip)
		}
	}
	return ipLimits
}

func GetAllUserTokens(userId int, startIdx int, num int) ([]*Token, error) {
	var tokens []*Token
	var err error
	err = DB.Where("user_id = ?", userId).Order("id desc").Limit(num).Offset(startIdx).Find(&tokens).Error
	return tokens, err
}

// sanitizeLikePattern 校验并清洗用户输入的 LIKE 搜索模式。
// 规则：
//  1. 转义 ! 和 _（使用 ! 作为 ESCAPE 字符，兼容 MySQL/PostgreSQL/SQLite）
//  2. 连续的 % 合并为单个 %
//  3. 最多允许 2 个 %
//  4. 含 % 时（模糊搜索），去掉 % 后关键词长度必须 >= 2
//  5. 不含 % 时按精确匹配
func sanitizeLikePattern(input string) (string, error) {
	// 1. 先转义 ESCAPE 字符 ! 自身，再转义 _
	//    使用 ! 而非 \ 作为 ESCAPE 字符，避免 MySQL 中反斜杠的字符串转义问题
	input = strings.ReplaceAll(input, "!", "!!")
	input = strings.ReplaceAll(input, `_`, `!_`)

	// 2. 连续的 % 直接拒绝
	if strings.Contains(input, "%%") {
		return "", errors.New("搜索模式中不允许包含连续的 % 通配符")
	}

	// 3. 统计 % 数量，不得超过 2
	count := strings.Count(input, "%")
	if count > 2 {
		return "", errors.New("搜索模式中最多允许包含 2 个 % 通配符")
	}

	// 4. 含 % 时，去掉 % 后关键词长度必须 >= 2
	if count > 0 {
		stripped := strings.ReplaceAll(input, "%", "")
		if len(stripped) < 2 {
			return "", errors.New("使用模糊搜索时，关键词长度至少为 2 个字符")
		}
		return input, nil
	}

	// 5. 无 % 时，精确全匹配
	return input, nil
}

const searchHardLimit = 100

func SearchUserTokens(userId int, keyword string, token string, offset int, limit int) (tokens []*Token, total int64, err error) {
	// model 层强制截断
	if limit <= 0 || limit > searchHardLimit {
		limit = searchHardLimit
	}
	if offset < 0 {
		offset = 0
	}

	if token != "" {
		token = strings.TrimPrefix(token, "sk-")
	}

	// 超量用户（令牌数超过上限）只允许精确搜索，禁止模糊搜索
	maxTokens := operation_setting.GetMaxUserTokens()
	hasFuzzy := strings.Contains(keyword, "%") || strings.Contains(token, "%")
	if hasFuzzy {
		count, err := CountUserTokens(userId)
		if err != nil {
			common.SysLog("failed to count user tokens: " + err.Error())
			return nil, 0, errors.New("获取令牌数量失败")
		}
		if int(count) > maxTokens {
			return nil, 0, errors.New("令牌数量超过上限，仅允许精确搜索，请勿使用 % 通配符")
		}
	}

	baseQuery := DB.Model(&Token{}).Where("user_id = ?", userId)

	// 非空才加 LIKE 条件，空则跳过（不过滤该字段）
	if keyword != "" {
		keywordPattern, err := sanitizeLikePattern(keyword)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where("name LIKE ? ESCAPE '!'", keywordPattern)
	}
	if token != "" {
		tokenPattern, err := sanitizeLikePattern(token)
		if err != nil {
			return nil, 0, err
		}
		baseQuery = baseQuery.Where(commonKeyCol+" LIKE ? ESCAPE '!'", tokenPattern)
	}

	// 先查匹配总数（用于分页，受 maxTokens 上限保护，避免全表 COUNT）
	err = baseQuery.Limit(maxTokens).Count(&total).Error
	if err != nil {
		common.SysError("failed to count search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}

	// 再分页查数据
	err = baseQuery.Order("id desc").Offset(offset).Limit(limit).Find(&tokens).Error
	if err != nil {
		common.SysError("failed to search tokens: " + err.Error())
		return nil, 0, errors.New("搜索令牌失败")
	}
	return tokens, total, nil
}

func ValidateUserToken(key string) (token *Token, err error) {
	if key == "" {
		return nil, errors.New("未提供令牌")
	}
	token, err = GetTokenByKey(key, false)
	if err == nil {
		token, err = ensureSubscriptionTokenReady(token)
		if err != nil {
			common.SysLog("failed to prepare subscription token: " + err.Error())
			return nil, errors.New("令牌激活失败，请联系管理员")
		}
		if token.Status == common.TokenStatusExhausted {
			keyPrefix := key[:3]
			keySuffix := key[len(key)-3:]
			return token, errors.New("该令牌额度已用尽 TokenStatusExhausted[sk-" + keyPrefix + "***" + keySuffix + "]")
		} else if token.Status == common.TokenStatusExpired {
			return token, errors.New("该令牌已过期")
		}
		if token.Status != common.TokenStatusEnabled {
			return token, errors.New("该令牌状态不可用")
		}
		if token.ExpiredTime != -1 && token.ExpiredTime < common.GetTimestamp() {
			if !common.RedisEnabled {
				token.Status = common.TokenStatusExpired
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			return token, errors.New("该令牌已过期")
		}
		if !token.UnlimitedQuota && token.RemainQuota <= 0 {
			if !common.RedisEnabled {
				// in this case, we can make sure the token is exhausted
				token.Status = common.TokenStatusExhausted
				err := token.SelectUpdate()
				if err != nil {
					common.SysLog("failed to update token status" + err.Error())
				}
			}
			keyPrefix := key[:3]
			keySuffix := key[len(key)-3:]
			return token, errors.New(fmt.Sprintf("[sk-%s***%s] 该令牌额度已用尽 !token.UnlimitedQuota && token.RemainQuota = %d", keyPrefix, keySuffix, token.RemainQuota))
		}
		return token, nil
	}
	common.SysLog("ValidateUserToken: failed to get token: " + err.Error())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("无效的令牌")
	} else {
		return nil, errors.New("无效的令牌，数据库查询出错，请联系管理员")
	}
}

func GetTokenByIds(id int, userId int) (*Token, error) {
	if id == 0 || userId == 0 {
		return nil, errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	var err error = nil
	err = DB.First(&token, "id = ? and user_id = ?", id, userId).Error
	return &token, err
}

func GetTokenById(id int) (*Token, error) {
	if id == 0 {
		return nil, errors.New("id 为空！")
	}
	token := Token{Id: id}
	var err error = nil
	err = DB.First(&token, "id = ?", id).Error
	if shouldUpdateRedis(true, err) {
		gopool.Go(func() {
			if err := cacheSetToken(token); err != nil {
				common.SysLog("failed to update user status cache: " + err.Error())
			}
		})
	}
	return &token, err
}

func GetTokenByKey(key string, fromDB bool) (token *Token, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) && token != nil {
			gopool.Go(func() {
				if err := cacheSetToken(*token); err != nil {
					common.SysLog("failed to update user status cache: " + err.Error())
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		// Try Redis first
		token, err := cacheGetTokenByKey(key)
		if err == nil {
			token.syncRenewalQueueState(nil)
			return token, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Where(commonKeyCol+" = ?", key).First(&token).Error
	if err == nil && token != nil {
		token.syncRenewalQueueState(nil)
	}
	return token, err
}

func (token *Token) Insert() error {
	var err error
	err = DB.Create(token).Error
	return err
}

// Update Make sure your token's fields is completed, because this will update non-zero values
func (token *Token) Update() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Model(token).Select("name", "status", "expired_time", "remain_quota", "unlimited_quota",
		"model_limits_enabled", "model_limits", "allow_ips", "group", "cross_group_retry").Updates(token).Error
	return err
}

func (token *Token) SelectUpdate() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheSetToken(*token)
				if err != nil {
					common.SysLog("failed to update token cache: " + err.Error())
				}
			})
		}
	}()
	// This can update zero values
	return DB.Model(token).Select("accessed_time", "status").Updates(token).Error
}

func (token *Token) Delete() (err error) {
	defer func() {
		if shouldUpdateRedis(true, err) {
			gopool.Go(func() {
				err := cacheDeleteToken(token.Key)
				if err != nil {
					common.SysLog("failed to delete token cache: " + err.Error())
				}
			})
		}
	}()
	err = DB.Delete(token).Error
	return err
}

func (token *Token) IsModelLimitsEnabled() bool {
	return token.ModelLimitsEnabled
}

func (token *Token) GetModelLimits() []string {
	if token.ModelLimits == "" {
		return []string{}
	}
	return strings.Split(token.ModelLimits, ",")
}

func (token *Token) GetModelLimitsMap() map[string]bool {
	limits := token.GetModelLimits()
	limitsMap := make(map[string]bool)
	for _, limit := range limits {
		limitsMap[limit] = true
	}
	return limitsMap
}

func DisableModelLimits(tokenId int) error {
	token, err := GetTokenById(tokenId)
	if err != nil {
		return err
	}
	token.ModelLimitsEnabled = false
	token.ModelLimits = ""
	return token.Update()
}

func DeleteTokenById(id int, userId int) (err error) {
	// Why we need userId here? In case user want to delete other's token.
	if id == 0 || userId == 0 {
		return errors.New("id 或 userId 为空！")
	}
	token := Token{Id: id, UserId: userId}
	err = DB.Where(token).First(&token).Error
	if err != nil {
		return err
	}
	return token.Delete()
}

func IncreaseTokenQuota(tokenId int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheIncrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to increase token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, tokenId, quota)
		return nil
	}
	return increaseTokenQuota(tokenId, quota)
}

func increaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("CASE WHEN unlimited_quota THEN remain_quota ELSE remain_quota + ? END", quota),
			"used_quota":    gorm.Expr("used_quota - ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

func DecreaseTokenQuota(id int, key string, quota int) (err error) {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if common.RedisEnabled {
		gopool.Go(func() {
			err := cacheDecrTokenQuota(key, int64(quota))
			if err != nil {
				common.SysLog("failed to decrease token quota: " + err.Error())
			}
		})
	}
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeTokenQuota, id, -quota)
		return nil
	}
	return decreaseTokenQuota(id, quota)
}

func decreaseTokenQuota(id int, quota int) (err error) {
	err = DB.Model(&Token{}).Where("id = ?", id).Updates(
		map[string]interface{}{
			"remain_quota":  gorm.Expr("CASE WHEN unlimited_quota THEN remain_quota ELSE remain_quota - ? END", quota),
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"accessed_time": common.GetTimestamp(),
		},
	).Error
	return err
}

// CountUserTokens returns total number of tokens for the given user, used for pagination
func CountUserTokens(userId int) (int64, error) {
	var total int64
	err := DB.Model(&Token{}).Where("user_id = ?", userId).Count(&total).Error
	return total, err
}

// BatchDeleteTokens 删除指定用户的一组令牌，返回成功删除数量
func BatchDeleteTokens(ids []int, userId int) (int, error) {
	if len(ids) == 0 {
		return 0, errors.New("ids 不能为空！")
	}

	tx := DB.Begin()

	var tokens []Token
	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Find(&tokens).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Where("user_id = ? AND id IN (?)", userId, ids).Delete(&Token{}).Error; err != nil {
		tx.Rollback()
		return 0, err
	}

	if err := tx.Commit().Error; err != nil {
		return 0, err
	}

	if common.RedisEnabled {
		gopool.Go(func() {
			for _, t := range tokens {
				_ = cacheDeleteToken(t.Key)
			}
		})
	}

	return len(tokens), nil
}
