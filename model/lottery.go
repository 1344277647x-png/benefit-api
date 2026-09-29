package model

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	lotteryDefaultRuleVersion = "2026-09-v1"
	lotteryJackpotInterval    = int64(50)
	lotteryPoolWeightTotal    = int64(100000)
	lotteryMaxRequestKey      = 128
)

var (
	ErrLotteryInactive           = errors.New("lottery activity is not active")
	ErrLotteryNoDraws            = errors.New("no lottery draws available")
	ErrLotteryRequestKeyRequired = errors.New("lottery idempotency key is required")
	ErrLotteryRequestConflict    = errors.New("lottery draw state changed, please retry")
)

type LotteryAccount struct {
	UserId                     int   `json:"user_id" gorm:"primaryKey"`
	RechargeRemainderCents     int64 `json:"recharge_remainder_cents"`
	AvailableDraws             int   `json:"available_draws"`
	TotalDraws                 int64 `json:"total_draws"`
	TotalEligibleRechargeCents int64 `json:"total_eligible_recharge_cents"`
	TotalRewardQuota           int64 `json:"total_reward_quota"`
	CreatedAt                  int64 `json:"created_at"`
	UpdatedAt                  int64 `json:"updated_at"`
}

type LotteryGrant struct {
	Id            int    `json:"id"`
	TopUpId       int    `json:"top_up_id" gorm:"uniqueIndex:uk_lottery_grant_topup"`
	UserId        int    `json:"user_id" gorm:"index"`
	RechargeCents int64  `json:"recharge_cents"`
	DrawsGranted  int    `json:"draws_granted"`
	RuleVersion   string `json:"rule_version" gorm:"type:varchar(64)"`
	CreatedAt     int64  `json:"created_at"`
}

type LotteryDraw struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index;uniqueIndex:uk_lottery_draw_request,priority:1"`
	RequestKey  string `json:"-" gorm:"type:varchar(128);uniqueIndex:uk_lottery_draw_request,priority:2"`
	DrawNumber  int64  `json:"draw_number"`
	Tier        string `json:"tier" gorm:"type:varchar(16)"`
	RewardCents int64  `json:"reward_cents"`
	RewardQuota int    `json:"reward_quota"`
	RuleVersion string `json:"rule_version" gorm:"type:varchar(64)"`
	CreatedAt   int64  `json:"created_at"`
}

type LotteryRule struct {
	Id             int64  `json:"id"`
	Version        string `json:"version" gorm:"type:varchar(64);uniqueIndex"`
	RegularWeights string `json:"-" gorm:"type:text"`
	JackpotWeights string `json:"-" gorm:"type:text"`
	CreatedBy      int    `json:"created_by" gorm:"index"`
	CreatedAt      int64  `json:"created_at" gorm:"type:bigint;index"`
}

type LotteryPrize struct {
	RewardCents int64 `json:"reward_cents"`
	Weight      int64 `json:"weight"`
}

type LotteryStatus struct {
	Enabled                    bool    `json:"enabled"`
	Active                     bool    `json:"active"`
	StartAt                    int64   `json:"start_at"`
	EndAt                      int64   `json:"end_at"`
	AvailableDraws             int     `json:"available_draws"`
	TotalDraws                 int64   `json:"total_draws"`
	NextDrawIsJackpot          bool    `json:"next_draw_is_jackpot"`
	RechargeRemainderCents     int64   `json:"recharge_remainder_cents"`
	AmountToNextDrawCents      int64   `json:"amount_to_next_draw_cents"`
	TotalEligibleRechargeCents int64   `json:"total_eligible_recharge_cents"`
	TotalRewardQuota           int64   `json:"total_reward_quota"`
	DrawThresholdCents         int64   `json:"draw_threshold_cents"`
	JackpotInterval            int64   `json:"jackpot_interval"`
	RuleVersion                string  `json:"-"`
	PrizeAmountsCents          []int64 `json:"prize_amounts_cents"`
}

