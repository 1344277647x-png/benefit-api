package model

import (
	"errors"
	"strconv"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

var operationsReadyDB atomic.Pointer[gorm.DB]

// Accounting is a separate journal. An unsupported legacy source is explicitly
// unknown. No amount from it is ever promoted to cash revenue by inference.
func operationsOpenWalletTx(tx *gorm.DB, userId int, openingQuota int64) error {
	var count int64
	key := OperationsWalletKey(userId)
	if err := tx.Model(&OperationsPool{}).Where("pool_key = ?", key).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if openingQuota < 0 {
		openingQuota = 0
	}
	return OperationsCreditTx(tx, key, "opening:"+key, "opening", userId, openingQuota, "0", strconv.FormatInt(openingQuota, 10), common.GetTimestamp())
}

func operationsAvailable(tx *gorm.DB) bool {
	if tx == nil {
		return false
	}
	if DB != nil && operationsReadyDB.Load() == DB {
		return true
	}
	return tx.Migrator().HasTable(&OperationsEvent{})
}

// Initialize before the first reservation; a post-settlement balance may already
// exclude several other in-flight reservations and is not a valid opening value.
func prepareOperationsWallet(userId int) error {
	if !operationsAvailable(DB) {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").First(&user, userId).Error; err != nil {
			return err
		}
		return enqueueOperationsTx(tx, "opening:"+OperationsWalletKey(userId), "", operationsFact{Action: "open", UserId: userId, Opening: int64(user.Quota)})
	})
}

func operationsCreditWalletTx(tx *gorm.DB, userId, quota int, topup *TopUp, eventKey string) error {
	if !operationsAvailable(tx) {
		return nil
	}
	var user User
	if err := lockForUpdate(tx).Select("id", "quota").First(&user, userId).Error; err != nil {
		return err
	}
	f := operationsFact{Action: "credit", UserId: userId, PoolKey: OperationsWalletKey(userId), Opening: int64(user.Quota) - int64(quota), Quota: int64(quota), Gift: topup == nil, Rate: operation_setting.USDExchangeRate}
	if topup != nil {
		if topup.Id > 0 {
			eventKey = "topup:" + strconv.Itoa(topup.Id)
		}
		// These providers persist the verified local-currency payment amount.
		// Stripe/Creem or manual credits with ambiguous amount semantics stay unknown.
		f.Money = topup.Money
		f.Verified = (topup.PaymentProvider == PaymentProviderAlipay || topup.PaymentProvider == PaymentProviderEpay) && topup.Money >= 0
	}
	return enqueueOperationsTx(tx, eventKey, "", f)
}

func operationsSnapshotSubscriptionTx(tx *gorm.DB, sub *UserSubscription, order *SubscriptionOrder, chargedQuota int) error {
	return operationsSnapshotPlanTx(tx, "subscription:"+strconv.Itoa(sub.Id), sub, order, chargedQuota)
}

func operationsSnapshotPlanTx(tx *gorm.DB, key string, sub *UserSubscription, order *SubscriptionOrder, chargedQuota int) error {
	if !operationsAvailable(tx) {
		return nil
	}
	if sub.AmountTotal <= 0 || sub.NextResetTime > 0 {
		return nil
	}
	f := operationsFact{Action: "plan", PoolKey: key, UserId: sub.UserId, Quota: sub.AmountTotal, EndTime: sub.EndTime, Gift: order == nil, Rate: operation_setting.USDExchangeRate}
	if order != nil {
		f.Money = order.Money
		f.Verified = (order.PaymentProvider == PaymentProviderEpay || order.PaymentProvider == PaymentProviderAlipay) && order.Money >= 0
		if order.PaymentMethod == PaymentMethodBalance {
			f.Balance, f.Charged = true, int64(chargedQuota)
			var user User
			if err := lockForUpdate(tx).Select("id", "quota").First(&user, sub.UserId).Error; err != nil {
				return err
			}
			f.Opening = int64(user.Quota) + int64(chargedQuota)
		}
	}
	return enqueueOperationsTx(tx, "fund:"+key, "", f)
}

func operationsSnapshotTeamTx(tx *gorm.DB, sub *TeamSubscription, order *TeamOrder) error {
	if NormalizeResetPeriod(sub.ResetPeriod) != SubscriptionResetNever {
		return nil
	}
	charged := 0
	if order.PaymentMethod == PaymentMethodBalance {
		var err error
		charged, err = calcSubscriptionBalanceQuota(float64(order.Money))
		if err != nil {
			return err
		}
	}
	return operationsSnapshotPlanTx(tx, "team:"+strconv.Itoa(sub.Id),
		&UserSubscription{UserId: order.PayerUserId, AmountTotal: sub.AmountTotal, StartTime: sub.StartTime, EndTime: sub.EndTime},
		&SubscriptionOrder{Money: float64(order.Money), PaymentMethod: order.PaymentMethod, PaymentProvider: order.PaymentProvider}, charged)
}

func operationsSettleTeamTx(tx *gorm.DB, usage *TeamUsage) error {
	if !operationsAvailable(tx) {
		return nil
	}
	key := "team:" + strconv.Itoa(usage.TeamSubscriptionId)
	var team Team
	if err := tx.First(&team, usage.TeamId).Error; err != nil {
		return err
	}
	return enqueueOperationsTx(tx, "usage:"+usage.RequestId, usage.RequestId, operationsFact{Action: "usage", PoolKey: key, UserId: team.OwnerId, MemberId: usage.MemberUserId, Quota: usage.FinalAmount, Kind: "team_usage"})
}

func recordOperationsConsumption(userId int, requestId string, params RecordConsumeLogParams) error {
	if !operationsAvailable(DB) {
		return nil
	}
	if requestId == "" {
		return errors.New("accounting requires request ID")
	}
	var entry OperationsEntry
	if err := DB.First(&entry, "event_key = ?", "usage:"+requestId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if entry.Quota != int64(params.Quota) || entry.UserId != userId && entry.RequestUserId != userId {
		return errors.New("consumption does not match committed accounting")
	}
	return nil
}
