package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test registers the actual API router rather than a test-only handler group.
func TestTeamOfficialPurchaseAndTokenRoutesWithLocalReleaseOverlay(t *testing.T) {
	if !operation_setting.TeamReleaseReady {
		t.Skip("team release gate is disabled")
	}
	require.NoError(t, i18n.Init())
	setupRelayRouterTestDB(t)
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
	settings := config.GlobalConfig.Get("team_setting").(*operation_setting.TeamSetting)
	previousEnabled := settings.Enabled
	settings.Enabled = true
	t.Cleanup(func() { settings.Enabled = previousEnabled })
	payment := operation_setting.GetPaymentSetting()
	previousPayment := *payment
	payment.ComplianceConfirmed = true
	payment.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() { *payment = previousPayment })
	previousAddress, previousID, previousKey, previousMethods := operation_setting.PayAddress,
		operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods
	operation_setting.PayAddress = "https://local-payment.example.test"
	operation_setting.EpayId = "local-team-merchant"
	operation_setting.EpayKey = "local-test-only"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "Local Epay"}}
	t.Cleanup(func() {
		operation_setting.PayAddress, operation_setting.EpayId, operation_setting.EpayKey, operation_setting.PayMethods =
			previousAddress, previousID, previousKey, previousMethods
	})
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}, &model.SubscriptionPlan{},
		&model.Team{}, &model.TeamMember{}, &model.TeamInvitation{}, &model.TeamSubscription{},
		&model.TeamQuotaPeriod{}, &model.TeamOrder{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}, &model.BusinessEvent{}))
	owner := &model.User{Username: "official-purchase-owner", Status: common.UserStatusEnabled,
		Group: "default", Quota: 2_000_000, AffCode: "official-purchase-owner-local"}
	require.NoError(t, model.DB.Create(owner).Error)
	plan := &model.SubscriptionPlan{Scope: "team", SeatLimit: 2, Title: "Official route local plan",
		Enabled: true, PriceAmount: 1, TotalAmount: 1000,
		DurationUnit: model.SubscriptionDurationMonth, DurationValue: 1, QuotaResetPeriod: model.SubscriptionResetNever}
	require.NoError(t, model.DB.Create(plan).Error)
	session, err := service.CreateLoginSession(owner.Id, "password", "127.0.0.1", "local-official-route")
	require.NoError(t, err)
	engine := gin.New()
	SetApiRouter(engine)
	send := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+session.AccessToken)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	created := send(http.MethodPost, "/api/team/", `{"name":"Official route local team"}`)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	assert.Contains(t, created.Body.String(), `"success":true`)
	paid := send(http.MethodPost, "/api/team/balance/pay", fmt.Sprintf(`{"plan_id":%d}`, plan.Id))
	require.Equal(t, http.StatusOK, paid.Code, paid.Body.String())
	assert.Contains(t, paid.Body.String(), `"success":true`)
	epay := send(http.MethodPost, "/api/team/epay/pay", fmt.Sprintf(`{"plan_id":%d,"payment_method":"alipay"}`, plan.Id))
	require.Equal(t, http.StatusOK, epay.Code, epay.Body.String())
	assert.Contains(t, epay.Body.String(), `"message":"success"`)
	var pending model.TeamOrder
	require.NoError(t, model.DB.Where("payer_user_id = ? AND payment_provider = ?", owner.Id,
		model.PaymentProviderEpay).First(&pending).Error)
	assert.Equal(t, common.TopUpStatusPending, pending.Status, "opening a payment page cannot activate the next term")
	self := send(http.MethodGet, "/api/team/self", "")
	require.Equal(t, http.StatusOK, self.Code)
	assert.Contains(t, self.Body.String(), `"pending_payment"`)
	assert.NotContains(t, self.Body.String(), pending.TradeNo, "browser response must not reveal the payment trade number")
	resumed := send(http.MethodPost, "/api/team/epay/resume", "")
	require.Equal(t, http.StatusOK, resumed.Code, resumed.Body.String())
	assert.Contains(t, resumed.Body.String(), pending.TradeNo, "reopening must use the same payable order")
	var pendingCount int64
	require.NoError(t, model.DB.Model(&model.TeamOrder{}).Where("team_id = ? AND status = ?", pending.TeamId, common.TopUpStatusPending).Count(&pendingCount).Error)
	assert.EqualValues(t, 1, pendingCount, "resume cannot create another payable order")
	key := send(http.MethodPost, "/api/team/tokens", `{"name":"Official route team key"}`)
	require.Equal(t, http.StatusOK, key.Code, key.Body.String())
	assert.Contains(t, key.Body.String(), `"success":true`)
	var token model.Token
	require.NoError(t, model.DB.Where("user_id = ? AND team_id > 0", owner.Id).First(&token).Error)
	assert.True(t, token.TeamEnabled)
	var stored model.User
	require.NoError(t, model.DB.First(&stored, owner.Id).Error)
	assert.Less(t, stored.Quota, 2_000_000, "only the explicit balance purchase debits the owner wallet")
}

