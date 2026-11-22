package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationsProjectionAlertDoesNotCountAsRequest(t *testing.T) {
	logs := []operationsReportLog{
		{Type: LogTypeConsume, RequestId: "same-request", ChannelId: 1, Quota: 100, Other: `{"operations_income_usd":1,"operations_income_known":true,"upstream_cost_usd":0.2}`},
		{Type: LogTypeError, RequestId: "same-request", Other: `{"business_event":{"category":"billing","action":"accounting_projection_failed"},"admin_info":{"status":"pending_accounting"}}`},
	}
	report := aggregateOperationsReport(logs, OperationsReportParams{GroupBy: "channel"})
	assert.Equal(t, int64(1), report.Summary.RequestCount)
	assert.Zero(t, report.Summary.ErrorCount)
	assert.Zero(t, report.Summary.ErrorRate)
	assert.Equal(t, int64(1), report.Summary.MatchedRequests)
	assert.Zero(t, report.Summary.PendingRequests)
	require.Len(t, report.Rows, 1)
	assert.Equal(t, 1, report.Rows[0].ChannelID)
	require.NotNil(t, report.Summary.PartialProfitUSD)
	assert.InDelta(t, 0.8, *report.Summary.PartialProfitUSD, 1e-9)
	// A real relay failure remains a request, even without channel metadata.
	logs = append(logs, operationsReportLog{Type: LogTypeError, RequestId: "relay-failure", Other: `{}`})
	report = aggregateOperationsReport(logs, OperationsReportParams{})
	assert.Equal(t, int64(2), report.Summary.RequestCount)
	assert.Equal(t, int64(1), report.Summary.ErrorCount)
	assert.Equal(t, int64(1), report.Summary.PendingRequests)
}

func TestOperationsFilteredReportsRetainCompletenessGuard(t *testing.T) {
	old := DB
	DB = operationsTestDB(t)
	t.Cleanup(func() { DB = old })
	event := OperationsEvent{EventKey: "pending-report", Lane: "wallet:1", UserId: 1, RequestId: "unlogged", CreatedAt: 200}
	require.NoError(t, DB.Create(&event).Error)
	filters := []OperationsReportParams{
		{}, {AfterCostEffective: true}, {ModelName: "model"},
		{Username: "user"}, {Group: "group"}, {ChannelID: 1},
	}
	for _, params := range filters {
		params.StartTimestamp, params.EndTimestamp = 100, 300
		profit := 9.0
		report := &OperationsReport{
			Summary: OperationsReportSummary{ProfitUSD: &profit, OperationsCoverage: OperationsCoverage{PartialProfitUSD: &profit}},
			Rows:    []OperationsReportRow{{ProfitUSD: &profit}},
		}
		require.NoError(t, checkOperationsReportCompleteness(report, nil, params))
		assert.Nil(t, report.Summary.ProfitUSD, "filter: %+v", params)
		assert.Nil(t, report.Rows[0].ProfitUSD)
		assert.Equal(t, "partial", report.Summary.RevenueStatus)
		assert.NotEmpty(t, report.Warnings)
		assert.Equal(t, &profit, report.Summary.PartialProfitUSD)
	}
	// Once pending work is done, a complete filtered view is not permanently
	// disabled. An unmatched journal entry must still suppress full profit.
	require.NoError(t, DB.Model(&event).Update("done", true).Error)
	params := OperationsReportParams{StartTimestamp: 100, EndTimestamp: 300, AfterCostEffective: true}
	profit := 9.0
	report := &OperationsReport{Summary: OperationsReportSummary{ProfitUSD: &profit}}
	require.NoError(t, checkOperationsReportCompleteness(report, nil, params))
	assert.NotNil(t, report.Summary.ProfitUSD)
	assert.Empty(t, report.Warnings)
	require.NoError(t, DB.Create(&OperationsEntry{EventKey: "missing-log", RequestId: "missing-log", Kind: "wallet_usage", CreatedAt: 200}).Error)
	require.NoError(t, checkOperationsReportCompleteness(report, nil, params))
	assert.Nil(t, report.Summary.ProfitUSD)
	assert.NotEmpty(t, report.Warnings)
}

func TestOperationsReportProjectionAlertAndPendingEventIntegration(t *testing.T) {
	db := operationsTestDB(t)
	require.NoError(t, db.AutoMigrate(&OperationsState{}, &Log{}))
	oldDB, oldLog := DB, LOG_DB
	DB, LOG_DB = db, db
	t.Cleanup(func() { DB, LOG_DB = oldDB, oldLog })
	require.NoError(t, db.Create(&OperationsState{Id: 1, StartedAt: 100}).Error)
	require.NoError(t, db.Create(&ChannelCostVersion{ChannelId: 1, EffectiveAt: 100, Multiplier: "0.1"}).Error)
	require.NoError(t, db.Create(&OperationsEntry{EventKey: "known", RequestId: "known", Kind: "wallet_usage", UserId: 1, Quota: 100, ValueUSD: "1", UnknownQuota: "0", CreatedAt: 200}).Error)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 1, Type: LogTypeConsume, RequestId: "known", ChannelId: 1, Quota: 100, CreatedAt: 200, Other: `{"operations_request_started_at":150,"upstream_cost_usd":0.2}`},
		{UserId: 1, Type: LogTypeError, RequestId: "known", CreatedAt: 201, Other: `{"business_event":{"category":"billing","action":"accounting_projection_failed"}}`},
	}).Error)
	params := OperationsReportParams{StartTimestamp: 100, EndTimestamp: 300, AfterCostEffective: true}
	report, err := GetOperationsReport(params)
	require.NoError(t, err)
	assert.Equal(t, int64(1), report.Summary.RequestCount)
	assert.Zero(t, report.Summary.ErrorCount)
	assert.Zero(t, report.ExcludedRequests, "background alerts are not cost-filter exclusions")
	require.NotNil(t, report.Summary.ProfitUSD)
	assert.InDelta(t, 0.8, *report.Summary.ProfitUSD, 1e-9)
	require.NoError(t, db.Create(&OperationsEvent{EventKey: "pending", Lane: "wallet:1", UserId: 1, CreatedAt: 210}).Error)
	report, err = GetOperationsReport(params)
	require.NoError(t, err)
	assert.Nil(t, report.Summary.ProfitUSD)
	assert.NotEmpty(t, report.Warnings)
	require.NotNil(t, report.Summary.PartialProfitUSD)
	assert.InDelta(t, 0.8, *report.Summary.PartialProfitUSD, 1e-9)
	var count int64
	require.NoError(t, db.Model(&Log{}).Where("type = ?", LogTypeError).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the administrator's alert must remain stored")
}
