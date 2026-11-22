package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOperationsCoverageNeverSubtractsUnmatchedTotals(t *testing.T) {
	logs := []operationsReportLog{
		{Type: LogTypeConsume, Quota: 100, Other: `{"operations_income_usd":9.21875,"operations_income_known":true,"upstream_cost_usd":2}`},
		{Type: LogTypeConsume, Quota: 900, Other: `{"operations_income_usd":100,"operations_income_known":true}`},
		{Type: LogTypeConsume, Quota: 1000, Other: `{"upstream_cost_usd":50}`},
		{Type: LogTypeError, RequestId: "failed", Other: `{}`},
	}
	r := aggregateOperationsReport(logs, OperationsReportParams{})
	require.Equal(t, int64(1), r.Summary.MatchedRequests)
	require.Equal(t, int64(3), r.Summary.PendingRequests)
	require.Equal(t, int64(100), r.Summary.MatchedQuota)
	require.Equal(t, int64(2000), r.Summary.RevenueQuota)
	require.InDelta(t, 7.21875, *r.Summary.PartialProfitUSD, 0.000001)
	require.Nil(t, r.Summary.ProfitUSD)
	require.Equal(t, int64(1), r.Summary.ErrorsWithoutQuota)
}

func TestOperationsCoverageUnknownAndZeroIncome(t *testing.T) {
	unknown := aggregateOperationsReport([]operationsReportLog{{Type: LogTypeConsume, Quota: 1}}, OperationsReportParams{})
	require.Nil(t, unknown.Summary.PartialProfitUSD)
	zero := aggregateOperationsReport([]operationsReportLog{{Type: LogTypeConsume, Quota: 1, Other: `{"operations_income_usd":0,"operations_income_known":true,"upstream_cost_usd":2}`}}, OperationsReportParams{})
	require.Equal(t, -2.0, *zero.Summary.PartialProfitUSD)
}

func TestOperationsCoverageEffectiveFilterAndVersionBoundary(t *testing.T) {
	oldDB := DB
	DB = operationsTestDB(t)
	t.Cleanup(func() { DB = oldDB })
	require.NoError(t, DB.Create(&ChannelCostVersion{ChannelId: 1, EffectiveAt: 100, Multiplier: "0.1"}).Error)
	require.NoError(t, DB.Create(&ChannelCostVersion{ChannelId: 1, EffectiveAt: 200, Multiplier: "0.2"}).Error)
	logs := []operationsReportLog{
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: 250, Quota: 100, Other: `{"operations_request_started_at":99,"group_ratio":1}`},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: 250, Quota: 100, Other: `{"operations_request_started_at":100,"group_ratio":1}`},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: 250, Quota: 100, Other: `{"operations_request_started_at":200,"group_ratio":1}`},
		{ChannelId: 2, Type: LogTypeConsume, CreatedAt: 250, Other: `{"operations_request_started_at":200}`},
		{ChannelId: 1, Type: LogTypeConsume, CreatedAt: 250, Other: `{}`},
	}
	require.NoError(t, enrichOperationsAccounting(logs, OperationsReportParams{EndTimestamp: 300}))
	require.Equal(t, "before_cost_effective", logs[0].CostReason)
	require.Equal(t, "cost_version_missing", logs[3].CostReason)
	require.Equal(t, "request_start_missing", logs[4].CostReason)
	cost1, ok := operationReportCostValue(logs[1].Other)
	require.True(t, ok)
	cost2, ok := operationReportCostValue(logs[2].Other)
	require.True(t, ok)
	require.InDelta(t, 100/common.QuotaPerUnit*0.1, cost1, 1e-10)
	require.InDelta(t, cost1*2, cost2, 1e-10)
	r := aggregateOperationsReport(logs, OperationsReportParams{AfterCostEffective: true})
	require.Equal(t, int64(2), r.Summary.RequestCount)
}

func TestOperationsPendingSamplesAreBoundedButCountsAreComplete(t *testing.T) {
	logs := make([]operationsReportLog, 25)
	for i := range logs {
		logs[i] = operationsReportLog{Type: LogTypeConsume, IncomeReason: "funding_source_unknown", CostReason: "retry_cost_unknown"}
	}
	r := aggregateOperationsReport(logs, OperationsReportParams{})
	require.Len(t, r.Summary.PendingSamples, 20)
	require.Equal(t, int64(25), r.Summary.Reasons["funding_source_unknown"])
	require.Equal(t, int64(25), r.Summary.Reasons["retry_cost_unknown"])
}
