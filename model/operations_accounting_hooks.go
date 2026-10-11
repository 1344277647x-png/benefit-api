package model

import (
	"errors"
	"strconv"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
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
	return tx.Migrator().HasTable(&OperationsPool{})
}

// Initialize before the first reservation; a post-settlement balance may already
// exclude several other in-flight reservations and is not a valid opening value.
func prepareOperationsWallet(userId int) error {
	if !operationsAvailable(DB) {
		return nil
	}
	var count int64
	if err := DB.Model(&OperationsPool{}).Where("pool_key = ?", OperationsWalletKey(userId)).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").First(&user, userId).Error; err != nil {
			return err
		}
		return operationsOpenWalletTx(tx, userId, int64(user.Quota))
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
	// This hook executes after the actual quota credit, in the same transaction.
	if err := operationsOpenWalletTx(tx, userId, int64(user.Quota)-int64(quota)); err != nil {
		return err
	}
	value, unknown, kind := "0", "0", "gift"
	if topup != nil {
		kind = "unverified_credit"
		if topup.Id > 0 {
			eventKey = "topup:" + strconv.Itoa(topup.Id)
			kind = "receipt"
		}
		// These providers persist the verified local-currency payment amount.
		// Stripe/Creem or manual credits with ambiguous amount semantics stay unknown.
		if (topup.PaymentProvider == PaymentProviderAlipay || topup.PaymentProvider == PaymentProviderEpay) && topup.Money >= 0 && operation_setting.USDExchangeRate > 0 {
			value = decimal.NewFromFloat(topup.Money).DivRound(decimal.NewFromFloat(operation_setting.USDExchangeRate), 24).String()
		} else {
			unknown = strconv.Itoa(quota)
		}
	}
	return OperationsCreditTx(tx, OperationsWalletKey(userId), eventKey, kind, userId, int64(quota), value, unknown, common.GetTimestamp())
}

func operationsSnapshotSubscriptionTx(tx *gorm.DB, sub *UserSubscription, order *SubscriptionOrder, chargedQuota int) error {
	return operationsSnapshotPlanTx(tx, "subscription:"+strconv.Itoa(sub.Id), sub, order, chargedQuota)
}

func operationsSnapshotPlanTx(tx *gorm.DB, key string, sub *UserSubscription, order *SubscriptionOrder, chargedQuota int) error {
	if !operationsAvailable(tx) {
		return nil
	}
	value, unknown, kind := "0", "0", "gift"
	if sub.AmountTotal <= 0 || sub.NextResetTime > 0 {
		return nil
	}
	if order != nil {
		kind = "receipt"
		if order.PaymentMethod == PaymentMethodBalance {
			kind = "transfer"
			var user User
			if err := lockForUpdate(tx).Select("id", "quota").First(&user, sub.UserId).Error; err != nil {
				return err
			}
			if err := operationsOpenWalletTx(tx, sub.UserId, int64(user.Quota)+int64(chargedQuota)); err != nil {
				return err
			}
			entry, err := OperationsTakeTx(tx, OperationsWalletKey(sub.UserId), "purchase:"+key, "", "transfer", sub.UserId, int64(chargedQuota), common.GetTimestamp())
			if err != nil {
				return err
			}
			value = entry.ValueUSD
			if chargedQuota > 0 {
				u, _ := decimal.NewFromString(entry.UnknownQuota)
				unknown = u.Mul(decimal.NewFromInt(sub.AmountTotal)).DivRound(decimal.NewFromInt(int64(chargedQuota)), 24).String()
			}
		} else if (order.PaymentProvider == PaymentProviderEpay || order.PaymentProvider == PaymentProviderAlipay) && order.Money >= 0 && operation_setting.USDExchangeRate > 0 {
			value = decimal.NewFromFloat(order.Money).DivRound(decimal.NewFromFloat(operation_setting.USDExchangeRate), 24).String()
		} else {
			unknown = strconv.FormatInt(sub.AmountTotal, 10)
		}
	}
	if err := OperationsCreditTx(tx, key, "fund:"+key, kind, sub.UserId, sub.AmountTotal, value, unknown, common.GetTimestamp()); err != nil {
		return err
	}
	return tx.Model(&OperationsPool{}).Where("pool_key = ?", key).Update("end_time", sub.EndTime).Error
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
	var pool OperationsPool
	if err := lockForUpdate(tx).First(&pool, "pool_key = ?", key).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // Historical terms lack immutable funding provenance.
		}
		return err
	}
	entry, err := OperationsTakeTx(tx, key, "usage:"+usage.RequestId, usage.RequestId, "team_usage", pool.UserId, usage.FinalAmount, common.GetTimestamp())
	if err != nil {
		return err
	}
	return tx.Model(&OperationsEntry{}).Where("event_key = ?", entry.EventKey).Update("request_user_id", usage.MemberUserId).Error
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