type LotteryHistory struct {
	Items    []LotteryDraw `json:"items"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	Total    int64         `json:"total"`
}

func defaultLotteryRewardPool(tier string) []LotteryPrize {
	if tier == "jackpot" {
		return []LotteryPrize{
			{RewardCents: 500, Weight: 79999},
			{RewardCents: 1000, Weight: 15000},
			{RewardCents: 5000, Weight: 5000},
			{RewardCents: 10000, Weight: 1},
		}
	}
	return []LotteryPrize{
		{RewardCents: 50, Weight: 74999},
		{RewardCents: 100, Weight: 20000},
		{RewardCents: 200, Weight: 5000},
		{RewardCents: 10000, Weight: 1},
	}
}

func validateLotteryWeights(weights []int64) error {
	if len(weights) != 4 {
		return errors.New("lottery pool must contain all four fixed prizes")
	}
	var total int64
	for _, weight := range weights {
		if weight < 0 || weight > lotteryPoolWeightTotal {
			return errors.New("lottery weight is out of range")
		}
		total += weight
	}
	if total != lotteryPoolWeightTotal {
		return errors.New("lottery pool weights must total 100.000%")
	}
	return nil
}

func CreateLotteryRule(operatorID int, regularWeights, jackpotWeights []int64) (*LotteryRule, error) {
	if operatorID <= 0 {
		return nil, errors.New("invalid lottery rule operator")
	}
	if err := validateLotteryWeights(regularWeights); err != nil {
		return nil, err
	}
	if err := validateLotteryWeights(jackpotWeights); err != nil {
		return nil, err
	}
	regularJSON, err := common.Marshal(regularWeights)
	if err != nil {
		return nil, err
	}
	jackpotJSON, err := common.Marshal(jackpotWeights)
	if err != nil {
		return nil, err
	}
	rule := &LotteryRule{
		Version:        fmt.Sprintf("lottery-%d-%s", time.Now().UnixNano(), common.GetRandomString(6)),
		RegularWeights: string(regularJSON), JackpotWeights: string(jackpotJSON),
		CreatedBy: operatorID, CreatedAt: common.GetTimestamp(),
	}
	return rule, DB.Create(rule).Error
}

func currentLotteryRuleVersion() string {
	version := strings.TrimSpace(operation_setting.GetLotterySettingSnapshot().RuleVersion)
	if version == "" {
		return lotteryDefaultRuleVersion
	}
	return version
}

func lotteryRewardPool(tier, version string) ([]LotteryPrize, error) {
	pool := defaultLotteryRewardPool(tier)
	if version == "" || version == lotteryDefaultRuleVersion {
		return pool, nil
	}
	var rule LotteryRule
	if err := DB.Where("version = ?", version).First(&rule).Error; err != nil {
		return nil, err
	}
	encoded := rule.RegularWeights
	if tier == "jackpot" {
		encoded = rule.JackpotWeights
	}
	var weights []int64
	if err := common.UnmarshalJsonStr(encoded, &weights); err != nil {
		return nil, err
	}
	if err := validateLotteryWeights(weights); err != nil {
		return nil, err
	}
	for i := range pool {
		pool[i].Weight = weights[i]
	}
	return pool, nil
}

func lotteryRewardForRoll(tier string, roll int64, versions ...string) (int64, error) {
	if roll < 0 || roll >= lotteryPoolWeightTotal {
		return 0, errors.New("lottery roll is out of range")
	}
	version := lotteryDefaultRuleVersion
	if len(versions) > 0 && versions[0] != "" {
		version = versions[0]
	}
	pool, err := lotteryRewardPool(tier, version)
	if err != nil {
		return 0, err
	}
	for _, prize := range pool {
		if roll < prize.Weight {
			return prize.RewardCents, nil
		}
		roll -= prize.Weight
	}
	return 0, errors.New("lottery reward selection failed")
}

func lotteryReward(tier, version string) (int64, error) {
	roll, err := cryptorand.Int(cryptorand.Reader, big.NewInt(lotteryPoolWeightTotal))
	if err != nil {
		return 0, fmt.Errorf("generate lottery random value: %w", err)
	}
	return lotteryRewardForRoll(tier, roll.Int64(), version)
}

func lotteryRewardQuota(rewardCents int64) (int, error) {
	if operation_setting.Price <= 0 || math.IsNaN(operation_setting.Price) || math.IsInf(operation_setting.Price, 0) {
		return 0, ErrInvalidTopUpQuota
	}
	quota, err := common.WalletQuotaFromDecimalStrict(
		decimal.NewFromInt(rewardCents).
			Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
			Div(decimal.NewFromInt(100)).
			Div(decimal.NewFromFloat(operation_setting.Price)),
	)
	if err != nil || quota <= 0 {
		return 0, ErrInvalidTopUpQuota
	}
	return quota, nil
}

func lotteryRechargeCents(money float64) (int64, error) {
	if money <= 0 || math.IsNaN(money) || math.IsInf(money, 0) {
		return 0, ErrPaymentAmountMismatch
	}
	cents := decimal.NewFromFloat(money).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
	if cents <= 0 {
		return 0, ErrPaymentAmountMismatch
	}
	return cents, nil
}

func grantLotteryDrawsForTopUpTx(tx *gorm.DB, topUp *TopUp) error {
	if tx == nil || topUp == nil || !operation_setting.IsLotteryActiveAt(common.GetTimestamp()) {
		return nil
	}
	if topUp.PaymentProvider != PaymentProviderAlipay && topUp.PaymentProvider != PaymentProviderEpay {
		return nil
	}

	rechargeCents, err := lotteryRechargeCents(topUp.Money)
	if err != nil {
		return err
	}
	grant := LotteryGrant{
		TopUpId:       topUp.Id,
		UserId:        topUp.UserId,
		RechargeCents: rechargeCents,
		RuleVersion:   currentLotteryRuleVersion(),
		CreatedAt:     common.GetTimestamp(),
	}
	grantResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant)
	if grantResult.Error != nil {
		return grantResult.Error
	}
	if grantResult.RowsAffected == 0 {
		return nil
	}

	now := common.GetTimestamp()
	account := LotteryAccount{UserId: topUp.UserId, CreatedAt: now, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; err != nil {
		return err
	}
	if err := lockForUpdate(tx).Where("user_id = ?", topUp.UserId).First(&account).Error; err != nil {
		return err
	}

	totalCents := account.RechargeRemainderCents + rechargeCents
	drawsGranted := int(totalCents / operation_setting.LotteryDrawThresholdCents)
	remainderCents := totalCents % operation_setting.LotteryDrawThresholdCents
	if err := tx.Model(&LotteryAccount{}).Where("user_id = ?", topUp.UserId).Updates(map[string]interface{}{
		"recharge_remainder_cents":      remainderCents,
		"available_draws":               gorm.Expr("available_draws + ?", drawsGranted),
		"total_eligible_recharge_cents": gorm.Expr("total_eligible_recharge_cents + ?", rechargeCents),
		"updated_at":                    now,
	}).Error; err != nil {
		return err
	}
	return tx.Model(&LotteryGrant{}).Where("id = ?", grant.Id).Update("draws_granted", drawsGranted).Error
}

func GetLotteryStatus(userId int) (*LotteryStatus, error) {
	account := LotteryAccount{UserId: userId}
	err := DB.Where("user_id = ?", userId).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return lotteryStatusFromAccount(account), nil
}

func lotteryStatusFromAccount(account LotteryAccount) *LotteryStatus {
	setting := operation_setting.GetLotterySettingSnapshot()
	remaining := operation_setting.LotteryDrawThresholdCents - account.RechargeRemainderCents
	if remaining <= 0 || remaining > operation_setting.LotteryDrawThresholdCents {
		remaining = operation_setting.LotteryDrawThresholdCents
	}
	return &LotteryStatus{
		Enabled:                    setting.Enabled,
		Active:                     operation_setting.IsLotteryActiveAt(common.GetTimestamp()),
		StartAt:                    setting.StartAt,
		EndAt:                      setting.EndAt,
		AvailableDraws:             account.AvailableDraws,
		TotalDraws:                 account.TotalDraws,
		NextDrawIsJackpot:          (account.TotalDraws+1)%lotteryJackpotInterval == 0,
		RechargeRemainderCents:     account.RechargeRemainderCents,
		AmountToNextDrawCents:      remaining,
		TotalEligibleRechargeCents: account.TotalEligibleRechargeCents,
		TotalRewardQuota:           account.TotalRewardQuota,
		DrawThresholdCents:         operation_setting.LotteryDrawThresholdCents,
		JackpotInterval:            lotteryJackpotInterval,
		RuleVersion:                currentLotteryRuleVersion(),
		PrizeAmountsCents:          []int64{50, 100, 200, 500, 1000, 5000, 10000},
	}
}

func DrawLottery(userId int, requestKey string) (*LotteryDraw, *LotteryStatus, error) {
	requestKey = strings.TrimSpace(requestKey)
	if requestKey == "" || len(requestKey) > lotteryMaxRequestKey {
		return nil, nil, ErrLotteryRequestKeyRequired
	}

	existing := LotteryDraw{}
	if err := DB.Where("user_id = ? AND request_key = ?", userId, requestKey).First(&existing).Error; err == nil {
		status, statusErr := GetLotteryStatus(userId)
		return &existing, status, statusErr
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	if !operation_setting.IsLotteryActiveAt(common.GetTimestamp()) {
		return nil, nil, ErrLotteryInactive
	}

	var draw LotteryDraw
	var account LotteryAccount
	var credited bool
	err := DB.Transaction(func(tx *gorm.DB) error {
		now := common.GetTimestamp()
		reservation := LotteryDraw{
			UserId:      userId,
			RequestKey:  requestKey,
			RuleVersion: currentLotteryRuleVersion(),
			CreatedAt:   now,
		}
		reserveResult := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&reservation)
		if reserveResult.Error != nil {
			return reserveResult.Error
		}
		if reserveResult.RowsAffected == 0 {
			if err := tx.Where("user_id = ? AND request_key = ?", userId, requestKey).First(&draw).Error; err != nil {
				return err
			}
			if err := tx.Where("user_id = ?", userId).First(&account).Error; err != nil {
				return err
			}
			return nil
		}

		account = LotteryAccount{UserId: userId, CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("user_id = ?", userId).First(&account).Error; err != nil {
			return err
		}
		if account.AvailableDraws <= 0 {
			return ErrLotteryNoDraws
		}

		drawNumber := account.TotalDraws + 1
		tier := "regular"
		if drawNumber%lotteryJackpotInterval == 0 {
			tier = "jackpot"
		}
		rewardCents, err := lotteryReward(tier, reservation.RuleVersion)
		if err != nil {
			return err
		}
		rewardQuota, err := lotteryRewardQuota(rewardCents)
		if err != nil {
			return err
		}

		result := tx.Model(&LotteryAccount{}).
			Where("user_id = ? AND available_draws > 0 AND total_draws = ?", userId, account.TotalDraws).
			Updates(map[string]interface{}{
				"available_draws":    gorm.Expr("available_draws - 1"),
				"total_draws":        drawNumber,
				"total_reward_quota": gorm.Expr("total_reward_quota + ?", rewardQuota),
				"updated_at":         now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrLotteryRequestConflict
		}
		if err := creditTopUpQuota(tx, userId, rewardQuota, nil); err != nil {
			return err
		}

		draw = reservation
		draw.DrawNumber = drawNumber
		draw.Tier = tier
		draw.RewardCents = rewardCents
		draw.RewardQuota = rewardQuota
		if err := tx.Model(&LotteryDraw{}).Where("id = ?", draw.Id).Updates(map[string]interface{}{
			"draw_number":  drawNumber,
			"tier":         tier,
			"reward_cents": rewardCents,
			"reward_quota": rewardQuota,
		}).Error; err != nil {
			return err
		}
		if err := enqueueBusinessEventTx(tx, &BusinessEvent{
			EventKey: fmt.Sprintf("lottery:draw:%d", draw.Id), Category: BusinessEventLottery,
			Action: "lottery.draw", UserId: userId, RequestId: requestKey,
			Content: "Lottery draw completed", CreatedAt: now,
		}, map[string]any{"reward_cents": rewardCents, "reward_quota": rewardQuota, "tier": tier,
			"draw_number": drawNumber, "rule_version": draw.RuleVersion}); err != nil {
			return err
		}
		account.AvailableDraws--
		account.TotalDraws = drawNumber
		account.TotalRewardQuota += int64(rewardQuota)
		credited = true
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if credited {
		syncCreditUserQuotaCache(userId, draw.RewardQuota, "lottery reward")
	}
	return &draw, lotteryStatusFromAccount(account), nil
}

func GetLotteryHistory(userId int, page int, pageSize int) (*LotteryHistory, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	result := &LotteryHistory{Items: make([]LotteryDraw, 0), Page: page, PageSize: pageSize}
	query := DB.Model(&LotteryDraw{}).Where("user_id = ?", userId)
	if err := query.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	if err := query.Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&result.Items).Error; err != nil {
		return nil, err
	}
	return result, nil
}
