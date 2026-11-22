package model

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func accountingIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &UserSubscription{}, &SubscriptionPreConsumeRecord{}, &BusinessEvent{}))
	old, redis := DB, common.RedisEnabled
	DB, common.RedisEnabled = db, false
	t.Cleanup(func() { DB, common.RedisEnabled = old, redis })
	return db
}

func TestOperationsPoisonedPoolDoesNotStopBalancesOrOtherUsers(t *testing.T) {
	db := accountingIsolationDB(t)
	require.NoError(t, db.Create(&[]User{{Id: 1, Username: "poison", AffCode: "p001", Quota: 100}, {Id: 2, Username: "healthy", AffCode: "h002", Quota: 100}}).Error)
	require.NoError(t, db.Create(&[]OperationsPool{{PoolKey: "wallet:1", UserId: 1, Quota: 100, ValueUSD: "invalid", UnknownQuota: "0"}, {PoolKey: "wallet:2", UserId: 2, Quota: 100, ValueUSD: "10", UnknownQuota: "0"}}).Error)
	for _, r := range []struct {
		id  int
		req string
	}{{1, "poison-1"}, {1, "poison-2"}, {2, "healthy-1"}} {
		require.NoError(t, SettleOperationsWallet(r.id, r.req, 10, 10))
	}
	require.Error(t, ProcessOperationsEvents(context.Background(), 200))
	var users []User
	require.NoError(t, db.Order("id").Find(&users).Error)
	assert.Equal(t, 80, users[0].Quota)
	assert.Equal(t, 90, users[1].Quota)
	var healthy OperationsEntry
	require.NoError(t, db.First(&healthy, "event_key = ?", "usage:healthy-1").Error)
	assert.Equal(t, "1", healthy.ValueUSD)
	var events []OperationsEvent
	require.NoError(t, db.Where("user_id = ?", 1).Order("id").Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, 1, events[0].Attempts)
	assert.Zero(t, events[1].Attempts) // cannot reorder one funding inventory
	var alerts int64
	require.NoError(t, db.Model(&BusinessEvent{}).Where("action = ?", "accounting_projection_failed").Count(&alerts).Error)
	assert.Equal(t, int64(1), alerts)
	// Repair reporting only, then simulate worker restart/replay. No re-charge.
	require.NoError(t, db.Model(&OperationsPool{}).Where("pool_key = ?", "wallet:1").Update("value_usd", "10").Error)
	require.NoError(t, db.Model(&OperationsEvent{}).Where("user_id = ?", 1).Update("next_attempt_at", 0).Error)
	require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
	require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
	require.NoError(t, SettleOperationsWallet(1, "poison-1", 10, 10))
	require.Error(t, SettleOperationsWallet(1, "poison-1", 11, 11))
	var user User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 80, user.Quota)
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "wallet:1").Error)
	assert.Equal(t, int64(80), pool.Quota)
	assert.Equal(t, "8", pool.ValueUSD)
}

