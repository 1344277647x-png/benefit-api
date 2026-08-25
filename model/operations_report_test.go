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

	"github.com/stretchr/testify/require"
)

func TestAggregateOperationsReportKeepsCostUnknown(t *testing.T) {
	day := time.Date(2026, time.January, 4, 12, 0, 0, 0, time.UTC).Unix()
	logs := []operationsReportLog{
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        day,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			Quota:            500000,
			PromptTokens:     100,
			CompletionTokens: 50,
			ChannelId:        3,
			Group:            "default",
			Other:            `{"upstream_cost_usd":0.12}`,
		},
		{
			UserId:           7,
			Username:         "alice",
			CreatedAt:        day + 10,
			Type:             LogTypeConsume,
			ModelName:        "gpt-test",
			Quota:            250000,
			PromptTokens:     20,
			CompletionTokens: 5,
			ChannelId:        3,
			Group:            "default",
		},
		{
			UserId:    7,
			Username:  "alice",
			CreatedAt: day + 20,
			Type:      LogTypeRefund,
			ModelName: "gpt-test",
			Quota:     100000,
			ChannelId: 3,
			Group:     "default",
		},
		{
			UserId:    7,
			Username:  "alice",
			CreatedAt: day + 30,
			Type:      LogTypeError,
			ModelName: "gpt-test",
			ChannelId: 3,
			Group:     "default",
		},
	}

	report := aggregateOperationsReport(logs, OperationsReportParams{
		StartTimestamp: day,
		EndTimestamp:   day + 60,
		GroupBy:        "model",
	})

	require.Len(t, report.Rows, 1)
	require.Equal(t, int64(3), report.Summary.RequestCount)
	require.Equal(t, int64(2), report.Summary.SuccessCount)
	require.Equal(t, int64(1), report.Summary.ErrorCount)
	require.InDelta(t, 33.3333, report.Summary.ErrorRate, 0.0001)
	require.Equal(t, int64(175), report.Summary.TokenCount)
	require.Equal(t, int64(750000), report.Summary.RevenueQuota)
	require.Equal(t, int64(100000), report.Summary.RefundQuota)
	require.Equal(t, "partial", report.Summary.CostStatus)
	require.Equal(t, int64(1), report.Summary.UnknownCostRequests)
	require.Nil(t, report.Summary.ProfitUSD)
	require.Equal(t, "gpt-test", report.Rows[0].ModelName)
	require.Equal(t, "partial", report.Rows[0].CostStatus)
	require.InDelta(t, 33.3333, report.Rows[0].ErrorRate, 0.0001)
}

func TestGetOperationsReportSQLiteFiltersReservedGroupColumn(t *testing.T) {
	start := time.Date(2026, time.February, 2, 0, 0, 0, 0, time.UTC).Unix()
	prefix := "operations-report-integration-"
	logs := []Log{
		{
			UserId:           91,
			Username:         "report-user",
			CreatedAt:        start + 10,
			Type:             LogTypeConsume,
			ModelName:        "report-model",
			Quota:            500000,
			PromptTokens:     10,
			CompletionTokens: 5,
			ChannelId:        901,
			Group:            "report-group",
			Other:            `{"provider_cost_usd":0.2}`,
			RequestId:        prefix + "consume",
		},
		{
			UserId:    91,
			Username:  "report-user",
			CreatedAt: start + 20,
			Type:      LogTypeRefund,
			ModelName: "report-model",
			Quota:     100000,
			ChannelId: 901,
			Group:     "report-group",
			RequestId: prefix + "refund",
		},
		{
			UserId:    91,
			Username:  "report-user",
			CreatedAt: start + 30,
			Type:      LogTypeError,
			ModelName: "report-model",
			ChannelId: 901,
			Group:     "report-group",
			RequestId: prefix + "error",
		},
	}
	require.NoError(t, LOG_DB.Create(&logs).Error)
	t.Cleanup(func() {
		require.NoError(t, LOG_DB.Where("request_id LIKE ?", prefix+"%").Delete(&Log{}).Error)
	})

	report, err := GetOperationsReport(OperationsReportParams{
		StartTimestamp: start,
		EndTimestamp:   start + 60,
		ModelName:      "report-model",
		Username:       "report-user",
		Group:          "report-group",
		ChannelID:      901,
		GroupBy:        "group",
	})
	require.NoError(t, err)
	require.False(t, report.Truncated)
	require.Len(t, report.Rows, 1)
	require.Equal(t, "report-group", report.Rows[0].Label)
	require.Equal(t, int64(2), report.Summary.RequestCount)
	require.Equal(t, int64(1), report.Summary.ErrorCount)
	require.InDelta(t, 50, report.Summary.ErrorRate, 0.0001)
	require.Equal(t, int64(100000), report.Summary.RefundQuota)
	require.Equal(t, "known", report.Summary.CostStatus)
}

func TestOperationReportCostValueRejectsInvalidValues(t *testing.T) {
	cost, known := operationReportCostValue(`{"cost_usd":0.25}`)
	require.True(t, known)
	require.Equal(t, 0.25, cost)

	_, known = operationReportCostValue(`{"cost_usd":-1}`)
	require.False(t, known)
	_, known = operationReportCostValue(`{"cost_usd":{"value":1}}`)
	require.False(t, known)
}
