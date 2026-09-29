package controller

import (
	"bytes"
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
	"github.com/QuantumNous/new-api/setting/ratio_setting"
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

func TestTeamTokenCreatePersistsSelectedGroupAndModelLimits(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Team{}, &model.TeamMember{}, &model.TeamSubscription{}, &model.Token{}))

	previousGroups := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"CodexPlus":"Plus","CodexPro":"Pro"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"CodexPlus":1,"CodexPro":1}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
	})

	owner := model.User{Username: "team-token-owner", Group: "CodexPlus", Status: common.UserStatusEnabled, AffCode: common.GetRandomString(12)}
	require.NoError(t, db.Create(&owner).Error)
	team := model.Team{OwnerId: owner.Id, Name: "Token team", Status: model.TeamStatusActive}
	require.NoError(t, db.Create(&team).Error)
	require.NoError(t, db.Create(&model.TeamMember{TeamId: team.Id, UserId: owner.Id, Role: "owner"}).Error)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.TeamSubscription{TeamId: team.Id, PlanTitle: "Active", SeatLimit: 2,
		AmountTotal: 1000, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "CodexPlus", Model: "gpt-5-codex", ChannelId: 1, Enabled: true},
		{Group: "CodexPro", Model: "gpt-5-pro", ChannelId: 2, Enabled: true},
	}).Error)

	body, err := common.Marshal(map[string]any{"name": "Plus key", "group": "CodexPlus", "model_limits": "gpt-5-codex"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", owner.Id)
	context.Set("group", owner.Group)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/team/tokens", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	TeamCreateToken(context)

	var created struct {
		Success bool `json:"success"`
		Data    struct {
			Id          int      `json:"id"`
			Group       string   `json:"group"`
			ModelLimits []string `json:"model_limits"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &created))
	require.True(t, created.Success)
	assert.Equal(t, "CodexPlus", created.Data.Group)
	assert.Equal(t, []string{"gpt-5-codex"}, created.Data.ModelLimits)

	var token model.Token
	require.NoError(t, db.First(&token, created.Data.Id).Error)
	assert.Equal(t, "CodexPlus", token.Group)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, "gpt-5-codex", token.ModelLimits)

	listRecorder := httptest.NewRecorder()
	listContext, _ := gin.CreateTestContext(listRecorder)
	listContext.Set("id", owner.Id)
	listContext.Set("group", owner.Group)
	listContext.Request = httptest.NewRequest(http.MethodGet, "/api/team/tokens", nil)
	TeamListTokens(listContext)
	var listed struct {
		Success bool `json:"success"`
		Data    []struct {
			Group              string   `json:"group"`
			ModelLimitsEnabled bool     `json:"model_limits_enabled"`
			ModelLimits        []string `json:"model_limits"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(listRecorder.Body.Bytes(), &listed))
	require.True(t, listed.Success)
	require.Len(t, listed.Data, 1)
	assert.Equal(t, "CodexPlus", listed.Data[0].Group)
	assert.True(t, listed.Data[0].ModelLimitsEnabled)
	assert.Equal(t, []string{"gpt-5-codex"}, listed.Data[0].ModelLimits)
}

func TestTeamTokenCreateRejectsModelOutsideSelectedGroup(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Team{}, &model.TeamMember{}, &model.TeamSubscription{}, &model.Token{}))

	previousGroups := setting.UserUsableGroups2JSONString()
	previousRatios := ratio_setting.GroupRatio2JSONString()
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"CodexPlus":"Plus","CodexPro":"Pro"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"CodexPlus":1,"CodexPro":1}`))
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousRatios))
	})

	owner := model.User{Username: "team-token-rejected", Group: "CodexPlus", Status: common.UserStatusEnabled, AffCode: common.GetRandomString(12)}
	require.NoError(t, db.Create(&owner).Error)
	team := model.Team{OwnerId: owner.Id, Name: "Rejected token team", Status: model.TeamStatusActive}
	require.NoError(t, db.Create(&team).Error)
	require.NoError(t, db.Create(&model.TeamMember{TeamId: team.Id, UserId: owner.Id, Role: "owner"}).Error)
	now := common.GetTimestamp()
	require.NoError(t, db.Create(&model.TeamSubscription{TeamId: team.Id, PlanTitle: "Active", SeatLimit: 2,
		AmountTotal: 1000, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "CodexPlus", Model: "gpt-5-codex", ChannelId: 1, Enabled: true},
		{Group: "CodexPro", Model: "gpt-5-pro", ChannelId: 2, Enabled: true},
	}).Error)

	body, err := common.Marshal(map[string]any{"name": "Invalid key", "group": "CodexPlus", "model_limits": "gpt-5-pro"})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set("id", owner.Id)
	context.Set("group", owner.Group)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/team/tokens", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	TeamCreateToken(context)

	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Success)
	var count int64
	require.NoError(t, db.Model(&model.Token{}).Where("team_id = ?", team.Id).Count(&count).Error)
	assert.Zero(t, count)
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
