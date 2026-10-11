package model

import (
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// Reconcile term usage before recognizing any remainder. An in-flight request,
// missing settlement journal or cancelled term leaves the remainder pending.
func expireOperationsTermTx(tx *gorm.DB, key string, now int64) error {
	parts := strings.Split(key, ":")
	if len(parts) != 2 {
		return fmt.Errorf("invalid term identity")
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil || id <= 0 {
		return fmt.Errorf("invalid term identity")
	}
	var actualUsed, pending int64
	switch parts[0] {
	case "subscription":
		var sub UserSubscription
		if err := lockForUpdate(tx).First(&sub, id).Error; err != nil {
			return err
		}
		if sub.EndTime > now || sub.Status == "cancelled" || sub.NextResetTime > 0 {
			return nil
		}
		actualUsed = sub.AmountUsed
		if err := tx.Model(&SubscriptionPreConsumeRecord{}).
			Where("user_subscription_id = ? AND status <> ?", id, "refunded").
			Where("NOT EXISTS (SELECT 1 FROM operations_entries WHERE operations_entries.request_id = subscription_pre_consume_records.request_id AND operations_entries.pool_key = ? AND operations_entries.kind = ?)", key, "subscription_usage").
			Count(&pending).Error; err != nil {
			return err
		}
	case "team":
		var sub TeamSubscription
		if err := lockForUpdate(tx).First(&sub, id).Error; err != nil {
			return err
		}
		if sub.EndTime > now || NormalizeResetPeriod(sub.ResetPeriod) != SubscriptionResetNever {
			return nil
		}
		if err := tx.Model(&TeamUsage{}).Where("team_subscription_id = ? AND status = ?", id, "reserved").Count(&pending).Error; err != nil {
			return err
		}
		if err := tx.Model(&TeamQuotaPeriod{}).Where("team_subscription_id = ?", id).Select("COALESCE(SUM(amount_used), 0)").Scan(&actualUsed).Error; err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported term identity")
	}
	if pending != 0 {
		return nil
	}
	var accounted int64
	if err := tx.Model(&OperationsEntry{}).Where("pool_key = ? AND kind IN ?", key, []string{"subscription_usage", "team_usage"}).Select("COALESCE(SUM(quota), 0)").Scan(&accounted).Error; err != nil {
		return err
	}
	if actualUsed != accounted {
		return nil
	}
	return ExpireOperationsPoolTx(tx, key, now)
}

// Persist scan order so blocked terms cannot monopolize every maintenance tick.
func ExpireOperationsTerms(now int64) error {
	if !operationsAvailable(DB) {
		return nil
	}
	var firstErr error
	var pools []OperationsPool
	if err := DB.Select("pool_key").Where("end_time > 0 AND end_time <= ? AND expired = ?", now, false).Order("expiry_checked_at ASC, pool_key ASC").Limit(1000).Find(&pools).Error; err != nil {
		return err
	}
	for _, pool := range pools {
		if err := DB.Transaction(func(tx *gorm.DB) error { return expireOperationsTermTx(tx, pool.PoolKey, now) }); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := DB.Model(&OperationsPool{}).Where("pool_key = ?", pool.PoolKey).Update("expiry_checked_at", now).Error; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
