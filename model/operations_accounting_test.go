package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func operationsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&OperationsPool{}, &OperationsEntry{}, &ChannelCostVersion{}))
	return db
}

func TestOperationsProportionalGiftAndSubscriptionExpiry(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		if err := OperationsCreditTx(tx, "wallet:1", "paid:1", "receipt", 1, 10000, "100", "0", 1); err != nil {
			return err
		}
		if err := OperationsCreditTx(tx, "wallet:1", "gift:1", "gift", 1, 2000, "0", "0", 2); err != nil {
			return err
		}
		entry, err := OperationsTakeTx(tx, "wallet:1", "consume:1", "r1", "wallet_usage", 1, 1200, 3)
		require.NoError(t, err)
		require.Equal(t, "10", entry.ValueUSD)
		require.Equal(t, "0", entry.UnknownQuota)
		again, err := OperationsTakeTx(tx, "wallet:1", "consume:1", "r1", "wallet_usage", 1, 1200, 3)
		require.NoError(t, err)
		require.Equal(t, entry.ValueUSD, again.ValueUSD)
		_, err = OperationsTakeTx(tx, "wallet:1", "consume:1", "r1", "wallet_usage", 1, 1300, 3)
		require.Error(t, err)
		funds, err := OperationsTakeTx(tx, "wallet:1", "purchase:1", "", "transfer", 1, 6000, 4)
		require.NoError(t, err)
		require.Equal(t, "50", funds.ValueUSD)
		if err = OperationsCreditTx(tx, "subscription:1", "order:1", "transfer", 1, 6400, funds.ValueUSD, funds.UnknownQuota, 4); err != nil {
			return err
		}
		if err = tx.Model(&OperationsPool{}).Where("pool_key = ?", "subscription:1").Update("end_time", 100).Error; err != nil {
			return err
		}
		used, err := OperationsTakeTx(tx, "subscription:1", "consume:2", "r2", "subscription_usage", 1, 3200, 5)
		require.NoError(t, err)
		require.Equal(t, "25", used.ValueUSD)
		require.Error(t, ExpireOperationsPoolTx(tx, "subscription:1", 99))
		if err = ExpireOperationsPoolTx(tx, "subscription:1", 100); err != nil {
			return err
		}
		return ExpireOperationsPoolTx(tx, "subscription:1", 101)
	}))
	var expiry OperationsEntry
	require.NoError(t, db.First(&expiry, "event_key = ?", "expiry:subscription:1").Error)
	require.Equal(t, "25", expiry.ValueUSD)
	var wallet OperationsPool
	require.NoError(t, db.First(&wallet, "pool_key = ?", "wallet:1").Error)
	require.Equal(t, int64(4800), wallet.Quota)
	require.Equal(t, "40", wallet.ValueUSD)
}

func TestOperations5964AndUnknownRemainder(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, OperationsCreditTx(db, "subscription:2", "order:2", "receipt", 2, 6400, "59", "0", 1))
	e, err := OperationsTakeTx(db, "subscription:2", "c:1", "r1", "subscription_usage", 2, 1000, 2)
	require.NoError(t, err)
	require.Equal(t, "9.21875", e.ValueUSD)
	require.NoError(t, OperationsCreditTx(db, "wallet:3", "legacy:3", "opening", 3, 100, "0", "100", 1))
	require.NoError(t, OperationsCreditTx(db, "wallet:3", "paid:3", "receipt", 3, 100, "10", "0", 2))
	e, err = OperationsTakeTx(db, "wallet:3", "c:3", "r3", "wallet_usage", 3, 60, 3)
	require.NoError(t, err)
	require.Equal(t, "3", e.ValueUSD)
	require.Equal(t, "30", e.UnknownQuota)
	rest, err := OperationsTakeTx(db, "wallet:3", "c:4", "r4", "wallet_usage", 3, 140, 4)
	require.NoError(t, err)
	a, _ := decimal.NewFromString(e.ValueUSD)
	b, _ := decimal.NewFromString(rest.ValueUSD)
	require.True(t, a.Add(b).Equal(decimal.NewFromInt(10)))
}

