package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func createTeamOrderTx(tx *gorm.DB, ownerId, planId int, provider, method string) (*TeamOrder, error) {
	var team Team
	if err := lockForUpdate(tx).Where("owner_id = ? AND status = ?", ownerId, TeamStatusActive).First(&team).Error; err != nil {
		return nil, err
	}
	plan, err := getSubscriptionPlanByIdTx(tx, planId)
	if err != nil {
		return nil, err
	}
	if !plan.Enabled || plan.Scope != "team" || plan.SeatLimit < 2 || plan.SeatLimit > 100 ||
		plan.TotalAmount <= 0 || plan.TotalAmount > common.MaxWalletQuota || plan.PriceAmount < 0.01 || plan.PriceAmount > 9999 ||
		!decimal.NewFromFloat(plan.PriceAmount).Equal(decimal.NewFromFloat(plan.PriceAmount).Round(2)) {
		return nil, errors.New("team plan unavailable")
	}
	if provider == PaymentProviderBalance && plan.AllowBalancePay != nil && !*plan.AllowBalancePay {
		return nil, errors.New("this plan does not accept wallet balance")
	}
	now := common.GetTimestamp()
	var count int64
	if err := tx.Model(&TeamMember{}).Where("team_id = ?", team.Id).Count(&count).Error; err != nil {
		return nil, err
	}
	var pending int64
	if err := tx.Model(&TeamInvitation{}).Where("team_id = ? AND status = ? AND expires_at > ?", team.Id, TeamInvitationPending, now).Count(&pending).Error; err != nil {
		return nil, err
	}
	if count+pending > int64(plan.SeatLimit) {
		return nil, errors.New("remove members or cancel invitations before buying a smaller plan")
	}
	if err := tx.Model(&TeamOrder{}).Where("team_id = ? AND status = ?", team.Id, common.TopUpStatusPending).Count(&count).Error; err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, errors.New("team already has a pending payment")
	}
	order := &TeamOrder{
		TeamId: team.Id, PayerUserId: ownerId, PlanId: plan.Id, PlanTitle: plan.Title,
		SeatLimit: plan.SeatLimit, AmountTotal: plan.TotalAmount, DurationUnit: plan.DurationUnit,
		DurationValue: plan.DurationValue, CustomSeconds: plan.CustomSeconds,
		ResetPeriod: NormalizeResetPeriod(plan.QuotaResetPeriod), ResetCustomSeconds: plan.QuotaResetCustomSeconds,
		Money: TeamOrderAmount(plan.PriceAmount), TradeNo: fmt.Sprintf("TEAM%dNO%s%d", team.Id, common.GetRandomString(8), time.Now().UnixNano()),
		PaymentMethod: method, PaymentProvider: provider, Status: common.TopUpStatusPending, CreateTime: now,
	}
	if err := tx.Create(order).Error; err != nil {
		return nil, err
	}
	return order, nil
}

func CreateTeamPaymentOrder(ownerId, planId int, provider, method string) (*TeamOrder, error) {
	var order *TeamOrder
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		order, err = createTeamOrderTx(tx, ownerId, planId, provider, method)
		return err
	})
	return order, err
}

func completeTeamOrderTx(tx *gorm.DB, order *TeamOrder) error {
	if order.Status == common.TopUpStatusSuccess {
		return nil
	}
	if order.Status != common.TopUpStatusPending {
		return errors.New("team order is not pending")
	}
	var team Team
	if err := lockForUpdate(tx).Where("id = ? AND owner_id = ? AND status = ?", order.TeamId, order.PayerUserId, TeamStatusActive).First(&team).Error; err != nil {
		return err
	}
	var members int64
	if err := tx.Model(&TeamMember{}).Where("team_id = ?", team.Id).Count(&members).Error; err != nil {
		return err
	}
	var invitations int64
	if err := tx.Model(&TeamInvitation{}).Where("team_id = ? AND status = ? AND expires_at > ?", team.Id, TeamInvitationPending, common.GetTimestamp()).Count(&invitations).Error; err != nil {
		return err
	}
	if members+invitations > int64(order.SeatLimit) {
		return errors.New("team exceeds purchased seat limit")
	}
	var last TeamSubscription
	result := tx.Where("team_id = ? AND status = ?", team.Id, TeamStatusActive).Order("end_time desc").First(&last)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}
	start := common.GetTimestamp()
	if result.Error == nil && last.EndTime > start {
		start = last.EndTime
	}
	plan := &SubscriptionPlan{DurationUnit: order.DurationUnit, DurationValue: order.DurationValue, CustomSeconds: order.CustomSeconds}
	end, err := calcPlanEndTime(time.Unix(start, 0), plan)
	if err != nil {
		return err
	}
	sub := &TeamSubscription{TeamId: team.Id, PlanId: order.PlanId, PlanTitle: order.PlanTitle,
		SeatLimit: order.SeatLimit, AmountTotal: order.AmountTotal, ResetPeriod: order.ResetPeriod,
		ResetCustomSeconds: order.ResetCustomSeconds, StartTime: start, EndTime: end, Status: TeamStatusActive}
	if err := tx.Create(sub).Error; err != nil {
		return err
	}
	order.Status = common.TopUpStatusSuccess
	order.CompleteTime = common.GetTimestamp()
	return tx.Save(order).Error
}

func PurchaseTeamWithBalance(ownerId, planId int) error {
	var charged int
	err := DB.Transaction(func(tx *gorm.DB) error {
		order, err := createTeamOrderTx(tx, ownerId, planId, PaymentProviderBalance, PaymentMethodBalance)
		if err != nil {
			return err
		}
		charged, err = calcSubscriptionBalanceQuota(float64(order.Money))
		if err != nil {
			return err
		}
		var owner User
		if err := lockForUpdate(tx).Where("id = ? AND status = ?", ownerId, common.UserStatusEnabled).First(&owner).Error; err != nil {
			return err
		}
		if owner.Quota < charged {
			return errors.New("insufficient wallet balance")
		}
		if err := tx.Model(&owner).Update("quota", gorm.Expr("quota - ?", charged)).Error; err != nil {
			return err
		}
		return completeTeamOrderTx(tx, order)
	})
	if err == nil && charged > 0 {
		if cacheErr := cacheDecrUserQuota(ownerId, int64(charged)); cacheErr != nil {
			common.SysLog("failed to update wallet cache after team purchase: " + cacheErr.Error())
		}
	}
	return err
}

func CompleteTeamPayment(tradeNo, provider, providerTradeNo, money, method string) error {
	paid, err := decimal.NewFromString(money)
	if err != nil || paid.Sign() <= 0 || tradeNo == "" || providerTradeNo == "" || method == "" || provider != PaymentProviderEpay {
		return ErrPaymentMethodMismatch
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var order TeamOrder
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&order).Error; err != nil {
			return err
		}
		if order.PaymentProvider != provider || !decimal.NewFromFloat(float64(order.Money)).Round(2).Equal(paid) || order.PaymentMethod != method {
			return ErrPaymentMethodMismatch
		}
		if order.ProviderTradeNo != nil && *order.ProviderTradeNo != providerTradeNo {
			return ErrPaymentMethodMismatch
		}
		if order.ProviderTradeNo == nil {
			order.ProviderTradeNo = &providerTradeNo
		}
		return completeTeamOrderTx(tx, &order)
	})
}

func ExpireTeamPayment(tradeNo string) error {
	return DB.Model(&TeamOrder{}).Where("trade_no = ? AND status = ?", tradeNo, common.TopUpStatusPending).
		Update("status", common.TopUpStatusExpired).Error
}
