package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"one-api/common"
	"one-api/logger"
	"time"

	"gorm.io/gorm"
)

// SubscriptionPackage 套餐定义表
type SubscriptionPackage struct {
	Id                 int            `json:"id" gorm:"primaryKey"`
	Name               string         `json:"name" gorm:"type:varchar(100);not null;index" validate:"required,max=100"`
	Description        string         `json:"description" gorm:"type:text"`
	PermanentQuota     int64          `json:"permanent_quota" gorm:"type:bigint;default:0" validate:"min=0"`      // 永久额度
	MonthlyQuota       int64          `json:"monthly_quota" gorm:"type:bigint;default:0" validate:"min=0"`        // 每月额度
	DailyQuota         int64          `json:"daily_quota" gorm:"type:bigint;default:0" validate:"min=0"`          // 每日额度
	Price              float64        `json:"price" gorm:"type:decimal(10,2);default:0" validate:"min=0"`         // 价格
	Currency           string         `json:"currency" gorm:"type:varchar(10);default:'CNY'" validate:"max=10"`   // 货币类型
	Duration           int            `json:"duration" gorm:"type:int;default:30" validate:"min=1"`               // 套餐持续天数
	Status             int            `json:"status" gorm:"type:int;default:1" validate:"oneof=0 1"`              // 状态: 0-禁用, 1-启用
	GroupLimit         string         `json:"group_limit" gorm:"type:varchar(500)"`                               // 限制的用户组，JSON格式
	ModelLimit         string         `json:"model_limit" gorm:"type:text"`                                       // 限制的模型，JSON格式
	MaxUsersPerPackage int            `json:"max_users_per_package" gorm:"type:int;default:0"`                    // 每个套餐最大用户数，0表示无限制
	Features           string         `json:"features" gorm:"type:text"`                                          // 套餐特性，JSON格式
	SortOrder          int            `json:"sort_order" gorm:"type:int;default:0"`                               // 排序
	CreatedTime        int64          `json:"created_time" gorm:"type:bigint"`                                    // 创建时间
	UpdatedTime        int64          `json:"updated_time" gorm:"type:bigint"`                                    // 更新时间
	CreatedBy          int            `json:"created_by" gorm:"type:int;index"`                                   // 创建者ID
	DeletedAt          gorm.DeletedAt `json:"-" gorm:"index"`                                                     // 软删除
}

// UserSubscription 用户订阅表
type UserSubscription struct {
	Id                  int            `json:"id" gorm:"primaryKey"`
	UserId              int            `json:"user_id" gorm:"type:int;not null;index" validate:"required"`
	PackageId           int            `json:"package_id" gorm:"type:int;not null;index" validate:"required"`
	Status              int            `json:"status" gorm:"type:int;default:1" validate:"oneof=0 1 2 3"`      // 状态: 0-已取消, 1-激活, 2-暂停, 3-已过期
	StartTime           int64          `json:"start_time" gorm:"type:bigint;not null"`                         // 开始时间
	EndTime             int64          `json:"end_time" gorm:"type:bigint;not null;index"`                     // 结束时间
	PermanentQuotaUsed  int64          `json:"permanent_quota_used" gorm:"type:bigint;default:0"`              // 已使用永久额度
	MonthlyQuotaUsed    int64          `json:"monthly_quota_used" gorm:"type:bigint;default:0"`                // 已使用月额度
	DailyQuotaUsed      int64          `json:"daily_quota_used" gorm:"type:bigint;default:0"`                  // 已使用日额度
	LastMonthlyReset    int64          `json:"last_monthly_reset" gorm:"type:bigint;default:0"`                // 上次月重置时间
	LastDailyReset      int64          `json:"last_daily_reset" gorm:"type:bigint;default:0"`                  // 上次日重置时间
	TotalUsage          int64          `json:"total_usage" gorm:"type:bigint;default:0"`                       // 总使用量
	CreatedTime         int64          `json:"created_time" gorm:"type:bigint"`                                // 创建时间
	UpdatedTime         int64          `json:"updated_time" gorm:"type:bigint"`                                // 更新时间
	DeletedAt           gorm.DeletedAt `json:"-" gorm:"index"`                                                 // 软删除

	// 关联关系
	Package *SubscriptionPackage `json:"package,omitempty" gorm:"foreignKey:PackageId"`
	User    *User                `json:"user,omitempty" gorm:"foreignKey:UserId"`
}

