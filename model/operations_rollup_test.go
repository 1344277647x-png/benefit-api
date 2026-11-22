/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOperationsHourlyRollupExactParityAndHybridWindow(t *testing.T) {
	db := operationsTestDB(t)
	oldDB, oldLogDB := DB, LOG_DB
	DB, LOG_DB = db, db
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLogDB })
	require.NoError(t, db.AutoMigrate(&OperationsState{}, &OperationsRollupBucket{}, &OperationsHourlyRollup{}, &Log{}, &OperationsRollupDirtyHour{}))

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	// Use two closed hours strictly in the past plus a trailing partial third hour.
	baseHour := floorOperationsHour(common.GetTimestamp() - 5*3600)
	hour1 := baseHour
	hour2 := baseHour + 3600
	hour3 := baseHour + 2*3600

	require.NoError(t, DB.Create(&OperationsState{Id: 1, StartedAt: hour1}).Error)
	require.NoError(t, DB.Create(&ChannelCostVersion{
		ChannelId: 10, Multiplier: "0.5", EffectiveAt: hour1 + 100, CreatedAt: hour1, OperatorId: 1,
	}).Error)

	// Insert logs across 3 hours: matched consume, unmatched consume (missing cost), error log, refund log, and projection alert (must be ignored).
	logs := []Log{
		{
			UserId: 1, Username: "alice", CreatedAt: hour1 + 200, Type: LogTypeConsume,
			ModelName: "gpt-5", Quota: 500000, PromptTokens: 100, CompletionTokens: 50,
			ChannelId: 10, Group: "default", RequestId: "req-h1-1",
			Other: common.MapToJsonStr(map[string]any{"operations_request_started_at": hour1 + 190}),
		},
		{
			UserId: 2, Username: "bob", CreatedAt: hour1 + 500, Type: LogTypeConsume,
			ModelName: "claude-sonnet", Quota: 250000, PromptTokens: 40, CompletionTokens: 20,
			ChannelId: 20, Group: "vip", RequestId: "req-h1-2",
			Other: "{}",
		},
		{
			UserId: 2, Username: "bob", CreatedAt: hour1 + 600, Type: LogTypeError,
			ModelName: "claude-sonnet", Quota: 0, ChannelId: 20, Group: "vip", RequestId: "req-h1-err",
			Other: "{}",
		},
		{
			UserId: 1, Username: "alice", CreatedAt: hour1 + 700, Type: LogTypeError,
			ModelName: "", Quota: 0, ChannelId: 0, Group: "", RequestId: "req-proj-alert",
			Other: common.MapToJsonStr(map[string]any{"business_event": map[string]any{"category": BusinessEventBilling, "action": "accounting_projection_failed"}}),
		},
		{
			UserId: 1, Username: "alice", CreatedAt: hour2 + 300, Type: LogTypeConsume,
			ModelName: "gpt-5", Quota: 1000000, PromptTokens: 200, CompletionTokens: 100,
			ChannelId: 10, Group: "default", RequestId: "req-h2-1",
			Other: common.MapToJsonStr(map[string]any{"operations_request_started_at": hour2 + 290}),
		},
		{
			UserId: 1, Username: "alice", CreatedAt: hour3 + 100, Type: LogTypeConsume,
			ModelName: "gpt-5", Quota: 500000, PromptTokens: 80, CompletionTokens: 20,
			ChannelId: 10, Group: "default", RequestId: "req-h3-1",
			Other: common.MapToJsonStr(map[string]any{"operations_request_started_at": hour3 + 90}),
		},
	}
	for i := range logs {
		require.NoError(t, LOG_DB.Create(&logs[i]).Error)
	}

	// Matching journal entries for all consume logs.
	entries := []OperationsEntry{
		{EventKey: "e-h1-1", PoolKey: "wallet:1", UserId: 1, RequestId: "req-h1-1", Kind: "wallet_usage", Quota: 500000, ValueUSD: "1.0", UnknownQuota: "0", CreatedAt: hour1 + 200},
		{EventKey: "e-h1-2", PoolKey: "wallet:2", UserId: 2, RequestId: "req-h1-2", Kind: "wallet_usage", Quota: 250000, ValueUSD: "0.5", UnknownQuota: "0", CreatedAt: hour1 + 500},
		{EventKey: "e-h2-1", PoolKey: "wallet:1", UserId: 1, RequestId: "req-h2-1", Kind: "wallet_usage", Quota: 1000000, ValueUSD: "2.0", UnknownQuota: "0", CreatedAt: hour2 + 300},
		{EventKey: "e-h3-1", PoolKey: "wallet:1", UserId: 1, RequestId: "req-h3-1", Kind: "wallet_usage", Quota: 500000, ValueUSD: "1.0", UnknownQuota: "0", CreatedAt: hour3 + 100},
	}
	for i := range entries {
		require.NoError(t, DB.Create(&entries[i]).Error)
	}

	// Capture baseline raw-log reports across all 5 group_by modes BEFORE sealing rollups.
	windowStart := hour1
	windowEnd := hour3 + 600 // partial hour3 so hybrid rollup + raw-log merge is exercised!
	groupModes := []string{"day", "model", "channel", "user", "group"}
	rawByMode := make(map[string]*OperationsReport, len(groupModes))
	for _, mode := range groupModes {
		rep, err := GetOperationsReport(OperationsReportParams{
			StartTimestamp: windowStart,
			EndTimestamp:   windowEnd,
			GroupBy:        mode,
		})
		require.NoError(t, err)
		assert.False(t, rep.PreAggregated)
		rawByMode[mode] = rep
	}

	// Seal closed hours via ProcessOperationsHourlyRollups.
	require.NoError(t, ProcessOperationsHourlyRollups(t.Context(), 10))
	var sealedCount int64
	require.NoError(t, DB.Model(&OperationsRollupBucket{}).Count(&sealedCount).Error)
	assert.GreaterOrEqual(t, sealedCount, int64(2))

	// Query again: must use rollups for hour1 & hour2 + raw logs for partial hour3, matching baseline exactly.
	for _, mode := range groupModes {
		t.Run("parity_"+mode, func(t *testing.T) {
			got, err := GetOperationsReport(OperationsReportParams{
				StartTimestamp: windowStart,
				EndTimestamp:   windowEnd,
				GroupBy:        mode,
			})
			require.NoError(t, err)
			assert.True(t, got.PreAggregated)
			assert.Equal(t, 2, got.RollupHoursUsed)
			want := rawByMode[mode]
			assert.Equal(t, want.Summary.RequestCount, got.Summary.RequestCount)
			assert.Equal(t, want.Summary.SuccessCount, got.Summary.SuccessCount)
			assert.Equal(t, want.Summary.ErrorCount, got.Summary.ErrorCount)
			assert.Equal(t, want.Summary.TokenCount, got.Summary.TokenCount)
			assert.Equal(t, want.Summary.RevenueQuota, got.Summary.RevenueQuota)
			assert.InDelta(t, want.Summary.RevenueUSD, got.Summary.RevenueUSD, 1e-9)
			assert.InDelta(t, want.Summary.CostUSD, got.Summary.CostUSD, 1e-9)
			assert.Equal(t, want.Summary.MatchedRequests, got.Summary.MatchedRequests)
			assert.Equal(t, want.Summary.PendingRequests, got.Summary.PendingRequests)
			assert.Equal(t, want.Summary.MatchedQuota, got.Summary.MatchedQuota)
			assert.Equal(t, want.Summary.Reasons, got.Summary.Reasons)
			assert.Equal(t, want.ChannelIDs, got.ChannelIDs)
			require.Len(t, got.Rows, len(want.Rows))
			for i := range want.Rows {
				assert.Equal(t, want.Rows[i].Key, got.Rows[i].Key)
				assert.Equal(t, want.Rows[i].RequestCount, got.Rows[i].RequestCount)
				assert.Equal(t, want.Rows[i].RevenueQuota, got.Rows[i].RevenueQuota)
				assert.InDelta(t, want.Rows[i].RevenueUSD, got.Rows[i].RevenueUSD, 1e-9)
				assert.InDelta(t, want.Rows[i].CostUSD, got.Rows[i].CostUSD, 1e-9)
			}
		})
	}

	// Verify AfterCostEffective filter and ExcludedRequests parity on pre-aggregated path.
	afterGot, err := GetOperationsReport(OperationsReportParams{
		StartTimestamp:     windowStart,
		EndTimestamp:       windowEnd,
		GroupBy:            "channel",
		AfterCostEffective: true,
	})
	require.NoError(t, err)
	assert.True(t, afterGot.PreAggregated)
	assert.EqualValues(t, 3, afterGot.Summary.RequestCount)
	assert.EqualValues(t, 2, afterGot.ExcludedRequests)

	// Verify pending OperationsEvent prevents sealing an hour.
	hour4 := baseHour - 3600
	require.NoError(t, DB.Create(&OperationsEvent{
		EventKey: "pending-h4", Lane: "wallet:1", RequestId: "req-h4", UserId: 1, CreatedAt: hour4 + 10, Done: false,
	}).Error)
	require.NoError(t, RollupOperationsHour(t.Context(), hour4))
	var h4Buckets int64
	require.NoError(t, DB.Model(&OperationsRollupBucket{}).Where("hour_bucket = ?", hour4).Count(&h4Buckets).Error)
	assert.Zero(t, h4Buckets)
}

