package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamEpayMethodsAreSeparateFromNativeWalletGateways(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	previousAddress, previousId, previousKey, previousMethods := operation_setting.PayAddress,
		operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	previousAlipay, previousAppId := setting.AlipayEnabled, setting.AlipayAppId
	previousPrivate, previousPublic := setting.AlipayPrivateKey, setting.AlipayPublicKey
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods =
			previousAddress, previousId, previousKey, previousMethods
		setting.AlipayEnabled, setting.AlipayAppId = previousAlipay, previousAppId
		setting.AlipayPrivateKey, setting.AlipayPublicKey = previousPrivate, previousPublic
	})
	operation_setting.PayAddress = "https://local-payment.example.test"
	operation_setting.EpayId = "local-team-merchant"
	operation_setting.EpayKey = "local-test-only"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Epay Alipay"}}
	setting.AlipayEnabled = true
	setting.AlipayAppId, setting.AlipayPrivateKey, setting.AlipayPublicKey = "local-app", "local-private", "local-public"
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/user/topup/info", nil)
	GetTopUpInfo(context)
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			EpayMethods []map[string]string `json:"epay_pay_methods"`
			PayMethods  []map[string]string `json:"pay_methods"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &result))
	require.True(t, result.Success)
	require.Len(t, result.Data.EpayMethods, 1)
	assert.Equal(t, "alipay", result.Data.EpayMethods[0]["type"])
	require.Len(t, result.Data.PayMethods, 2)
	assert.Equal(t, model.PaymentMethodAlipayNative, result.Data.PayMethods[1]["type"])
}

func TestTeamPurchaseRoutesRejectWhenOperatorDisablesTeams(t *testing.T) {
	settings := config.GlobalConfig.Get("team_setting").(*operation_setting.TeamSetting)
	previous := settings.Enabled
	settings.Enabled = false
	t.Cleanup(func() { settings.Enabled = previous })
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/team/balance/pay", nil)
	RequireTeamsEnabled(context)
	assert.True(t, context.IsAborted())
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestTeamPurchaseRoutesRejectWhenConsumeLoggingDisabled(t *testing.T) {
	previous := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() { common.LogConsumeEnabled = previous })
	assert.False(t, model.TeamTaskLogDeliverySupported())
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/team/balance/pay", nil)
	RequireTeamsEnabled(context)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

func TestTeamEpayNotificationRejectsUnsignedAndAmbiguousParameters(t *testing.T) {
	previousAddress, previousId, previousKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress = "https://payment.example.test"
	operation_setting.EpayId = "local-test-merchant"
	operation_setting.EpayKey = "local-test-only-key"
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = previousAddress, previousId, previousKey
	})
	for _, tc := range []struct {
		name string
		form url.Values
	}{
		{"unsigned success", url.Values{"out_trade_no": {"nonexistent"}, "trade_status": {"TRADE_SUCCESS"}}},
		{"ambiguous trade number", url.Values{"out_trade_no": {"first", "second"}, "trade_status": {"TRADE_SUCCESS"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/api/team/epay/notify", strings.NewReader(tc.form.Encode()))
			context.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			TeamEpayNotify(context)
			assert.Equal(t, "fail", recorder.Body.String())
		})
	}
}

func TestTeamEpayBrowserReturnNeverCreditsAnUnsignedOrder(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/team/epay/return?out_trade_no=nonexistent&trade_status=TRADE_SUCCESS", nil)
	TeamEpayReturn(context)
	assert.Equal(t, http.StatusFound, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Location"), "/team?pay=fail")
}

func TestTeamEpaySignedCallbackMustMatchConfiguredMerchant(t *testing.T) {
	previousAddress, previousId, previousKey := operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey
	operation_setting.PayAddress = "https://payment.example.test"
	operation_setting.EpayId = "local-test-merchant"
	operation_setting.EpayKey = "local-test-only-key"
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey = previousAddress, previousId, previousKey
	})
	for _, tc := range []struct {
		name, merchant string
		valid          bool
	}{
		{"another merchant with the same signing key", "other-merchant", false},
		{"configured merchant", "local-test-merchant", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := epay.GenerateParams(map[string]string{
				"pid": tc.merchant, "out_trade_no": "local-order", "trade_no": "local-transaction",
				"money": "10.50", "type": "alipay", "trade_status": epay.StatusTradeSuccess,
				"sign_type": "MD5",
			}, operation_setting.EpayKey)
			form := url.Values{}
			for key, value := range params {
				form.Set(key, value)
			}
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/api/team/epay/notify", strings.NewReader(form.Encode()))
			context.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			result, err := verifiedTeamEpayResult(context)
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, tc.merchant, params["pid"])
				assert.Equal(t, "local-order", result.ServiceTradeNo)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestStatusDoesNotAdvertiseClosedTeamSubscriptions(t *testing.T) {
	settings := config.GlobalConfig.Get("team_setting").(*operation_setting.TeamSetting)
	previous := settings.Enabled
	settings.Enabled = false
	t.Cleanup(func() { settings.Enabled = previous })
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	GetStatus(context)
	var response struct {
		Data struct {
			TeamEnabled bool `json:"team_subscriptions_enabled"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Data.TeamEnabled)
}