// QuotaResetLog 额度重置日志表
type QuotaResetLog struct {
	Id            int            `json:"id" gorm:"primaryKey"`
	UserId        int            `json:"user_id" gorm:"type:int;not null;index"`
	SubscriptionId int           `json:"subscription_id" gorm:"type:int;not null;index"`
	ResetType     string         `json:"reset_type" gorm:"type:varchar(20);not null"` // daily, monthly
	ResetTime     int64          `json:"reset_time" gorm:"type:bigint;not null;index"`
	PreviousUsage int64          `json:"previous_usage" gorm:"type:bigint;default:0"`
	NewQuota      int64          `json:"new_quota" gorm:"type:bigint;default:0"`
	CreatedTime   int64          `json:"created_time" gorm:"type:bigint"`
	DeletedAt     gorm.DeletedAt `json:"-" gorm:"index"`
}

// GetGroupLimitList 获取组限制列表
func (sp *SubscriptionPackage) GetGroupLimitList() []string {
	if sp.GroupLimit == "" {
		return []string{}
	}
	var groups []string
	err := json.Unmarshal([]byte(sp.GroupLimit), &groups)
	if err != nil {
		common.SysLog("解析套餐组限制失败: " + err.Error())
		return []string{}
	}
	return groups
}

// SetGroupLimitList 设置组限制列表
func (sp *SubscriptionPackage) SetGroupLimitList(groups []string) {
	if len(groups) == 0 {
		sp.GroupLimit = ""
		return
	}
	data, err := json.Marshal(groups)
	if err != nil {
		common.SysLog("序列化套餐组限制失败: " + err.Error())
		return
	}
	sp.GroupLimit = string(data)
}

// GetModelLimitList 获取模型限制列表
func (sp *SubscriptionPackage) GetModelLimitList() []string {
	if sp.ModelLimit == "" {
		return []string{}
	}
	var models []string
	err := json.Unmarshal([]byte(sp.ModelLimit), &models)
	if err != nil {
		common.SysLog("解析套餐模型限制失败: " + err.Error())
		return []string{}
	}
	return models
}

// SetModelLimitList 设置模型限制列表
func (sp *SubscriptionPackage) SetModelLimitList(models []string) {
	if len(models) == 0 {
		sp.ModelLimit = ""
		return
	}
	data, err := json.Marshal(models)
	if err != nil {
		common.SysLog("序列化套餐模型限制失败: " + err.Error())
		return
	}
	sp.ModelLimit = string(data)
}

// GetFeatures 获取套餐特性
func (sp *SubscriptionPackage) GetFeatures() map[string]interface{} {
	if sp.Features == "" {
		return map[string]interface{}{}
	}
	var features map[string]interface{}
	err := json.Unmarshal([]byte(sp.Features), &features)
	if err != nil {
		common.SysLog("解析套餐特性失败: " + err.Error())
		return map[string]interface{}{}
	}
	return features
}

// SetFeatures 设置套餐特性
func (sp *SubscriptionPackage) SetFeatures(features map[string]interface{}) {
	if len(features) == 0 {
		sp.Features = ""
		return
	}
	data, err := json.Marshal(features)
	if err != nil {
		common.SysLog("序列化套餐特性失败: " + err.Error())
		return
	}
	sp.Features = string(data)
}

// Insert 创建套餐
func (sp *SubscriptionPackage) Insert() error {
	now := common.GetTimestamp()
	sp.CreatedTime = now
	sp.UpdatedTime = now
	return DB.Create(sp).Error
}

// Update 更新套餐
func (sp *SubscriptionPackage) Update() error {
	sp.UpdatedTime = common.GetTimestamp()
	return DB.Save(sp).Error
}

// Delete 删除套餐
func (sp *SubscriptionPackage) Delete() error {
	return DB.Delete(sp).Error
}

// GetAllSubscriptionPackages 获取所有套餐
func GetAllSubscriptionPackages(status int) ([]*SubscriptionPackage, error) {
	var packages []*SubscriptionPackage
	query := DB.Model(&SubscriptionPackage{})

	if status >= 0 {
		query = query.Where("status = ?", status)
	}

	err := query.Order("sort_order asc, id desc").Find(&packages).Error
	return packages, err
}

// GetSubscriptionPackageById 根据ID获取套餐
func GetSubscriptionPackageById(id int) (*SubscriptionPackage, error) {
	if id == 0 {
		return nil, errors.New("套餐ID不能为空")
	}
	var pkg SubscriptionPackage
	err := DB.First(&pkg, id).Error
	return &pkg, err
}

// GetActiveSubscriptionPackages 获取启用的套餐
func GetActiveSubscriptionPackages() ([]*SubscriptionPackage, error) {
	return GetAllSubscriptionPackages(1)
}

