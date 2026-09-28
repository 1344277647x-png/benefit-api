package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamTokenAuthDistributorRelaySettlesAndRevokesLiveCredentials(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMaster, previousSQLite := common.IsMasterNode, common.SQLitePath
	previousDSN, hadDSN := os.LookupEnv("SQL_DSN")
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMemory, previousLogConsume := common.RedisEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled
	teamSetting := config.GlobalConfig.Get("team_setting").(*operation_setting.TeamSetting)
	previousEnabled := teamSetting.Enabled
	previousPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.IsMasterNode, common.SQLitePath = previousMaster, previousSQLite
		if hadDSN {
			require.NoError(t, os.Setenv("SQL_DSN", previousDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.RedisEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = previousRedis, previousMemory, previousLogConsume
		teamSetting.Enabled = previousEnabled
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
	})
	common.IsMasterNode, common.SQLitePath = false, "file:team_auth_relay?mode=memory&cache=shared"
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled, common.MemoryCacheEnabled, common.LogConsumeEnabled = false, false, true
	teamSetting.Enabled = true
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-4o-mini":0.001}`))
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{},
		&model.ChannelHealthSnapshot{}, &model.ChannelHealthBucket{}, &model.Team{}, &model.TeamMember{},
		&model.TeamSubscription{}, &model.TeamQuotaPeriod{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{},
		&model.TeamSyncLogReceipt{}, &model.Log{}))
	owner := &model.User{Id: 900001, Username: "local-auth-team-owner", Status: common.UserStatusEnabled,
		Group: "default", Quota: 1000, AffCode: "local-auth-owner"}
	member := &model.User{Id: 900002, Username: "local-auth-team-member", Status: common.UserStatusEnabled,
		Group: "default", Quota: 2000, AffCode: "local-auth-member"}
	require.NoError(t, db.Create(owner).Error)
	require.NoError(t, db.Create(member).Error)
	team, err := model.CreateTeam(owner.Id, "Authentication acceptance")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.TeamMember{TeamId: team.Id, UserId: member.Id, Role: "member"}).Error)
	now := time.Now().Unix()
	sub := &model.TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 10000,
		ResetPeriod: model.SubscriptionResetNever, StartTime: now - 60, EndTime: now + 3600, Status: model.TeamStatusActive}
	require.NoError(t, db.Create(sub).Error)
	token, err := model.CreateTeamToken(member.Id, "Member relay", "default", "gpt-4o-mini")
	require.NoError(t, err)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/responses" {
			_, _ = w.Write([]byte(`{"id":"resp_local","object":"response","created_at":1,"model":"gpt-4o-mini","status":"completed","output":[{"id":"msg_local","type":"message","role":"assistant","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`))
			return
		}
		if r.URL.Path == "/v1/embeddings" {
			_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"gpt-4o-mini","usage":{"prompt_tokens":2,"total_tokens":2}}`))
			return
		}
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-local","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`))
	}))
	t.Cleanup(upstream.Close)
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "local-auth-default", Key: "local-upstream-only",
		Status: common.ChannelStatusEnabled, BaseURL: &upstream.URL, Models: "gpt-4o-mini", Group: "default"}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "gpt-4o-mini", ChannelId: channel.Id, Enabled: true, Weight: 100}).Error)
	send := func(auth gin.HandlerFunc, path, body string, format types.RelayFormat) *httptest.ResponseRecorder {
		engine := gin.New()
		engine.POST(path, auth, middleware.Distribute(), func(c *gin.Context) { controller.Relay(c, format) })
		recorder := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token.Key)
		r.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(recorder, r)
		return recorder
	}
	request := func(auth gin.HandlerFunc, modelName string) *httptest.ResponseRecorder {
		return send(auth, "/v1/chat/completions", `{"model":"`+modelName+`","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`, types.RelayFormatOpenAI)
	}
	positive := request(middleware.TokenAuth(), "gpt-4o-mini")
	require.Equal(t, http.StatusOK, positive.Code, positive.Body.String())
	assert.Contains(t, positive.Body.String(), "pong")
	assert.EqualValues(t, 1, upstreamCalls.Load())
	var usage model.TeamUsage
	require.NoError(t, db.Where("team_id = ?", team.Id).First(&usage).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.Positive(t, usage.FinalAmount)
	assert.Equal(t, member.Id, usage.MemberUserId)
	var logs int64
	require.NoError(t, db.Model(&model.Log{}).Where("request_id = ?", usage.RequestId).Count(&logs).Error)
	assert.EqualValues(t, 1, logs)
	require.NoError(t, db.First(member, member.Id).Error)
	assert.EqualValues(t, 2000, member.Quota)
	for _, tc := range []struct {
		name, path, body, result string
		format                   types.RelayFormat
	}{
		{"claude", "/v1/messages", `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"max_tokens":8}`, "pong", types.RelayFormatClaude},
		{"gemini", "/v1beta/models/gpt-4o-mini:generateContent", `{"contents":[{"role":"user","parts":[{"text":"ping"}]}],"generationConfig":{"maxOutputTokens":8}}`, "pong", types.RelayFormatGemini},
		{"responses", "/v1/responses", `{"model":"gpt-4o-mini","input":"ping","max_output_tokens":8}`, "pong", types.RelayFormatOpenAIResponses},
		{"embedding", "/v1/embeddings", `{"model":"gpt-4o-mini","input":"ping"}`, "embedding", types.RelayFormatEmbedding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := send(middleware.TokenAuth(), tc.path, tc.body, tc.format)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), tc.result)
		})
	}
	require.NoError(t, db.Model(&model.TeamUsage{}).Where("team_id = ? AND status = ?", team.Id, "settled").Count(&logs).Error)
	assert.EqualValues(t, 5, logs)
	require.NoError(t, db.Model(&model.Log{}).Where("user_id = ?", member.Id).Count(&logs).Error)
	assert.EqualValues(t, 5, logs)
	require.NoError(t, db.First(member, member.Id).Error)
	assert.EqualValues(t, 2000, member.Quota)
	streamed := send(middleware.TokenAuth(), "/v1/chat/completions",
		`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"ping"}],"stream":true,"max_tokens":8}`,
		types.RelayFormatOpenAI)
	assert.Equal(t, http.StatusServiceUnavailable, streamed.Code)
	assert.NotContains(t, streamed.Body.String(), "pong")
	wsRouter := gin.New()
	wsRouter.GET("/v1/realtime", middleware.TokenAuth(), middleware.Distribute(),
		func(c *gin.Context) { controller.Relay(c, types.RelayFormatOpenAIRealtime) })
	realtime := httptest.NewRecorder()
	wsRequest := httptest.NewRequest(http.MethodGet, "/v1/realtime?model=gpt-4o-mini", nil)
	wsRequest.Header.Set("Authorization", "Bearer "+token.Key)
	wsRouter.ServeHTTP(realtime, wsRequest)
	assert.Equal(t, http.StatusServiceUnavailable, realtime.Code)
	assert.NotContains(t, realtime.Body.String(), "pong")
	assert.EqualValues(t, 5, upstreamCalls.Load(), "unsupported live protocols must not call the upstream")
	require.NoError(t, db.Model(&model.TeamUsage{}).Where("team_id = ?", team.Id).Count(&logs).Error)
	assert.EqualValues(t, 5, logs, "unsupported live protocols must not reserve quota")
	assert.Equal(t, http.StatusForbidden, request(middleware.TokenAuth(), "not-allowed").Code)
	require.NoError(t, model.DisableTeamToken(member.Id, team.Id, token.Id))
	assert.Equal(t, http.StatusUnauthorized, request(middleware.TokenAuth(), "gpt-4o-mini").Code)
	require.NoError(t, db.Model(token).Update("team_enabled", true).Error)
	require.NoError(t, db.Model(sub).Update("end_time", now-1).Error)
	assert.Equal(t, http.StatusUnauthorized, request(middleware.TokenAuth(), "gpt-4o-mini").Code)
	require.NoError(t, db.Model(sub).Update("end_time", now+3600).Error)
	require.NoError(t, model.RemoveTeamMember(owner.Id, member.Id))
	assert.Equal(t, http.StatusUnauthorized, request(middleware.TokenAuth(), "gpt-4o-mini").Code)
	assert.EqualValues(t, 5, upstreamCalls.Load(), "revoked credentials must not reach the upstream")
}
