package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Economic attribution follows settlement time. Reservations change spendable
// quota only; final settlement moves economic value and the wallet delta together.
func SettleOperationsWallet(userId int, requestId string, actualQuota, delta int) error {
	if !operationsAvailable(DB) || requestId == "" {
		return adjustWalletBillingQuota(userId, delta)
	}
	if actualQuota < 0 || actualQuota > common.MaxQuota {
		return errors.New("invalid wallet settlement quota")
	}
	applied := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").First(&user, userId).Error; err != nil {
			return err
		}
		var entry OperationsEntry
		if err := tx.First(&entry, "event_key = ?", "usage:"+requestId).Error; err == nil {
			if entry.UserId != userId || entry.Quota != int64(actualQuota) || entry.Kind != "wallet_usage" {
				return errors.New("wallet settlement idempotency conflict")
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Trusted requests may settle without making a reservation first.
		if err := operationsOpenWalletTx(tx, userId, int64(user.Quota)); err != nil {
			return err
		}
		if delta < 0 && user.Quota > common.MaxWalletQuota+delta {
			return ErrWalletQuotaLimitExceeded
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota - ?", delta)).Error; err != nil {
			return err
		}
		if err := operationsEnsureInventoryTx(tx, userId, int64(actualQuota), requestId); err != nil {
			return err
		}
		if _, err := OperationsTakeTx(tx, OperationsWalletKey(userId), "usage:"+requestId, requestId, "wallet_usage", userId, int64(actualQuota), common.GetTimestamp()); err != nil {
			return err
		}
		applied = true
		return nil
	})
	if err == nil && applied && delta != 0 {
		syncOperationsWalletCache(userId, -int64(delta))
	}
	return err
}

func operationsEnsureInventoryTx(tx *gorm.DB, userId int, needed int64, requestId string) error {
	var pool OperationsPool
	if err := lockForUpdate(tx).First(&pool, "pool_key = ?", OperationsWalletKey(userId)).Error; err != nil {
		return err
	}
	if needed <= pool.Quota {
		return nil
	}
	missing := needed - pool.Quota
	return OperationsCreditTx(tx, pool.PoolKey, "unfunded:"+requestId, "unfunded", userId, missing, "0", strconv.FormatInt(missing, 10), common.GetTimestamp())
}

func syncOperationsWalletCache(userId int, delta int64) {
	if !common.RedisEnabled || delta == 0 {
		return
	}
	if err := cacheIncrUserQuota(userId, delta); err != nil {
		common.SysError("failed to synchronize accounting wallet cache: " + err.Error())
	}
}

func adjustWalletBillingQuota(userId, delta int) error {
	if delta > 0 {
		return DecreaseUserQuotaForBilling(userId, delta, false)
	}
	if delta < 0 {
		return IncreaseUserQuotaForBilling(userId, -delta, false)
	}
	return nil
}

func CreditUserGiftQuota(userId, quota int) error {
	if err := common.ValidateWalletQuota(quota); err != nil {
		return err
	}
	if !operationsAvailable(DB) {
		return IncreaseUserQuota(userId, quota, true)
	}
	return AdjustOperationsWallet(userId, quota, true)
}

// Non-request adjustments have no verified payment provenance. They never
// become cash revenue merely because an administrator or task changed quota.
func AdjustOperationsWallet(userId, delta int, gift bool) error {
	if delta == 0 {
		return nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").First(&user, userId).Error; err != nil {
			return err
		}
		if err := operationsOpenWalletTx(tx, userId, int64(user.Quota)); err != nil {
			return err
		}
		if delta > 0 && user.Quota > common.MaxWalletQuota-delta {
			return ErrWalletQuotaLimitExceeded
		}
		key := fmt.Sprintf("adjustment:%d:%d", userId, time.Now().UnixNano())
		if delta > 0 {
			unknown, kind := strconv.Itoa(delta), "unverified_credit"
			if gift {
				unknown, kind = "0", "gift"
			}
			if err := OperationsCreditTx(tx, OperationsWalletKey(userId), key, kind, userId, int64(delta), "0", unknown, common.GetTimestamp()); err != nil {
				return err
			}
		} else {
			if err := operationsEnsureInventoryTx(tx, userId, -int64(delta), key); err != nil {
				return err
			}
			if _, err := OperationsTakeTx(tx, OperationsWalletKey(userId), key, "", "adjustment", userId, -int64(delta), common.GetTimestamp()); err != nil {
				return err
			}
		}
		return tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota + ?", delta)).Error
	})
	if err == nil {
		syncOperationsWalletCache(userId, int64(delta))
	}
	return err
}

func OverrideOperationsWallet(userId, quota int) error {
	if err := common.ValidateWalletQuota(quota); err != nil {
		return err
	}
	var delta int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "quota").First(&user, userId).Error; err != nil {
			return err
		}
		delta = int64(quota) - int64(user.Quota)
		if !operationsAvailable(tx) {
			return tx.Model(&User{}).Where("id = ?", userId).Update("quota", quota).Error
		}
		if err := operationsOpenWalletTx(tx, userId, int64(user.Quota)); err != nil {
			return err
		}
		var pool OperationsPool
		if err := lockForUpdate(tx).First(&pool, "pool_key = ?", OperationsWalletKey(userId)).Error; err != nil {
			return err
		}
		key := fmt.Sprintf("override:%d:%d", userId, time.Now().UnixNano())
		if _, err := OperationsTakeTx(tx, pool.PoolKey, key+":remove", "", "adjustment", userId, pool.Quota, common.GetTimestamp()); err != nil {
			return err
		}
		if err := OperationsCreditTx(tx, pool.PoolKey, key+":fund", "unverified_credit", userId, int64(quota), "0", strconv.Itoa(quota), common.GetTimestamp()); err != nil {
			return err
		}
		delta = int64(quota) - int64(user.Quota)
		return tx.Model(&User{}).Where("id = ?", userId).Update("quota", quota).Error
	})
	if err == nil {
		syncOperationsWalletCache(userId, delta)
	}
	return err
}