// Insert 创建用户订阅
func (us *UserSubscription) Insert() error {
	now := common.GetTimestamp()
	us.CreatedTime = now
	us.UpdatedTime = now
	return DB.Create(us).Error
}

// Update 更新用户订阅
func (us *UserSubscription) Update() error {
	us.UpdatedTime = common.GetTimestamp()
	return DB.Save(us).Error
}

// Delete 删除用户订阅
func (us *UserSubscription) Delete() error {
	return DB.Delete(us).Error
}

// GetUserSubscriptions 获取用户订阅列表
func GetUserSubscriptions(userId int, status int) ([]*UserSubscription, error) {
	var subscriptions []*UserSubscription
	query := DB.Model(&UserSubscription{}).Preload("Package").Where("user_id = ?", userId)

	if status >= 0 {
		query = query.Where("status = ?", status)
	}

	err := query.Order("created_time desc").Find(&subscriptions).Error
	return subscriptions, err
}

// GetActiveUserSubscriptions 获取用户有效订阅
func GetActiveUserSubscriptions(userId int) ([]*UserSubscription, error) {
	var subscriptions []*UserSubscription
	now := common.GetTimestamp()

	err := DB.Model(&UserSubscription{}).
		Preload("Package").
		Where("user_id = ? AND status = 1 AND start_time <= ? AND end_time > ?", userId, now, now).
		Order("created_time desc").
		Find(&subscriptions).Error

	return subscriptions, err
}

// GetUserSubscriptionById 根据ID获取用户订阅
func GetUserSubscriptionById(id int) (*UserSubscription, error) {
	if id == 0 {
		return nil, errors.New("订阅ID不能为空")
	}
	var subscription UserSubscription
	err := DB.Preload("Package").Preload("User").First(&subscription, id).Error
	return &subscription, err
}

// CheckQuotaAvailable 检查额度是否可用
func (us *UserSubscription) CheckQuotaAvailable(requiredQuota int64) (bool, string, error) {
	if us.Package == nil {
		pkg, err := GetSubscriptionPackageById(us.PackageId)
		if err != nil {
			return false, "", err
		}
		us.Package = pkg
	}

	now := common.GetTimestamp()

	// 检查订阅是否有效
	if us.Status != 1 || us.StartTime > now || us.EndTime <= now {
		return false, "订阅无效或已过期", nil
	}

	// 检查每日额度
	if us.Package.DailyQuota > 0 {
		// 检查是否需要重置每日额度
		today := time.Unix(now, 0).Truncate(24 * time.Hour).Unix()
		lastResetDay := time.Unix(us.LastDailyReset, 0).Truncate(24 * time.Hour).Unix()

		if today > lastResetDay {
			// 需要重置每日额度
			us.DailyQuotaUsed = 0
			us.LastDailyReset = now
		}

		if us.DailyQuotaUsed+requiredQuota > us.Package.DailyQuota {
			return false, fmt.Sprintf("每日额度不足，已用: %s，总额: %s，需要: %s",
				logger.LogQuota(int(us.DailyQuotaUsed)),
				logger.LogQuota(int(us.Package.DailyQuota)),
				logger.LogQuota(int(requiredQuota))), nil
		}
	}

	// 检查每月额度
	if us.Package.MonthlyQuota > 0 {
		// 检查是否需要重置每月额度
		nowTime := time.Unix(now, 0)
		lastResetTime := time.Unix(us.LastMonthlyReset, 0)

		// 如果当前月份不同于上次重置月份，则重置
		if nowTime.Year() != lastResetTime.Year() || nowTime.Month() != lastResetTime.Month() {
			us.MonthlyQuotaUsed = 0
			us.LastMonthlyReset = now
		}

		if us.MonthlyQuotaUsed+requiredQuota > us.Package.MonthlyQuota {
			return false, fmt.Sprintf("每月额度不足，已用: %s，总额: %s，需要: %s",
				logger.LogQuota(int(us.MonthlyQuotaUsed)),
				logger.LogQuota(int(us.Package.MonthlyQuota)),
				logger.LogQuota(int(requiredQuota))), nil
		}
	}

	// 检查永久额度
	if us.Package.PermanentQuota > 0 {
		if us.PermanentQuotaUsed+requiredQuota > us.Package.PermanentQuota {
			return false, fmt.Sprintf("永久额度不足，已用: %s，总额: %s，需要: %s",
				logger.LogQuota(int(us.PermanentQuotaUsed)),
				logger.LogQuota(int(us.Package.PermanentQuota)),
				logger.LogQuota(int(requiredQuota))), nil
		}
	}

	return true, "", nil
}

