package model

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Spendable quota and the immutable receipt commit together. Economic value is
// projected later and can never roll back a successful wallet settlement.
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
		exists, err := operationsSettlementExists(tx, "usage:"+requestId, userId, int64(actualQuota), int64(delta), OperationsWalletKey(userId))
		if err != nil || exists {
			return err
		}
		if delta < 0 && user.Quota > common.MaxWalletQuota+delta {
			return ErrWalletQuotaLimitExceeded
		}
		if err := tx.Model(&User{}).Where("id = ?", userId).Update("quota", gorm.Expr("quota - ?", delta)).Error; err != nil {
			return err
		}
		if err := enqueueOperationsTx(tx, "usage:"+requestId, requestId, operationsFact{Action: "usage", PoolKey: OperationsWalletKey(userId), UserId: userId, Opening: int64(user.Quota), Quota: int64(actualQuota), Delta: int64(delta), Kind: "wallet_usage"}); err != nil {
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
		if delta > 0 && user.Quota > common.MaxWalletQuota-delta {
			return ErrWalletQuotaLimitExceeded
		}
		key := fmt.Sprintf("adjustment:%d:%d", userId, time.Now().UnixNano())
		f := operationsFact{Action: "adjustment", PoolKey: OperationsWalletKey(userId), UserId: userId, Opening: int64(user.Quota), Quota: -int64(delta), Kind: "adjustment"}
		if delta > 0 {
			f.Action, f.Quota, f.Gift = "credit", int64(delta), gift
		}
		if err := enqueueOperationsTx(tx, key, "", f); err != nil {
			return err
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
		key := fmt.Sprintf("override:%d:%d", userId, time.Now().UnixNano())
		if err := enqueueOperationsTx(tx, key, "", operationsFact{Action: "override", PoolKey: OperationsWalletKey(userId), UserId: userId, Opening: int64(user.Quota), Quota: int64(quota)}); err != nil {
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
