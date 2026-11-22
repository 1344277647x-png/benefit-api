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
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetBillingReconciliationAndOpsQueueSummary(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&User{}, &BusinessEvent{}, &SubscriptionPreConsumeRecord{}, &OperationsEvent{}))
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}))

	oldQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

	now := common.GetTimestamp()
	require.NoError(t, DB.Create(&User{
		Id: 1188, Username: "viky", Password: "pw", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default",
	}).Error)

	// 1. Settlement failure with known reserved and expected quotas (undercharged by 70000).
	settleDetails := common.MapToJsonStr(map[string]any{
		"admin_info": map[string]any{
			"status":          "pending_manual_review",
			"funding_source":  "subscription",
			"subscription_id": 42,
			"model":           "gpt-5",
			"reserved_quota":  30000,
			"expected_quota":  100000,
			"failure_stage":   "funding_settlement_failed",
		},
	})
	require.NoError(t, DB.Create(&BusinessEvent{
		EventKey: "billing-exception:req-settle-1", Category: BusinessEventBilling, Action: "settlement_failed",
		UserId: 1188, RequestId: "req-settle-1", Content: "settlement failed", DetailsJSON: settleDetails,
		CreatedAt: now - 1000, DeadLetterAt: now - 500,
	}).Error)
	// Same request in subscription_pre_consume_records must NOT be double-counted.
	require.NoError(t, DB.Create(&SubscriptionPreConsumeRecord{
		RequestId: "req-settle-1", UserId: 1188, UserSubscriptionId: 42, PreConsumed: 30000, Status: "consumed",
	}).Error)

	// 2. Accounting projection failure must NOT distort known net diff.
	projDetails := common.MapToJsonStr(map[string]any{
		"admin_info": map[string]any{
			"status": "pending_accounting",
			"quota":  250000,
		},
	})
	require.NoError(t, DB.Create(&BusinessEvent{
		EventKey: "operations-pending:99", Category: BusinessEventBilling, Action: "accounting_projection_failed",
		UserId: 1188, RequestId: "req-proj-1", Content: "projection failed", DetailsJSON: projDetails,
		CreatedAt: now - 900,
	}).Error)

	// 3. Unsettled pre-consume WITH consume log -> known actual (over-reserved by 80000).
	recKnown := SubscriptionPreConsumeRecord{
		RequestId: "req-over-1", UserId: 1188, UserSubscriptionId: 42, PreConsumed: 100000, Status: "consumed",
	}
	require.NoError(t, DB.Create(&recKnown).Error)
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("id = ?", recKnown.Id).Update("created_at", now-800).Error)
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 1188, Username: "viky", Type: LogTypeConsume, RequestId: "req-over-1", ModelName: "claude-sonnet", Quota: 20000, CreatedAt: now - 800,
	}).Error)

	// 4. Unsettled pre-consume WITHOUT consume log -> actual stays unknown and separate from known net diff.
	recUnknown := SubscriptionPreConsumeRecord{
		RequestId: "req-unknown-1", UserId: 1188, UserSubscriptionId: 42, PreConsumed: 500000, Status: "consumed",
	}
	require.NoError(t, DB.Create(&recUnknown).Error)
	require.NoError(t, DB.Model(&SubscriptionPreConsumeRecord{}).Where("id = ?", recUnknown.Id).Update("created_at", now-700).Error)

	// 5. Pending operations event for queue summary check.
	require.NoError(t, DB.Create(&OperationsEvent{
		EventKey: "op-pending-1", Lane: "wallet:1188", RequestId: "req-proj-1", UserId: 1188, CreatedAt: now - 600, Done: false, Attempts: 2,
	}).Error)

	report, err := GetBillingReconciliation(t.Context(), BillingReconciliationParams{
		StartTimestamp: now - 3600,
		EndTimestamp:   now + 60,
	})
	require.NoError(t, err)
	assert.True(t, report.ReadOnly)
	assert.False(t, report.Truncated)
	assert.Equal(t, 4, report.Summary.TotalItems)
	assert.Equal(t, 1, report.Summary.SettlementExceptionCount)
	assert.Equal(t, 1, report.Summary.ProjectionFailureCount)
	assert.Equal(t, 2, report.Summary.UnsettledPreConsumeCount)
	assert.Equal(t, 1, report.Summary.UnknownActualCount)
	// Known totals: (30000 -> 100000, diff +70000) + (100000 -> 20000, diff -80000) = pre 130000, actual 120000, net -10000
	assert.EqualValues(t, 130000, report.Summary.KnownPreConsumedQuota)
	assert.EqualValues(t, 120000, report.Summary.KnownActualQuota)
	assert.EqualValues(t, -10000, report.Summary.KnownNetDiffQuota)
	// Unknown actual pre-consume must stay isolated.
	assert.EqualValues(t, 500000, report.Summary.UnknownPreConsumedQuota)
	assert.InDelta(t, 1.0, report.Summary.UnknownPreConsumedUSD, 1e-9)

	byReq := make(map[string]BillingReconciliationItem, len(report.Items))
	for _, item := range report.Items {
		byReq[item.RequestId] = item
		assert.Equal(t, "viky", item.Username)
	}
	require.NotNil(t, byReq["req-settle-1"].DiffQuota)
	assert.EqualValues(t, 70000, *byReq["req-settle-1"].DiffQuota)
	assert.NotZero(t, byReq["req-settle-1"].DeadLetterAt)

	assert.Nil(t, byReq["req-proj-1"].ActualQuota)
	require.NotNil(t, byReq["req-proj-1"].ProjectedQuota)
	assert.EqualValues(t, 250000, *byReq["req-proj-1"].ProjectedQuota)

	require.NotNil(t, byReq["req-over-1"].DiffQuota)
	assert.True(t, byReq["req-over-1"].HasConsumeLog)
	assert.EqualValues(t, -80000, *byReq["req-over-1"].DiffQuota)
	assert.Equal(t, "claude-sonnet", byReq["req-over-1"].ModelName)

	assert.Nil(t, byReq["req-unknown-1"].ActualQuota)
	assert.Nil(t, byReq["req-unknown-1"].DiffQuota)
	assert.False(t, byReq["req-unknown-1"].HasConsumeLog)

	// Filter by kind and request_id.
	filtered, err := GetBillingReconciliation(t.Context(), BillingReconciliationParams{
		StartTimestamp: now - 3600,
		EndTimestamp:   now + 60,
		Kind:           ReconciliationKindUnsettledPreConsume,
		RequestID:      "req-unknown-1",
	})
	require.NoError(t, err)
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, "req-unknown-1", filtered.Items[0].RequestId)

	// Queue summary reflects pending ops, dead letters, and unsettled pre-consumes older than grace window.
	queues, err := GetOpsQueueSummary(t.Context())
	require.NoError(t, err)
	assert.EqualValues(t, 1, queues.OperationsPending)
	assert.EqualValues(t, 1, queues.OperationsRetrying)
	assert.EqualValues(t, now-600, queues.OperationsOldestPendingAt)
	assert.EqualValues(t, 1, queues.BusinessPending)
	assert.EqualValues(t, 1, queues.BusinessDeadLetter)
	assert.EqualValues(t, 2, queues.BillingExceptionsCount)
	assert.EqualValues(t, 2, queues.UnsettledPreConsumeCount)

	// Sanity check: query never modified underlying pre-consume status.
	var persisted SubscriptionPreConsumeRecord
	require.NoError(t, DB.First(&persisted, "request_id = ?", "req-unknown-1").Error)
	assert.Equal(t, "consumed", persisted.Status)
	_ = time.Second
}