func TestOperationsReceiptFailureRollsBackCoreSettlement(t *testing.T) {
	db := accountingIsolationDB(t)
	require.NoError(t, db.Create(&User{Id: 1, Username: "receipt-failure", Quota: 100}).Error)
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_receipt BEFORE INSERT ON operations_events BEGIN SELECT RAISE(ABORT, 'receipt unavailable'); END`).Error)
	require.Error(t, SettleOperationsWallet(1, "no-receipt", 10, 10))
	var user User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 100, user.Quota)
	var count int64
	require.NoError(t, db.Model(&OperationsEvent{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestOperationsInvalidReportingRateDoesNotRejectFundingEvent(t *testing.T) {
	db := accountingIsolationDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return enqueueOperationsTx(tx, "invalid-rate", "", operationsFact{Action: "credit", PoolKey: "wallet:1", UserId: 1, Quota: 100, Verified: true, Money: 70, Rate: math.NaN()})
	}))
	require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "wallet:1").Error)
	assert.Equal(t, int64(100), pool.Quota)
	assert.Equal(t, "100", pool.UnknownQuota)
	assert.Equal(t, "0", pool.ValueUSD)
}

func TestOperationsSubscriptionWithoutReportPoolSettlesExactlyOnce(t *testing.T) {
	db := accountingIsolationDB(t)
	require.NoError(t, db.Create(&UserSubscription{Id: 1, UserId: 1, AmountTotal: 100, AmountUsed: 20}).Error)
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{RequestId: "legacy", UserId: 1, UserSubscriptionId: 1, PreConsumed: 20, Status: "consumed"}).Error)
	require.NoError(t, SettleOperationsSubscription(1, 1, "legacy", 10, -10))
	require.NoError(t, SettleOperationsSubscription(1, 1, "legacy", 10, -10))
	require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
	require.NoError(t, RefundSubscriptionPreConsume("legacy"))
	var sub UserSubscription
	require.NoError(t, db.First(&sub, 1).Error)
	assert.Equal(t, int64(10), sub.AmountUsed)
	var entryCount int64
	require.NoError(t, db.Model(&OperationsEntry{}).Where("request_id = ?", "legacy").Count(&entryCount).Error)
	assert.Zero(t, entryCount) // no invented revenue
}

func TestOperationsFundingOrderAndExchangeRateAreFrozen(t *testing.T) {
	db := accountingIsolationDB(t)
	oldRate := operation_setting.USDExchangeRate
	operation_setting.USDExchangeRate = 7
	t.Cleanup(func() { operation_setting.USDExchangeRate = oldRate })
	require.NoError(t, db.Create(&User{Id: 1, Username: "mixed", Quota: 100}).Error)
	// Paid 70 local currency for 100 quota; then a 100 quota gift.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return operationsCreditWalletTx(tx, 1, 100, &TopUp{Id: 1, Money: 70, PaymentProvider: PaymentProviderEpay}, "paid")
	}))
	require.NoError(t, CreditUserGiftQuota(1, 100))
	// Purchase consumes half the mixed wallet -> $5 of value in 64 plan quota.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", 1).Update("quota", 100).Error; err != nil {
			return err
		}
		return operationsSnapshotSubscriptionTx(tx, &UserSubscription{Id: 1, UserId: 1, AmountTotal: 64, EndTime: 9999999999}, &SubscriptionOrder{PaymentMethod: PaymentMethodBalance}, 100)
	}))
	require.NoError(t, db.Create(&UserSubscription{Id: 1, UserId: 1, AmountTotal: 64, AmountUsed: 0}).Error)
	require.NoError(t, SettleOperationsSubscription(1, 1, "plan-use", 32, 32))
	operation_setting.USDExchangeRate = 70
	require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
	require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
	var income OperationsEntry
	require.NoError(t, db.First(&income, "event_key = ?", "usage:plan-use").Error)
	assert.Equal(t, "2.5", income.ValueUSD)
	assert.Equal(t, "0", income.UnknownQuota)
	var wallet OperationsPool
	require.NoError(t, db.First(&wallet, "pool_key = ?", "wallet:1").Error)
	assert.Equal(t, int64(100), wallet.Quota)
	assert.Equal(t, "5", wallet.ValueUSD)
}

func TestOperationsConcurrentSettlementAndProjectionAreIdempotent(t *testing.T) {
	db := accountingIsolationDB(t)
	require.NoError(t, db.Create(&User{Id: 1, Username: "concurrent", Quota: 100}).Error)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- SettleOperationsWallet(1, "same", 10, 10) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	errs = make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- ProcessOperationsEvents(context.Background(), 200) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var user User
	require.NoError(t, db.First(&user, 1).Error)
	assert.Equal(t, 90, user.Quota)
	var entries int64
	require.NoError(t, db.Model(&OperationsEntry{}).Where("request_id = ?", "same").Count(&entries).Error)
	assert.Equal(t, int64(1), entries)
}
