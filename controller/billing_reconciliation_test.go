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
package controller

import (
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingReconciliationAndOpsAlertsEndpoints(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.BusinessEvent{}, &model.SubscriptionPreConsumeRecord{}, &model.OperationsEvent{}))

	oldAddr := system_setting.ServerAddress
	system_setting.ServerAddress = "http://localhost:3000"
	service.ResetOpsAlertsCacheForTest()
	t.Cleanup(func() {
		system_setting.ServerAddress = oldAddr
		service.ResetOpsAlertsCacheForTest()
	})

	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.User{
		Id: 77, Username: "=FORMULA_USER", Email: "u@example.test", Group: "default",
	}).Error)
	details := common.MapToJsonStr(map[string]any{
		"admin_info": map[string]any{
			"reserved_quota": 5000,
			"expected_quota": 12000,
			"failure_stage":  "funding_settlement_failed",
			"model":          "gpt-5",
		},
	})
	require.NoError(t, db.Create(&model.BusinessEvent{
		EventKey: "billing-exception:req-csv-1", Category: model.BusinessEventBilling, Action: "partial_settlement_failed",
		UserId: 77, RequestId: "req-csv-1", Content: "+cmd|summary", DetailsJSON: details, CreatedAt: now - 10,
	}).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/reconciliation", GetBillingReconciliation)
	r.GET("/ops-alerts", GetOpsAlerts)

	// 1. JSON API contract
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/reconciliation", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var apiResp struct {
		Success bool                              `json:"success"`
		Data    model.BillingReconciliationReport `json:"data"`
	}
	require.NoError(t, common.Unmarshal(rec.Body.Bytes(), &apiResp))
	require.True(t, apiResp.Success)
	assert.True(t, apiResp.Data.ReadOnly)
	require.Len(t, apiResp.Data.Items, 1)
	assert.EqualValues(t, 7000, apiResp.Data.Summary.KnownNetDiffQuota)

	// 2. JSON download header
	recDownload := httptest.NewRecorder()
	r.ServeHTTP(recDownload, httptest.NewRequest(http.MethodGet, "/reconciliation?download=true", nil))
	require.Equal(t, http.StatusOK, recDownload.Code)
	assert.Contains(t, recDownload.Header().Get("Content-Disposition"), "benefit-api-billing-reconciliation.json")

	// 3. CSV export escapes spreadsheet formulas in text fields while keeping numbers numeric
	recCSV := httptest.NewRecorder()
	r.ServeHTTP(recCSV, httptest.NewRequest(http.MethodGet, "/reconciliation?format=csv", nil))
	require.Equal(t, http.StatusOK, recCSV.Code)
	assert.Contains(t, recCSV.Header().Get("Content-Type"), "text/csv")
	body := strings.TrimPrefix(recCSV.Body.String(), "\xef\xbb\xbf")
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "funding_diff_quota", rows[0][13])
	assert.Equal(t, "token_diff_quota", rows[0][23])
	assert.Equal(t, "'=FORMULA_USER", rows[1][5])
	assert.Equal(t, "'+cmd|summary", rows[1][22])
	assert.Equal(t, "7000", rows[1][13])

	// 4. Invalid kind rejected
	recBad := httptest.NewRecorder()
	r.ServeHTTP(recBad, httptest.NewRequest(http.MethodGet, "/reconciliation?kind=drop_table", nil))
	assert.Contains(t, recBad.Body.String(), `"success":false`)

	// 5. Ops alerts endpoint returns structured JSON with pending billing exception alert
	recAlerts := httptest.NewRecorder()
	r.ServeHTTP(recAlerts, httptest.NewRequest(http.MethodGet, "/ops-alerts", nil))
	require.Equal(t, http.StatusOK, recAlerts.Code)
	var alertsResp struct {
		Success bool                      `json:"success"`
		Data    service.OpsAlertsSnapshot `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recAlerts.Body.Bytes(), &alertsResp))
	require.True(t, alertsResp.Success)
	assert.Equal(t, "not_configured", alertsResp.Data.TLS.Status)
	assert.EqualValues(t, 1, alertsResp.Data.Queues.BillingExceptionsCount)
	assert.Equal(t, "warning", alertsResp.Data.AlertLevel)
}