func TestOperationsCostRemovesSellingMultiplier(t *testing.T) {
	previous := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = previous })
	for _, tc := range []struct {
		name  string
		other map[string]any
		want  float64
		known bool
	}{
		{"group", map[string]any{"group_ratio": 2.0}, 0.1, true},
		{"no special price sentinel", map[string]any{"group_ratio": 2.0, "user_group_ratio": -1}, 0.1, true},
		{"special", map[string]any{"group_ratio": 2.0, "user_group_ratio": 0.5}, 0.4, true},
		{"missing", map[string]any{}, 0, false},
		{"zero", map[string]any{"group_ratio": 0}, 0, false},
		{"tools", map[string]any{"group_ratio": 2, "tool_surcharges": []any{1}}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, known := OperationsEstimateCost(1000000, tc.other, "0.1")
			require.Equal(t, tc.known, known)
			require.InDelta(t, tc.want, got, 1e-10)
		})
	}
}

func TestOperationsWithdrawalRollsBackWhenJournalWriteFails(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, OperationsCreditTx(db, "wallet:4", "fund:4", "receipt", 4, 100, "10", "0", 1))
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_usage BEFORE INSERT ON operations_entries
		WHEN NEW.kind = 'wallet_usage' BEGIN SELECT RAISE(ABORT, 'journal unavailable'); END`).Error)
	_, err := OperationsTakeTx(db, "wallet:4", "usage:4", "r4", "wallet_usage", 4, 25, 2)
	require.Error(t, err)
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "wallet:4").Error)
	require.Equal(t, int64(100), pool.Quota)
	require.Equal(t, "10", pool.ValueUSD)
}

func TestOperationsCostUsesStartTimeAndLeavesMissingSnapshotsUnknown(t *testing.T) {
	db := operationsTestDB(t)
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	require.NoError(t, db.Create(&[]ChannelCostVersion{
		{ChannelId: 1, Multiplier: "0.1", EffectiveAt: 100},
		{ChannelId: 1, Multiplier: "0.9", EffectiveAt: 200},
	}).Error)
	logs := []operationsReportLog{
		{Type: LogTypeConsume, ChannelId: 1, CreatedAt: 250, Quota: 500000, Other: `{"group_ratio":1,"operations_request_started_at":150}`},
		{Type: LogTypeConsume, ChannelId: 1, CreatedAt: 250, Quota: 500000, Other: `{"group_ratio":1}`},
	}
	require.NoError(t, enrichOperationsAccounting(logs, OperationsReportParams{EndTimestamp: 300}))
	cost, known := operationReportCostValue(logs[0].Other)
	require.True(t, known)
	require.InDelta(t, 500000/common.QuotaPerUnit*0.1, cost, 1e-10)
	_, known = operationReportCostValue(logs[1].Other)
	require.False(t, known)
}

func TestOperationsCostVersionRejectsScientificNotationBeforeParsing(t *testing.T) {
	for _, input := range []string{"1e2147483647", "1e-2147483647", "", "-0.1", "1001", "0.000000001"} {
		_, err := SaveChannelCostVersion(1, 1, input, common.GetTimestamp())
		require.Error(t, err)
	}
}

func TestOperationsCheckinCreditsGiftAndDuplicateDoesNotCreditAgain(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Checkin{}))
	previousDB, previousRedis := DB, common.RedisEnabled
	DB, common.RedisEnabled = db, false
	t.Cleanup(func() { DB, common.RedisEnabled = previousDB, previousRedis })
	require.NoError(t, db.Create(&User{Id: 7, Username: "checkin-accounting", Quota: 100}).Error)
	require.NoError(t, OperationsCreditTx(db, "wallet:7", "fund:7", "receipt", 7, 100, "10", "0", 1))
	_, err := userCheckinWithTransaction(&Checkin{UserId: 7, CheckinDate: "2026-10-11", QuotaAwarded: 20}, 7, 20)
	require.NoError(t, err)
	_, err = userCheckinWithTransaction(&Checkin{UserId: 7, CheckinDate: "2026-10-11", QuotaAwarded: 20}, 7, 20)
	require.Error(t, err)
	var user User
	require.NoError(t, db.First(&user, 7).Error)
	require.Equal(t, 120, user.Quota)
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "wallet:7").Error)
	require.Equal(t, int64(120), pool.Quota)
	require.Equal(t, "10", pool.ValueUSD)
	require.Equal(t, "0", pool.UnknownQuota)
}

func TestOperationsExpiryWaitsForSubscriptionSettlement(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &SubscriptionPreConsumeRecord{}))
	require.NoError(t, db.Create(&UserSubscription{Id: 3, UserId: 3, AmountTotal: 6400, AmountUsed: 1000, StartTime: 1, EndTime: 100, Status: "expired"}).Error)
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{RequestId: "pending-sub", UserId: 3, UserSubscriptionId: 3, PreConsumed: 1000, Status: "consumed"}).Error)
	require.NoError(t, OperationsCreditTx(db, "subscription:3", "fund:3", "receipt", 3, 6400, "59", "0", 1))
	require.NoError(t, db.Model(&OperationsPool{}).Where("pool_key = ?", "subscription:3").Update("end_time", 100).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return expireOperationsTermTx(tx, "subscription:3", 101) }))
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "subscription:3").Error)
	require.False(t, pool.Expired)
	_, err := OperationsTakeTx(db, "subscription:3", "usage:pending-sub", "pending-sub", "subscription_usage", 3, 1000, 102)
	require.NoError(t, err)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return expireOperationsTermTx(tx, "subscription:3", 103) }))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return expireOperationsTermTx(tx, "subscription:3", 104) }))
	var expiry OperationsEntry
	require.NoError(t, db.First(&expiry, "event_key = ?", "expiry:subscription:3").Error)
	require.Equal(t, "49.78125", expiry.ValueUSD)
}

func TestOperationsTeamIncomeBelongsToRequestingMember(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, OperationsCreditTx(db, "team:8", "fund:team:8", "receipt", 1, 6400, "59", "0", 1))
	usage := &TeamUsage{RequestId: "member-request", TeamSubscriptionId: 8, MemberUserId: 2, FinalAmount: 1000}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return operationsSettleTeamTx(tx, usage) }))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error { return operationsSettleTeamTx(tx, usage) }))
	var entry OperationsEntry
	require.NoError(t, db.First(&entry, "event_key = ?", "usage:member-request").Error)
	require.Equal(t, 1, entry.UserId)
	require.Equal(t, 2, entry.RequestUserId)
	require.Equal(t, "9.21875", entry.ValueUSD)
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "team:8").Error)
	require.Equal(t, int64(5400), pool.Quota)
}

func TestOperationsWalletSettlementCommitsDeltaAndIncomeExactlyOnce(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	oldDB, oldRedis, oldBatch := DB, common.RedisEnabled, common.BatchUpdateEnabled
	DB, common.RedisEnabled, common.BatchUpdateEnabled = db, false, true
	t.Cleanup(func() { DB, common.RedisEnabled, common.BatchUpdateEnabled = oldDB, oldRedis, oldBatch })
	require.NoError(t, db.Create(&User{Id: 9, Username: "settlement-user", Quota: 6400}).Error)
	require.NoError(t, OperationsCreditTx(db, "wallet:9", "fund:9", "receipt", 9, 6400, "59", "0", 1))
	reserved, err := TryReserveUserQuota(9, 1000)
	require.NoError(t, err)
	require.True(t, reserved)
	require.NoError(t, SettleOperationsWallet(9, "wallet-request", 800, -200))
	require.NoError(t, SettleOperationsWallet(9, "wallet-request", 800, -200))
	var user User
	require.NoError(t, db.First(&user, 9).Error)
	require.Equal(t, 5600, user.Quota)
	var entry OperationsEntry
	require.NoError(t, db.First(&entry, "event_key = ?", "usage:wallet-request").Error)
	require.Equal(t, "7.375", entry.ValueUSD)
	require.Equal(t, "0", entry.UnknownQuota)
	require.NoError(t, CreditUserGiftQuota(9, 200))
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "wallet:9").Error)
	require.Equal(t, int64(5800), pool.Quota)
	require.Equal(t, "51.625", pool.ValueUSD)
}

func TestOperationsSettlementJournalFailureRollsBackWalletRefundDelta(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	oldDB, oldRedis := DB, common.RedisEnabled
	DB, common.RedisEnabled = db, false
	t.Cleanup(func() { DB, common.RedisEnabled = oldDB, oldRedis })
	require.NoError(t, db.Create(&User{Id: 10, Username: "rollback-settlement", Quota: 5400}).Error)
	require.NoError(t, OperationsCreditTx(db, "wallet:10", "fund:10", "receipt", 10, 6400, "59", "0", 1))
	require.NoError(t, db.Exec(`CREATE TRIGGER reject_wallet_settlement BEFORE INSERT ON operations_entries WHEN NEW.kind = 'wallet_usage' BEGIN SELECT RAISE(ABORT, 'journal offline'); END`).Error)
	require.Error(t, SettleOperationsWallet(10, "failed-wallet", 800, -200))
	var user User
	require.NoError(t, db.First(&user, 10).Error)
	require.Equal(t, 5400, user.Quota)
	var pool OperationsPool
	require.NoError(t, db.First(&pool, "pool_key = ?", "wallet:10").Error)
	require.Equal(t, int64(6400), pool.Quota)
	require.Equal(t, "59", pool.ValueUSD)
}

func TestOperationsSubscriptionSettlementDoesNotDependOnConsumeLog(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &SubscriptionPreConsumeRecord{}))
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	require.NoError(t, db.Create(&UserSubscription{Id: 9, UserId: 9, AmountTotal: 6400, AmountUsed: 1000, EndTime: 1000, Status: "active"}).Error)
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{RequestId: "sub-request", UserId: 9, UserSubscriptionId: 9, PreConsumed: 1000, Status: "consumed"}).Error)
	require.NoError(t, OperationsCreditTx(db, "subscription:9", "fund:sub:9", "receipt", 9, 6400, "59", "0", 1))
	require.NoError(t, SettleOperationsSubscription(9, 9, "sub-request", 800, -200))
	require.NoError(t, SettleOperationsSubscription(9, 9, "sub-request", 800, -200))
	var sub UserSubscription
	require.NoError(t, db.First(&sub, 9).Error)
	require.Equal(t, int64(800), sub.AmountUsed)
	var entry OperationsEntry
	require.NoError(t, db.First(&entry, "event_key = ?", "usage:sub-request").Error)
	require.Equal(t, "7.375", entry.ValueUSD)
}

func TestOperationsStartIsPersistentAndExcludesOldLogs(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&OperationsState{}, &Log{}))
	oldDB, oldLog := DB, LOG_DB
	DB, LOG_DB = db, db
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog })
	require.NoError(t, db.Create(&OperationsState{Id: 1, StartedAt: 200}).Error)
	require.NoError(t, EnsureOperationsStart(db))
	require.NoError(t, db.Create(&[]Log{{Type: LogTypeConsume, CreatedAt: 199, RequestId: "old"}, {Type: LogTypeConsume, CreatedAt: 201, RequestId: "new"}}).Error)
	report, err := GetOperationsReport(OperationsReportParams{StartTimestamp: 100, EndTimestamp: 300})
	require.NoError(t, err)
	require.Equal(t, int64(200), report.AccountingStartedAt)
	require.Equal(t, int64(1), report.Summary.RequestCount)
	report, err = GetOperationsReport(OperationsReportParams{StartTimestamp: 100, EndTimestamp: 150})
	require.NoError(t, err)
	require.Equal(t, int64(0), report.Summary.RequestCount)
}

func TestOperationsMissingConsumeLogSuppressesProfit(t *testing.T) {
	db := operationsTestDB(t)
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	require.NoError(t, db.Create(&OperationsEntry{EventKey: "usage:missing", RequestId: "missing", Kind: "wallet_usage", CreatedAt: 200}).Error)
	profit := 10.0
	report := &OperationsReport{Summary: OperationsReportSummary{ProfitUSD: &profit}, Rows: []OperationsReportRow{{ProfitUSD: &profit}}}
	require.NoError(t, checkOperationsReportCompleteness(report, nil, OperationsReportParams{StartTimestamp: 100, EndTimestamp: 300}))
	require.Nil(t, report.Summary.ProfitUSD)
	require.Nil(t, report.Rows[0].ProfitUSD)
	require.NotEmpty(t, report.Warnings)
}

func TestOperationsSettledSubscriptionCannotRefundReservation(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&UserSubscription{}, &SubscriptionPreConsumeRecord{}))
	old := DB
	DB = db
	t.Cleanup(func() { DB = old })
	require.NoError(t, db.Create(&UserSubscription{Id: 1, UserId: 1, AmountUsed: 800}).Error)
	require.NoError(t, db.Create(&SubscriptionPreConsumeRecord{RequestId: "settled", UserId: 1, UserSubscriptionId: 1, PreConsumed: 1000, Status: "settled"}).Error)
	require.NoError(t, RefundSubscriptionPreConsume("settled"))
	var sub UserSubscription
	require.NoError(t, db.First(&sub, 1).Error)
	require.Equal(t, int64(800), sub.AmountUsed)
}