// Verify the official relay route against a local simulated upstream.
func TestTeamOfficialRelayRouteWithLocalReleaseOverlay(t *testing.T) {
	if !operation_setting.TeamReleaseReady {
		t.Skip("team release gate is disabled")
	}
	require.NoError(t, i18n.Init())
	setupRelayRouterTestDB(t)
	previousLogConsume, previousMemoryCache := common.LogConsumeEnabled, common.MemoryCacheEnabled
	common.LogConsumeEnabled, common.MemoryCacheEnabled = true, false
	t.Cleanup(func() {
		common.LogConsumeEnabled, common.MemoryCacheEnabled = previousLogConsume, previousMemoryCache
	})
	settings := config.GlobalConfig.Get("team_setting").(*operation_setting.TeamSetting)
	previousEnabled := settings.Enabled
	settings.Enabled = true
	t.Cleanup(func() { settings.Enabled = previousEnabled })
	previousPrices := ratio_setting.ModelPrice2JSONString()
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o-mini":0.001,"alpha-local":0.001}`))
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices)) })
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.ChannelHealthSnapshot{}, &model.ChannelHealthBucket{},
		&model.Team{}, &model.TeamMember{}, &model.TeamSubscription{}, &model.TeamQuotaPeriod{},
		&model.TeamUsage{}, &model.TeamSyncBillingEvent{}, &model.TeamSyncLogReceipt{}, &model.Log{}, &model.BusinessEvent{}))
	owner := &model.User{Username: "official-team-owner", Status: common.UserStatusEnabled,
		Group: "default", Quota: 1000, AffCode: "official-team-owner-local"}
	member := &model.User{Username: "official-team-member", Status: common.UserStatusEnabled,
		Group: "default", Quota: 2000, AffCode: "official-team-member-local"}
	require.NoError(t, model.DB.Create(owner).Error)
	require.NoError(t, model.DB.Create(member).Error)
	team, err := model.CreateTeam(owner.Id, "Official route acceptance")
	require.NoError(t, err)
	require.NoError(t, model.DB.Create(&model.TeamMember{TeamId: team.Id, UserId: member.Id, Role: "member"}).Error)
	now := time.Now().Unix()
	term := &model.TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 10000,
		ResetPeriod: model.SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}
	require.NoError(t, model.DB.Create(term).Error)
	token, err := model.CreateTeamToken(member.Id, "Official route key", "default", "gpt-4o-mini,alpha-local")
	require.NoError(t, err)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"id":"chatcmpl-local","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
		case "/v1/responses":
			_, _ = w.Write([]byte(`{"id":"resp_local","object":"response","created_at":1,"model":"gpt-4o-mini","status":"completed","output":[{"id":"msg_local","type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
		case "/v1/responses/compact":
			_, _ = w.Write([]byte(`{"id":"compact_local","object":"response.compaction","created_at":1,"model":"gpt-4o-mini","output":[{"type":"compaction","summary":"pong"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
		case "/v1/embeddings":
			_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"gpt-4o-mini","usage":{"prompt_tokens":2,"total_tokens":2}}`))
		case "/v1/images/generations":
			_, _ = w.Write([]byte(`{"created":1,"data":[{"b64_json":"aGVsbG8="}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(make([]byte, 48000))
		case "/v1/rerank":
			_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.8}],"usage":{"total_tokens":3}}`))
		case "/v1/alpha/search":
			_, _ = w.Write([]byte(`{"results":[{"title":"pong"}]}`))
		default:
			t.Errorf("unexpected upstream path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer upstream.Close()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "official-team-local",
		Key: "local-upstream-only", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL,
		Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, model.DB.Create(channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "gpt-4o-mini",
		ChannelId: channel.Id, Enabled: true, Weight: 100}).Error)
	searchChannel := &model.Channel{Type: constant.ChannelTypeNewAPI, Name: "official-team-search-local",
		Key: "local-search-upstream-only", Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL,
		Models: "alpha-local", Group: "default"}
	require.NoError(t, model.DB.Create(searchChannel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{Group: "default", Model: "alpha-local",
		ChannelId: searchChannel.Id, Enabled: true, Weight: 100}).Error)
	engine := gin.New()
	SetRelayRouter(engine)
	sendPath := func(path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token.Key)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}
	send := func(modelName string) *httptest.ResponseRecorder {
		return sendPath("/v1/chat/completions",
			`{"model":"`+modelName+`","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`)
	}
	success := send("gpt-4o-mini")
	require.Equal(t, http.StatusOK, success.Code, success.Body.String())
	assert.Contains(t, success.Body.String(), "pong")
	assert.EqualValues(t, 1, upstreamCalls.Load())
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("team_id = ? AND member_user_id = ?", team.Id, member.Id).First(&usage).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.Positive(t, usage.FinalAmount)
	var event model.TeamSyncBillingEvent
	require.NoError(t, model.DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
	assert.True(t, event.Ready)
	assert.Equal(t, "server_write_completed", event.ResponseState)
	require.NoError(t, service.DeliverPendingTeamSyncBillingEvents(context.Background(), 100))
	var logs int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", usage.RequestId).Count(&logs).Error)
	assert.EqualValues(t, 1, logs)
	require.NoError(t, model.DB.First(member, member.Id).Error)
	assert.EqualValues(t, 2000, member.Quota)
	for _, protocol := range []struct {
		name, path, body, result string
	}{
		{"claude", "/v1/messages", `{"model":"gpt-4o-mini","max_tokens":8,"messages":[{"role":"user","content":"ping"}]}`, "pong"},
		{"gemini", "/v1beta/models/gpt-4o-mini:generateContent", `{"contents":[{"role":"user","parts":[{"text":"ping"}]}],"generationConfig":{"maxOutputTokens":8}}`, "pong"},
		{"responses", "/v1/responses", `{"model":"gpt-4o-mini","input":"ping","max_output_tokens":8}`, "pong"},
		{"responses-compact", "/v1/responses/compact", `{"model":"gpt-4o-mini","input":"ping"}`, "pong"},
		{"embedding", "/v1/embeddings", `{"model":"gpt-4o-mini","input":"ping"}`, "embedding"},
		{"image", "/v1/images/generations", `{"model":"gpt-4o-mini","prompt":"ping","n":1}`, "aGVsbG8="},
		{"audio", "/v1/audio/speech", `{"model":"gpt-4o-mini","input":"ping","voice":"alloy","response_format":"pcm"}`, ""},
		{"rerank", "/v1/rerank", `{"model":"gpt-4o-mini","query":"ping","documents":["pong"]}`, "relevance_score"},
		{"alpha-search", "/v1/alpha/search", `{"model":"alpha-local","query":"ping"}`, "pong"},
	} {
		t.Run(protocol.name, func(t *testing.T) {
			response := sendPath(protocol.path, protocol.body)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), protocol.result)
			assert.NotEmpty(t, response.Body.Bytes())
		})
	}
	const successfulRequests = 10
	assert.EqualValues(t, successfulRequests, upstreamCalls.Load())
	var usages []model.TeamUsage
	require.NoError(t, model.DB.Where("team_id = ? AND member_user_id = ?", team.Id, member.Id).Find(&usages).Error)
	require.Len(t, usages, successfulRequests)
	require.NoError(t, service.DeliverPendingTeamSyncBillingEvents(context.Background(), 100))
	for _, settled := range usages {
		assert.Equal(t, "settled", settled.Status)
		assert.Positive(t, settled.FinalAmount)
		var billing model.TeamSyncBillingEvent
		require.NoError(t, model.DB.Where("request_id = ?", settled.RequestId).First(&billing).Error)
		expectedChannel := channel.Id
		if billing.ModelName == "alpha-local" {
			expectedChannel = searchChannel.Id
		}
		assert.Equal(t, expectedChannel, billing.ChannelId, "distribution must use the model-capable channel")
		assert.Equal(t, token.Id, billing.TokenId)
		require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where(&model.Log{
			RequestId: settled.RequestId, UserId: member.Id, Group: "default",
			ChannelId: expectedChannel, TokenId: token.Id, Quota: int(settled.FinalAmount),
		}).Count(&logs).Error)
		assert.EqualValues(t, 1, logs, "each formal protocol gets one member-owned team log")
	}
	streamRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"stream":true,"max_tokens":8}`))
	streamRequest.Header.Set("Authorization", "Bearer "+token.Key)
	streamRequest.Header.Set("Content-Type", "application/json")
	streamResponse := httptest.NewRecorder()
	engine.ServeHTTP(streamResponse, streamRequest)
	assert.Equal(t, http.StatusServiceUnavailable, streamResponse.Code)
	assert.NotContains(t, streamResponse.Body.String(), "pong")
	realtimeRequest := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gpt-4o-mini", nil)
	realtimeRequest.Header.Set("Authorization", "Bearer "+token.Key)
	realtimeResponse := httptest.NewRecorder()
	engine.ServeHTTP(realtimeResponse, realtimeRequest)
	assert.Equal(t, http.StatusServiceUnavailable, realtimeResponse.Code)
	assert.NotContains(t, realtimeResponse.Body.String(), "pong")
	require.NoError(t, model.DB.Model(&model.TeamUsage{}).Where("team_id = ?", team.Id).Count(&logs).Error)
	assert.EqualValues(t, successfulRequests, logs, "unsafe live protocols must not reserve team quota")
	assert.Equal(t, http.StatusForbidden, send("unauthorized-model").Code)
	require.NoError(t, model.DisableTeamToken(member.Id, team.Id, token.Id))
	assert.Equal(t, http.StatusUnauthorized, send("gpt-4o-mini").Code)
	require.NoError(t, model.DB.Model(token).Update("team_enabled", true).Error)
	require.NoError(t, model.DB.Model(term).Update("end_time", now-1).Error)
	assert.Equal(t, http.StatusUnauthorized, send("gpt-4o-mini").Code)
	require.NoError(t, model.DB.Model(term).Update("end_time", now+3600).Error)
	require.NoError(t, model.RemoveTeamMember(owner.Id, member.Id))
	assert.Equal(t, http.StatusUnauthorized, send("gpt-4o-mini").Code)
	assert.EqualValues(t, successfulRequests, upstreamCalls.Load())
	require.NoError(t, model.DB.First(member, member.Id).Error)
	assert.EqualValues(t, 2000, member.Quota)
}
