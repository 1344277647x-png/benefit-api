package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTransferAffQuotaRequiresPaymentCompliance(t *testing.T) {
	payment := operation_setting.GetPaymentSetting()
	original := *payment
	payment.ComplianceConfirmed = false
	payment.ComplianceTermsVersion = ""
	t.Cleanup(func() { *payment = original })

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/user/referral/transfer", bytes.NewBufferString(`{"quota":100}`))
	context.Request.Header.Set("Content-Type", "application/json")
	context.Set("id", 901)

	TransferAffQuota(context)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success)
}

func TestValidateReferralOptionRejectsUnsafeValues(t *testing.T) {
	require.Error(t, validateReferralOption("referral_setting.minimum_topup_quota", "-1"))
	require.Error(t, validateReferralOption("referral_setting.reward_rate_basis_points", "10001"))
	require.Error(t, validateReferralOption("referral_setting.settlement_delay_hours", "721"))
	require.NoError(t, validateReferralOption("referral_setting.reward_rate_basis_points", "300"))
}