func TestGetBillingReconciliationTokenOnlyFallback(t *testing.T) {
	truncateTables(t)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	conn, err := logDB.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	old := LOG_DB
	LOG_DB = logDB
	t.Cleanup(func() { LOG_DB = old; _ = conn.Close() })
	require.NoError(t, LOG_DB.AutoMigrate(&Log{}, &BusinessEventLogReceipt{}))
	now := common.GetTimestamp()
	event := BusinessEvent{EventKey: "billing-exception:token-only", Category: BusinessEventBilling, Action: ReconciliationKindSettlementFailed, RequestId: "token-only", UserId: 9999, CreatedAt: now - 1,
		DetailsJSON: common.MapToJsonStr(map[string]any{"admin_info": map[string]any{"reserved_quota": 30, "expected_quota": 100, "token_reserved_quota": 30, "failure_stage": "token_adjustment_failed"}})}
	require.NoError(t, DeliverBusinessEvent(t.Context(), event))
	params := BillingReconciliationParams{StartTimestamp: now - 60, EndTimestamp: now, RequestID: event.RequestId}
	result, err := GetBillingReconciliation(t.Context(), params)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "billing_log", result.Items[0].Source)
	require.NotNil(t, result.Items[0].DiffQuota)
	assert.Zero(t, *result.Items[0].DiffQuota)
	require.NotNil(t, result.Items[0].TokenDiffQuota)
	assert.EqualValues(t, 70, *result.Items[0].TokenDiffQuota)
	assert.Zero(t, result.Summary.KnownNetDiffQuota)

	require.NoError(t, DB.Create(&event).Error)
	result, err = GetBillingReconciliation(t.Context(), params)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "business_event", result.Items[0].Source)
}
