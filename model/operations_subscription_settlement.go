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
		var e OperationsEntry
		if err := tx.First(&e, "event_key = ?", "usage:"+requestId).Error; err == nil {
			if e.UserId != userId || e.Quota != int64(actualQuota) || e.PoolKey != "subscription:"+strconv.Itoa(subscriptionId) {
				return errors.New("subscription settlement idempotency conflict")
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if delta < 0 && sub.AmountUsed < -delta || delta > 0 && sub.AmountTotal > 0 && (delta > sub.AmountTotal || sub.AmountUsed > sub.AmountTotal-delta) {
			return errors.New("subscription quota insufficient")
		}
		if err := tx.Model(&sub).Update("amount_used", gorm.Expr("amount_used + ?", delta)).Error; err != nil {
			return err
		}
		key := "subscription:" + strconv.Itoa(subscriptionId)
		var pool OperationsPool
		if err := tx.First(&pool, "pool_key = ?", key).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			} // Old and unlimited terms remain unverified.
			return err
		}
		if _, err := OperationsTakeTx(tx, key, "usage:"+requestId, requestId, "subscription_usage", userId, int64(actualQuota), common.GetTimestamp()); err != nil {
			return err
		}
		return tx.Model(&SubscriptionPreConsumeRecord{}).Where("request_id = ?", requestId).Update("status", "settled").Error
	})
}