// ConsumeQuota 消费额度
func (us *UserSubscription) ConsumeQuota(quota int64) error {
	// 检查额度是否可用
	available, reason, err := us.CheckQuotaAvailable(quota)
	if err != nil {
		return err
	}
	if !available {
		return errors.New(reason)
	}

	// 消费额度
	if us.Package.DailyQuota > 0 {
		us.DailyQuotaUsed += quota
	}
	if us.Package.MonthlyQuota > 0 {
		us.MonthlyQuotaUsed += quota
	}
	if us.Package.PermanentQuota > 0 {
		us.PermanentQuotaUsed += quota
	}

	us.TotalUsage += quota

	return us.Update()
}

// GetExpiredSubscriptions 获取过期的订阅
func GetExpiredSubscriptions() ([]*UserSubscription, error) {
	var subscriptions []*UserSubscription
	now := common.GetTimestamp()

	err := DB.Model(&UserSubscription{}).
		Preload("Package").
		Preload("User").
		Where("status = 1 AND end_time <= ?", now).
		Find(&subscriptions).Error

	return subscriptions, err
}

// ExpireSubscriptions 使订阅过期
func ExpireSubscriptions() error {
	now := common.GetTimestamp()

	return DB.Model(&UserSubscription{}).
		Where("status = 1 AND end_time <= ?", now).
		Update("status", 3).Error // 3表示已过期
}

// ResetDailyQuota 重置所有用户的每日额度
func ResetDailyQuota() error {
	now := common.GetTimestamp()
	today := time.Unix(now, 0).Truncate(24 * time.Hour).Unix()

	// 查找需要重置的订阅
	var subscriptions []*UserSubscription
	err := DB.Model(&UserSubscription{}).
		Preload("Package").
		Where("status = 1").
		Where("last_daily_reset < ?", today).
		Find(&subscriptions).Error

	if err != nil {
		return err
	}

	// 批量重置
	for _, subscription := range subscriptions {
		if subscription.Package != nil && subscription.Package.DailyQuota > 0 {
			// 记录重置日志
			resetLog := QuotaResetLog{
				UserId:         subscription.UserId,
				SubscriptionId: subscription.Id,
				ResetType:      "daily",
				ResetTime:      now,
				PreviousUsage:  subscription.DailyQuotaUsed,
				NewQuota:       subscription.Package.DailyQuota,
				CreatedTime:    now,
			}
			DB.Create(&resetLog)

			// 重置额度
			subscription.DailyQuotaUsed = 0
			subscription.LastDailyReset = now
			subscription.Update()
		}
	}

	return nil
}

// SubscribeToPackage 用户订阅套餐
func SubscribeToPackage(userId int, packageId int, duration int) (*UserSubscription, error) {
	// 获取套餐信息
	pkg, err := GetSubscriptionPackageById(packageId)
	if err != nil {
		return nil, err
	}

	if pkg.Status != 1 {
		return nil, errors.New("套餐已禁用")
	}

	// 检查用户组限制
	user, err := GetUserById(userId, false)
	if err != nil {
		return nil, err
	}

	groupLimits := pkg.GetGroupLimitList()
	if len(groupLimits) > 0 {
		allowed := false
		for _, group := range groupLimits {
			if user.Group == group {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, errors.New("您的用户组无权订阅此套餐")
		}
	}

	// 检查套餐用户数限制
	if pkg.MaxUsersPerPackage > 0 {
		var count int64
		DB.Model(&UserSubscription{}).Where("package_id = ? AND status = 1", packageId).Count(&count)
		if int(count) >= pkg.MaxUsersPerPackage {
			return nil, errors.New("套餐用户数已达上限")
		}
	}

	now := common.GetTimestamp()

	// 如果duration为0，使用套餐默认持续时间
	if duration == 0 {
		duration = pkg.Duration
	}

	endTime := now + int64(duration*24*3600) // 转换为秒

	// 创建订阅
	subscription := &UserSubscription{
		UserId:    userId,
		PackageId: packageId,
		Status:    1,
		StartTime: now,
		EndTime:   endTime,
		Package:   pkg,
	}

	err = subscription.Insert()
	if err != nil {
		return nil, err
	}

	// 记录日志
	RecordLog(userId, LogTypeSystem, fmt.Sprintf("订阅套餐: %s，有效期至: %s",
		pkg.Name,
		time.Unix(endTime, 0).Format("2006-01-02 15:04:05")))

	return subscription, nil
}