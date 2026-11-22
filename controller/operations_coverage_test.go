package controller

import (
	"encoding/csv"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOperationsCostFilterParsesAndRejectsInvalidBoolean(t *testing.T) {
	for _, value := range []string{"true", "false", "invalid"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/?after_cost_effective="+value, nil)
		params, err := parseOperationsReportParams(c)
		if value == "invalid" {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, value == "true", params.AfterCostEffective)
	}
}

func TestOperationsCSVSeparatesPartialProfitAndCoverage(t *testing.T) {
	partial := -2.0
	r := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(r)
	writeOperationsReportCSV(c, &model.OperationsReport{AfterCostEffective: true, ExcludedRequests: 3, Rows: []model.OperationsReportRow{{
		OperationsCoverage: model.OperationsCoverage{MatchedRequests: 1, PendingRequests: 3, MatchedQuota: 10, PartialProfitUSD: &partial, Reasons: map[string]int64{"funding_source_unknown": 3}}, RequestCount: 4, RevenueQuota: 100,
	}}})
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(r.Body.String(), "\ufeff"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	fields := map[string]string{}
	for i, header := range rows[0] {
		fields[header] = rows[1][i]
	}
	require.Equal(t, "", fields["profit_usd"])
	require.Equal(t, "-2.000000", fields["partial_profit_usd"])
	require.Equal(t, "25.0000", fields["request_coverage_percent"])
	require.Equal(t, "10.0000", fields["quota_coverage_percent"])
	require.Equal(t, "true", fields["after_cost_effective"])
}
