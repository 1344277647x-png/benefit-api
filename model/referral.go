package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ReferralRewardStatusPending = "pending"
	ReferralRewardStatusSettled = "settled"
	ReferralRewardStatusCapped  = "capped"
)

type ReferralReward struct {
	Id                    int    `json:"id"`
	InviterId             int    `json:"inviter_id" gorm:"index:idx_referral_inviter_status"`
	InviteeId             int    `json:"invitee_id" gorm:"uniqueIndex:uk_referral_invitee"`
	TopUpId               int    `json:"top_up_id" gorm:"uniqueIndex:uk_referral_topup"`
	TradeNo               string `json:"trade_no" gorm:"type:varchar(255);index"`
	TopUpQuota            int    `json:"top_up_quota"`
	RewardRateBasisPoints int    `json:"reward_rate_basis_points"`
	CalculatedRewardQuota int    `json:"calculated_reward_quota"`
	RewardQuota           int    `json:"reward_quota"`
	InviteeBonusQuota     int    `json:"invitee_bonus_quota"`
	Status                string `json:"status" gorm:"type:varchar(20);index:idx_referral_inviter_status"`
	AvailableAt           int64  `json:"available_at" gorm:"index"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralOverview struct {
	Enabled          bool
	ComplianceReady  bool
	AffiliateCode    string
	InviteCount      int64
	QualifiedCount   int64
	PendingQuota     int
	AvailableQuota   int
	TotalRewardQuota int
	Rules            operation_setting.ReferralSetting
}

type ReferralRewardItem struct {
	InviteeUsername string `json:"invitee_username"`
	RewardQuota     int    `json:"reward_quota"`
	Status          string `json:"status"`
	AvailableAt     int64  `json:"available_at"`
	CreatedAt       int64  `json:"created_at"`
}

type ReferralInviteeItem struct {
	Username     string `json:"username"`
	CreatedAt    int64  `json:"created_at"`
	Qualified    bool   `json:"qualified"`
	RewardStatus string `json:"reward_status"`
}

func createFirstTopupReferralReward(tx *gorm.DB, topUp *TopUp, creditedQuota int) error {
	rules := operation_setting.GetReferralSettingSnapshot()
	if !rules.Enabled || !operation_setting.IsPaymentComplianceConfirmed() || creditedQuota <= 0 || creditedQuota < rules.MinimumTopupQuota {
		return nil
	}

	var invitee User
	if err := lockForUpdate(tx).Select("id", "inviter_id", "quota").Where("id = ?", topUp.UserId).First(&invitee).Error; err != nil {
		return err
	}
	if invitee.InviterId <= 0 || invitee.InviterId == invitee.Id {
		return nil
	}
	var inviter User
	if err := lockForUpdate(tx).Select("id", "aff_quota", "aff_history").Where("id = ?", invitee.InviterId).First(&inviter).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	var earlierSuccessfulTopups int64
	if err := tx.Model(&TopUp{}).
		Where("user_id = ? AND status = ? AND id <> ?", invitee.Id, common.TopUpStatusSuccess, topUp.Id).
		Count(&earlierSuccessfulTopups).Error; err != nil {
		return err
	}
	if earlierSuccessfulTopups > 0 {
		return nil
	}

	calculatedReward, clamp := common.QuotaFromDecimalChecked(
		decimal.NewFromInt(int64(creditedQuota)).
			Mul(decimal.NewFromInt(int64(rules.RewardRateBasisPoints))).
			Div(decimal.NewFromInt(10000)),
	)
	if clamp != nil {
		return clamp
	}

	rewardQuota := calculatedReward
	if rules.PerInviteeCapQuota > 0 && rewardQuota > rules.PerInviteeCapQuota {
		rewardQuota = rules.PerInviteeCapQuota
	}

	monthStart, monthEnd := referralMonthRange(common.GetTimestamp())
	var monthlyReward int64
	if err := tx.Model(&ReferralReward{}).
		Where("inviter_id = ? AND created_at >= ? AND created_at < ?", invitee.InviterId, monthStart, monthEnd).
		Select("COALESCE(SUM(reward_quota), 0)").Scan(&monthlyReward).Error; err != nil {
		return err
	}
	if rules.MonthlyCapQuota > 0 {
		remaining := int64(rules.MonthlyCapQuota) - monthlyReward
		if remaining < 0 {
			remaining = 0
		}
		if int64(rewardQuota) > remaining {
			rewardQuota = int(remaining)
		}
	}

	// Reserve room for every pending reward while the inviter row is locked.
	// This prevents a later settlement from overflowing the int32 quota fields.
	var pendingReward int64
	if err := tx.Model(&ReferralReward{}).
		Where("inviter_id = ? AND status = ?", invitee.InviterId, ReferralRewardStatusPending).
		Select("COALESCE(SUM(reward_quota), 0)").Scan(&pendingReward).Error; err != nil {
		return err
	}
	if pendingReward < 0 {
		return errors.New("invalid pending referral reward total")
	}
	availableBalanceRoom := int64(common.MaxQuota) - int64(inviter.AffQuota) - pendingReward
	availableHistoryRoom := int64(common.MaxQuota) - int64(inviter.AffHistoryQuota) - pendingReward
	availableRewardRoom := min(availableBalanceRoom, availableHistoryRoom)
	if availableRewardRoom < 0 {
		availableRewardRoom = 0
	}
	if int64(rewardQuota) > availableRewardRoom {
		rewardQuota = int(availableRewardRoom)
	}

	inviteeBonusQuota := rules.InviteeBonusQuota
	availableInviteeRoom := int64(common.MaxQuota) - int64(invitee.Quota)
	if availableInviteeRoom < 0 {
		availableInviteeRoom = 0
	}
	if int64(inviteeBonusQuota) > availableInviteeRoom {
		inviteeBonusQuota = int(availableInviteeRoom)
	}

	now := common.GetTimestamp()
	status := ReferralRewardStatusPending
	if rewardQuota == 0 && calculatedReward > 0 {
		status = ReferralRewardStatusCapped
	} else if rules.SettlementDelayHours == 0 || rewardQuota == 0 {
		status = ReferralRewardStatusSettled
	}
	reward := ReferralReward{
		InviterId:             invitee.InviterId,
		InviteeId:             invitee.Id,
		TopUpId:               topUp.Id,
		TradeNo:               topUp.TradeNo,
		TopUpQuota:            creditedQuota,
		RewardRateBasisPoints: rules.RewardRateBasisPoints,
		CalculatedRewardQuota: calculatedReward,
		RewardQuota:           rewardQuota,
		InviteeBonusQuota:     inviteeBonusQuota,
		Status:                status,
		AvailableAt:           now + int64(rules.SettlementDelayHours*60*60),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reward)
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}

	if inviteeBonusQuota > 0 {
		if err := tx.Model(&User{}).Where("id = ?", invitee.Id).
			Update("quota", gorm.Expr("quota + ?", inviteeBonusQuota)).Error; err != nil {
			return err
		}
	}
	if status == ReferralRewardStatusSettled && rewardQuota > 0 {
		if err := tx.Model(&User{}).Where("id = ?", invitee.InviterId).Updates(map[string]any{
			"aff_quota":   gorm.Expr("aff_quota + ?", rewardQuota),
			"aff_history": gorm.Expr("aff_history + ?", rewardQuota),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func settleMaturedReferralRewards(tx *gorm.DB, inviterId int) error {
	now := common.GetTimestamp()
	var user User
	if err := lockForUpdate(tx).Select("id", "aff_quota", "aff_history").Where("id = ?", inviterId).First(&user).Error; err != nil {
		return err
	}

	var rewards []ReferralReward
	if err := lockForUpdate(tx).
		Where("inviter_id = ? AND status = ? AND available_at <= ?", inviterId, ReferralRewardStatusPending, now).
		Find(&rewards).Error; err != nil {
		return err
	}
	if len(rewards) == 0 {
		return nil
	}

	var total int64
	ids := make([]int, 0, len(rewards))
	for _, reward := range rewards {
		if reward.RewardQuota < 0 || total > int64(common.MaxQuota)-int64(reward.RewardQuota) {
			return errors.New("invalid referral reward total")
		}
		total += int64(reward.RewardQuota)
		ids = append(ids, reward.Id)
	}
	if total > int64(common.MaxQuota) {
		return errors.New("referral reward total exceeds quota limit")
	}

	if int64(user.AffQuota)+total > int64(common.MaxQuota) || int64(user.AffHistoryQuota)+total > int64(common.MaxQuota) {
		return errors.New("referral balance exceeds quota limit")
	}
	if err := tx.Model(&User{}).Where("id = ?", inviterId).Updates(map[string]any{
		"aff_quota":   gorm.Expr("aff_quota + ?", int(total)),
		"aff_history": gorm.Expr("aff_history + ?", int(total)),
	}).Error; err != nil {
		return err
	}
	return tx.Model(&ReferralReward{}).Where("id IN ? AND status = ?", ids, ReferralRewardStatusPending).
		Updates(map[string]any{"status": ReferralRewardStatusSettled, "updated_at": now}).Error
}

func GetReferralOverview(userId int) (*ReferralOverview, error) {
	if err := DB.Transaction(func(tx *gorm.DB) error { return settleMaturedReferralRewards(tx, userId) }); err != nil {
		return nil, err
	}
	var user User
	if err := DB.Select("aff_code", "aff_quota", "aff_history").Where("id = ?", userId).First(&user).Error; err != nil {
		return nil, err
	}
	if user.AffCode == "" {
		user.AffCode = common.GetRandomString(4)
		if err := DB.Model(&User{}).Where("id = ?", userId).Update("aff_code", user.AffCode).Error; err != nil {
			return nil, err
		}
	}

	var inviteCount int64
	if err := DB.Model(&User{}).Where("inviter_id = ?", userId).Count(&inviteCount).Error; err != nil {
		return nil, err
	}
	var qualifiedCount int64
	if err := DB.Model(&ReferralReward{}).Where("inviter_id = ?", userId).Count(&qualifiedCount).Error; err != nil {
		return nil, err
	}
	var pendingQuota int64
	if err := DB.Model(&ReferralReward{}).
		Where("inviter_id = ? AND status = ?", userId, ReferralRewardStatusPending).
		Select("COALESCE(SUM(reward_quota), 0)").Scan(&pendingQuota).Error; err != nil {
		return nil, err
	}
	return &ReferralOverview{
		Enabled:          operation_setting.IsReferralEnabled(),
		ComplianceReady:  operation_setting.IsPaymentComplianceConfirmed(),
		AffiliateCode:    user.AffCode,
		InviteCount:      inviteCount,
		QualifiedCount:   qualifiedCount,
		PendingQuota:     int(pendingQuota),
		AvailableQuota:   user.AffQuota,
		TotalRewardQuota: user.AffHistoryQuota,
		Rules:            operation_setting.GetReferralSettingSnapshot(),
	}, nil
}

func GetReferralRewards(userId int, pageInfo *common.PageInfo) ([]ReferralRewardItem, int64, error) {
	if err := DB.Transaction(func(tx *gorm.DB) error { return settleMaturedReferralRewards(tx, userId) }); err != nil {
		return nil, 0, err
	}
	query := DB.Model(&ReferralReward{}).Where("inviter_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rewards []ReferralReward
	if err := query.Order("id DESC").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&rewards).Error; err != nil {
		return nil, 0, err
	}

	usernames, err := getMaskedReferralUsernames(rewards)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ReferralRewardItem, 0, len(rewards))
	for _, reward := range rewards {
		items = append(items, ReferralRewardItem{
			InviteeUsername: usernames[reward.InviteeId],
			RewardQuota:     reward.RewardQuota,
			Status:          reward.Status,
			AvailableAt:     reward.AvailableAt,
			CreatedAt:       reward.CreatedAt,
		})
	}
	return items, total, nil
}

func GetReferralInvitees(userId int, pageInfo *common.PageInfo) ([]ReferralInviteeItem, int64, error) {
	query := DB.Model(&User{}).Where("inviter_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []User
	if err := query.Select("id", "username", "created_at").Order("id DESC").
		Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	ids := make([]int, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.Id)
	}
	rewardStatus := make(map[int]string)
	if len(ids) > 0 {
		var rewards []ReferralReward
		if err := DB.Select("invitee_id", "status").Where("invitee_id IN ?", ids).Find(&rewards).Error; err != nil {
			return nil, 0, err
		}
		for _, reward := range rewards {
			rewardStatus[reward.InviteeId] = reward.Status
		}
	}
	items := make([]ReferralInviteeItem, 0, len(users))
	for _, user := range users {
		status := rewardStatus[user.Id]
		items = append(items, ReferralInviteeItem{
			Username:     maskReferralUsername(user.Username),
			CreatedAt:    user.CreatedAt,
			Qualified:    status != "",
			RewardStatus: status,
		})
	}
	return items, total, nil
}

func TransferReferralQuota(userId int, quota int) error {
	if float64(quota) < common.QuotaPerUnit {
		return fmt.Errorf("转移额度最小为 %d", int(common.QuotaPerUnit))
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := settleMaturedReferralRewards(tx, userId); err != nil {
			return err
		}
		var user User
		if err := lockForUpdate(tx).Select("id", "quota", "aff_quota").Where("id = ?", userId).First(&user).Error; err != nil {
			return err
		}
		if user.AffQuota < quota {
			return errors.New("邀请额度不足")
		}
		if int64(user.Quota)+int64(quota) > int64(common.MaxQuota) {
			return errors.New("用户额度超过系统上限")
		}
		return tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]any{
			"aff_quota": gorm.Expr("aff_quota - ?", quota),
			"quota":     gorm.Expr("quota + ?", quota),
		}).Error
	})
}

func referralMonthRange(timestamp int64) (int64, int64) {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	now := time.Unix(timestamp, 0).In(location)
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	return start.Unix(), start.AddDate(0, 1, 0).Unix()
}

func getMaskedReferralUsernames(rewards []ReferralReward) (map[int]string, error) {
	ids := make([]int, 0, len(rewards))
	for _, reward := range rewards {
		ids = append(ids, reward.InviteeId)
	}
	result := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	var users []User
	if err := DB.Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		result[user.Id] = maskReferralUsername(user.Username)
	}
	return result, nil
}

func maskReferralUsername(username string) string {
	trimmed := strings.TrimSpace(username)
	runes := []rune(trimmed)
	switch len(runes) {
	case 0:
		return "***"
	case 1:
		return string(runes[0]) + "*"
	case 2:
		return string(runes[0]) + "*"
	default:
		return string(runes[0]) + strings.Repeat("*", min(len(runes)-2, 3)) + string(runes[len(runes)-1])
	}
}
