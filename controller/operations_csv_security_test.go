package controller

import (
	"encoding/csv"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOperationsCSVFormulaEscapingPreservesNumericColumns(t *testing.T) {
	for _, value := range []string{"=HYPERLINK(\"https://example.com\")", "+cmd", "-cmd", "@SUM(1)", " \t=1+1", "\r=1"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		profit := -2.0
		writeOperationsReportCSV(c, &model.OperationsReport{Rows: []model.OperationsReportRow{{
			Username: value, ChannelName: value, RevenueUSD: 3, ProfitUSD: &profit,
		}}})
		rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(recorder.Body.String(), "\ufeff"))).ReadAll()
		require.NoError(t, err)
		require.Len(t, rows, 2)
		assert.Equal(t, "'"+value, rows[1][6])
		assert.Equal(t, "'"+value, rows[1][4])
		assert.Equal(t, "-2.000000", rows[1][19])
	}
}