func TestOperationsHourlyRollupLateLogOnSeparateDatabase(t *testing.T) {
	primary, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, db := range []*gorm.DB{primary, logDB} {
		conn, err := db.DB()
		require.NoError(t, err)
		conn.SetMaxOpenConns(1)
		t.Cleanup(func() { _ = conn.Close() })
	}
	oldDB, oldLogDB := DB, LOG_DB
	DB, LOG_DB = primary, logDB
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLogDB })
	require.NoError(t, DB.AutoMigrate(&User{}, &OperationsState{}, &OperationsEntry{}, &OperationsEvent{}, &ChannelCostVersion{}, &OperationsRollupBucket{}, &OperationsHourlyRollup{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}, &OperationsRollupDirtyHour{}, &BusinessEventLogReceipt{}))

	hour := floorOperationsHour(common.GetTimestamp() - 4*3600)
	require.NoError(t, DB.Create(&OperationsState{Id: 1, StartedAt: hour}).Error)
	require.NoError(t, LOG_DB.Create(&Log{CreatedAt: hour + 10, Type: LogTypeError, RequestId: "original"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{CreatedAt: hour + 3610, Type: LogTypeError, RequestId: "blocked"}).Error)
	require.NoError(t, LOG_DB.Create(&Log{CreatedAt: hour + 7210, Type: LogTypeError, RequestId: "later"}).Error)
	require.NoError(t, DB.Create(&OperationsEvent{EventKey: "pending-rollup", Lane: "wallet:1", CreatedAt: hour + 3610}).Error)

	require.NoError(t, ProcessOperationsHourlyRollups(t.Context(), 4))
	var initial, later OperationsRollupBucket
	require.NoError(t, DB.First(&initial, "hour_bucket = ?", hour).Error)
	require.NoError(t, DB.First(&later, "hour_bucket = ?", hour+7200).Error)
	assert.NotEmpty(t, later.BuildKey, "a pending middle hour must not block later hours")
	var state OperationsState
	require.NoError(t, DB.First(&state, 1).Error)
	assert.GreaterOrEqual(t, state.RollupCursor, hour+3*3600)

	event := BusinessEvent{EventKey: "billing-exception:late", Category: BusinessEventBilling, Action: ReconciliationKindSettlementFailed, UserId: 1, RequestId: "late", CreatedAt: hour + 20}
	require.NoError(t, DeliverBusinessEvent(t.Context(), event))
	require.NoError(t, DeliverBusinessEvent(t.Context(), event))
	report, err := GetOperationsReport(OperationsReportParams{StartTimestamp: hour, EndTimestamp: hour + 3599})
	require.NoError(t, err)
	assert.False(t, report.PreAggregated)
	assert.EqualValues(t, 2, report.Summary.ErrorCount)

	require.NoError(t, ProcessOperationsHourlyRollups(t.Context(), 4))
	report, err = GetOperationsReport(OperationsReportParams{StartTimestamp: hour, EndTimestamp: hour + 3599})
	require.NoError(t, err)
	assert.True(t, report.PreAggregated)
	assert.EqualValues(t, 2, report.Summary.ErrorCount)
	var after, untouched OperationsRollupBucket
	require.NoError(t, DB.First(&after, "hour_bucket = ?", hour).Error)
	require.NoError(t, DB.First(&untouched, "hour_bucket = ?", hour+7200).Error)
	assert.NotEqual(t, initial.BuildKey, after.BuildKey)
	assert.Equal(t, later.BuildKey, untouched.BuildKey)

	require.NoError(t, createLog(&Log{CreatedAt: hour + 30, Type: LogTypeError, RequestId: "before-rebuild"}))
	const callback = "test:late-log-during-rollup"
	injected := false
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "operations_entries" && !injected {
			injected = true
			tx.AddError(createLog(&Log{CreatedAt: hour + 40, Type: LogTypeError, RequestId: "during-rebuild"}))
		}
	}))
	require.NoError(t, ProcessOperationsHourlyRollups(t.Context(), 4))
	require.NoError(t, DB.Callback().Query().Remove(callback))
	require.True(t, injected)
	report, err = GetOperationsReport(OperationsReportParams{StartTimestamp: hour, EndTimestamp: hour + 3599})
	require.NoError(t, err)
	assert.False(t, report.PreAggregated, "publishing an older snapshot must not clear a newer late-log marker")
	assert.EqualValues(t, 4, report.Summary.ErrorCount)
	require.NoError(t, ProcessOperationsHourlyRollups(t.Context(), 4))
	report, err = GetOperationsReport(OperationsReportParams{StartTimestamp: hour, EndTimestamp: hour + 3599})
	require.NoError(t, err)
	assert.True(t, report.PreAggregated)
	assert.EqualValues(t, 4, report.Summary.ErrorCount)
}
