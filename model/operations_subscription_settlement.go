package model

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func SettleOperationsSubscription(userId, subscriptionId int, requestId string, actualQuota int, delta int64) error {
	if !operationsAvailable(DB) || requestId == "" {
		return PostConsumeUserSubscriptionDelta(subscriptionId, delta)
	}
	if actualQuota < 0 || actualQuota > common.MaxQuota {
		return errors.New("invalid subscription settlement quota")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var sub UserSubscription
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ?", subscriptionId, userId).First(&sub).Error; err != nil {
			return err
		}
		key := "subscription:" + strconv.Itoa(subscriptionId)
		exists, err := operationsSettlementExists(tx, "usage:"+requestId, userId, int64(actualQuota), delta, key)
		if err != nil || exists {
			return err
		}
		if delta < 0 && sub.AmountUsed < -delta || delta > 0 && sub.AmountTotal > 0 && (delta > sub.AmountTotal || sub.AmountUsed > sub.AmountTotal-delta) {
			return errors.New("subscription quota insufficient")
		}
		if err := tx.Model(&sub).Update("amount_used", gorm.Expr("amount_used + ?", delta)).Error; err != nil {
			return err
		}
		if err := enqueueOperationsTx(tx, "usage:"+requestId, requestId, operationsFact{Action: "usage", PoolKey: key, UserId: userId, Quota: int64(actualQuota), Delta: delta, Kind: "subscription_usage"}); err != nil {
			return err
		}
		return tx.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestId).Update("status", "settled").Error
	})
}
