package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type teamSuccessPollingAdaptor struct{}

func TestTeamBillingLogDetailsExcludesProviderContentAndCredentials(t *testing.T) {
	details := teamBillingLogDetails(map[string]interface{}{
		"model_ratio": 2.0, "cache_tokens": 3, "cache_ratio": 0.1,
		"billing_mode": "tiered_expr", "matched_tier": "fast",
		"request_rules": "private prompt", "stream_status": "provider secret",
		"admin_info": map[string]interface{}{"channel_key": "secret"},
	})
	assert.Equal(t, 2.0, details["model_ratio"])
	assert.Equal(t, 3, details["cache_tokens"])
	assert.Equal(t, "tiered_expr", details["billing_mode"])
	assert.Equal(t, "fast", details["matched_tier"])
	assert.NotContains(t, details, "request_rules")
	assert.NotContains(t, details, "stream_status")
	assert.NotContains(t, details, "admin_info")
	malformed := teamBillingLogDetails(map[string]interface{}{
		"cache_ratio": math.NaN(),
		"admin_info": map[string]interface{}{"quota_saturation": map[string]interface{}{
			"kind": "nan", "original": math.NaN(), "clamped": 0,
		}},
	})
	encoded, err := common.Marshal(malformed)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "NaN")
	assert.NotContains(t, malformed, "cache_ratio")
	admin, ok := malformed["admin_info"].(map[string]interface{})
	require.True(t, ok)
	anomaly, ok := admin["quota_saturation"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "nan", anomaly["kind"])
	assert.NotContains(t, anomaly, "original")
	pricing := teamBillingLogDetails(map[string]interface{}{
		"billing_mode":  "tiered_expr",
		"expr_b64":      base64.StdEncoding.EncodeToString([]byte(`tier("long", p * 6 + c * 22.5 + cr * 0.6)`)),
		"matched_tier":  "long",
		"request_rules": []billingexpr.RequestRuleTrace{{Cond: `header("service-tier") == "fast"`, Multiplier: 2, Matched: true}},
	})
	assert.Equal(t, "long", pricing["matched_tier"])
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(`tier("long", p * 6 + c * 22.5 + cr * 0.6)`)), pricing["expr_b64"])
	assert.Equal(t, []billingexpr.RequestRuleTrace{{Cond: `header("service-tier") == "fast"`, Multiplier: 2, Matched: true}}, pricing["request_rules"])
	assert.NotContains(t, pricing, "pricing_snapshot_incomplete")
	invalid := teamBillingLogDetails(map[string]interface{}{
		"billing_mode": "tiered_expr", "expr_b64": "not base64",
		"request_rules": []billingexpr.RequestRuleTrace{{Cond: "invalid", Multiplier: math.Inf(1)}},
	})
	assert.NotContains(t, invalid, "expr_b64")
	assert.NotContains(t, invalid, "request_rules")
	assert.Equal(t, true, invalid["pricing_snapshot_incomplete"])
}

func (*teamSuccessPollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (*teamSuccessPollingAdaptor) FetchTask(_, _ string, body map[string]any, _ string) (*http.Response, error) {
	response, err := common.Marshal(taskdto.TaskResponse[model.Task]{Code: taskdto.TaskSuccessCode,
		Data: model.Task{TaskID: body["task_id"].(string), Status: model.TaskStatusSuccess, Progress: "100%"}})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(response))}, nil
}
func (*teamSuccessPollingAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusSuccess}, nil
}
func (*teamSuccessPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func teamBillingFixture(t *testing.T) (int, int) {
	t.Helper()
	if !model.DB.Migrator().HasTable(&model.TeamUsage{}) {
		require.NoError(t, model.DB.AutoMigrate(&model.Team{}, &model.TeamMember{}, &model.TeamSubscription{}, &model.TeamQuotaPeriod{}, &model.TeamUsage{}, &model.TeamSyncBillingEvent{}, &model.TeamSyncLogReceipt{}))
	}
	require.NoError(t, model.DB.AutoMigrate(&model.TeamUsage{}, &model.TeamSyncBillingEvent{}, &model.TeamSyncLogReceipt{}))
	require.NoError(t, model.DB.AutoMigrate(&model.TeamTaskBillingEvent{}, &model.TeamTaskLogReceipt{}))
	user := &model.User{Username: fmt.Sprintf("team-member-%s", t.Name()), Email: fmt.Sprintf("team-%s@example.test", t.Name()), AffCode: common.GetRandomString(12),
		Status: common.UserStatusEnabled, Quota: 1000}
	require.NoError(t, model.DB.Create(user).Error)
	team, err := model.CreateTeam(user.Id, "Billing test team")
	require.NoError(t, err)
	sub := &model.TeamSubscription{TeamId: team.Id, SeatLimit: 2, AmountTotal: 100,
		ResetPeriod: model.SubscriptionResetNever, StartTime: time.Now().Add(-time.Minute).Unix(),
		EndTime: time.Now().Add(time.Hour).Unix(), Status: model.TeamStatusActive}
	require.NoError(t, model.DB.Create(sub).Error)
	t.Cleanup(func() {
		model.DB.Where("team_id = ?", team.Id).Delete(&model.TeamTaskBillingEvent{})
		model.DB.Where("team_id = ?", team.Id).Delete(&model.TeamUsage{})
		model.DB.Where("team_id = ?", team.Id).Delete(&model.TeamSyncBillingEvent{})
		model.DB.Where("team_subscription_id = ?", sub.Id).Delete(&model.TeamQuotaPeriod{})
		model.DB.Delete(sub)
		model.DB.Where("team_id = ?", team.Id).Delete(&model.TeamMember{})
		model.DB.Delete(team)
		model.DB.Delete(user)
	})
	return team.Id, user.Id
}

func teamBillingTokenId(t *testing.T, userId int) int {
	t.Helper()
	token, err := model.CreateTeamToken(userId, "Local billing test", "default", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = model.DB.Delete(token).Error })
	return token.Id
}

func TestTeamTaskLogOutboxRecoversAcrossIndependentLogDatabase(t *testing.T) {
	previousLogConsume := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsume })
	teamId, userId := teamBillingFixture(t)
	channel := &model.Channel{Name: "team-outbox-" + t.Name()}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() { _ = model.DB.Delete(channel).Error })
	usage, period, err := model.PreConsumeTeamForToken("team-outbox-"+t.Name(), teamId, userId, teamBillingTokenId(t, userId), 70)
	require.NoError(t, err)
	task := &model.Task{TaskID: "task-outbox-" + t.Name(), UserId: userId, ChannelId: channel.Id,
		Group: "default", Quota: 70, Status: model.TaskStatusInProgress,
		Properties:  model.Properties{OriginModelName: "video-test"},
		PrivateData: model.TaskPrivateData{BillingSource: BillingSourceTeam, TeamId: teamId, TeamRequestId: usage.RequestId}}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { _ = model.DB.Delete(task).Error })
	final := *task
	final.Status = model.TaskStatusSuccess
	won, err := model.FinalizeTeamTask(&final, model.TaskStatusInProgress, 90, "video completed", nil)
	require.NoError(t, err)
	require.True(t, won)
	originalLogDB := model.LOG_DB
	logDB, err := gorm.Open(sqlite.Open("file:team_outbox_log?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = logDB
	t.Cleanup(func() {
		model.LOG_DB = originalLogDB
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, logDB.AutoMigrate(&model.TeamTaskLogReceipt{}))
	ctx := context.Background()
	require.Error(t, DeliverPendingTeamTaskBillingEvents(ctx, 100), "missing log table must leave the event pending")
	var receipts int64
	require.NoError(t, logDB.Model(&model.TeamTaskLogReceipt{}).Count(&receipts).Error)
	assert.Zero(t, receipts, "receipt must roll back if the log insert fails")
	var event model.TeamTaskBillingEvent
	require.NoError(t, model.DB.First(&event, "task_id = ?", task.ID).Error)
	assert.Zero(t, event.DeliveredAt)
	var member model.User
	require.NoError(t, model.DB.First(&member, userId).Error)
	assert.Equal(t, int64(20), int64(member.UsedQuota), "counter must commit before log database recovery")
	require.NoError(t, model.DB.First(period, period.Id).Error)
	assert.EqualValues(t, 90, period.AmountUsed)
	// The outbox must be sufficient even if task history is unavailable when
	// the independent log database comes back.
	require.NoError(t, model.DB.Delete(task).Error)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	require.NoError(t, DeliverPendingTeamTaskBillingEvents(ctx, 100))
	// Simulate a crash after the log transaction committed, before marking
	// the primary outbox row delivered. A second delivery must not duplicate it.
	require.NoError(t, model.DB.Model(&event).Update("delivered_at", 0).Error)
	require.NoError(t, DeliverPendingTeamTaskBillingEvents(ctx, 100))
	var logs []model.Log
	require.NoError(t, logDB.Where("request_id = ?", fmt.Sprintf("team-task-%d", task.ID)).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, model.LogTypeConsume, logs[0].Type)
	assert.Equal(t, 20, logs[0].Quota)
	assert.Equal(t, "default", logs[0].Group)
	assert.Equal(t, "video completed", logs[0].Content)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	assert.Equal(t, "team", other["billing_source"])
	assert.Equal(t, task.TaskID, other["task_id"])
	require.NoError(t, logDB.Model(&model.TeamTaskLogReceipt{}).Count(&receipts).Error)
	assert.EqualValues(t, 1, receipts)
	require.NoError(t, model.DB.First(&event, "task_id = ?", task.ID).Error)
	assert.NotZero(t, event.DeliveredAt)
}

func TestTeamSyncSettlementRecoversCountersAndIndependentLogExactlyOnce(t *testing.T) {
	previousLogConsume := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsume })
	teamId, userId := teamBillingFixture(t)
	channel := &model.Channel{Name: "sync-outbox-" + t.Name()}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() { _ = model.DB.Delete(channel).Error })
	first, _, err := model.PreConsumeTeamForToken(fmt.Sprintf("sync-outbox-%d", teamId), teamId, userId, teamBillingTokenId(t, userId), 70)
	require.NoError(t, err)
	require.NoError(t, model.MarkTeamSyncUpstreamAttempt(first.RequestId))
	other, _, err := model.PreConsumeTeamForToken("sync-other-"+t.Name(), teamId, userId, teamBillingTokenId(t, userId), 30)
	require.NoError(t, err)
	pricing := teamBillingLogDetails(map[string]interface{}{
		"model_ratio": 1.5, "cache_tokens": 3, "cache_ratio": 0.1,
		"billing_mode": "tiered_expr", "matched_tier": "long",
		"expr_b64":      base64.StdEncoding.EncodeToString([]byte(`tier("long", p * 6 + c * 22.5 + cr * 0.6)`)),
		"request_rules": []billingexpr.RequestRuleTrace{{Cond: `header("service-tier") == "fast"`, Multiplier: 2, Matched: true}},
	})
	encodedPricing, err := common.Marshal(pricing)
	require.NoError(t, err)
	require.NoError(t, model.RecordTeamSyncSettlementIntent(first.RequestId, 90, model.TeamSyncBillingMeta{
		ChannelId: channel.Id, Group: "default", ModelName: "local-model", PromptTokens: 4, CompletionTokens: 2,
		UseTimeSeconds: 3, IsStream: true,
		BillingDetails: string(encodedPricing),
	}))
	recovered, err := model.RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Zero(t, recovered)
	require.NoError(t, model.RefundTeamUsage(other.RequestId))
	recovered, err = model.RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Equal(t, 1, recovered)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 90, user.UsedQuota)
	assert.EqualValues(t, 1, user.RequestCount)
	require.NoError(t, model.DB.First(channel, channel.Id).Error)
	assert.EqualValues(t, 90, channel.UsedQuota)

	originalLogDB := model.LOG_DB
	logDB, err := gorm.Open(sqlite.Open("file:team_sync_log?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = logDB
	t.Cleanup(func() {
		model.LOG_DB = originalLogDB
		if sqlDB, err := logDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, logDB.AutoMigrate(&model.TeamSyncLogReceipt{}))
	ctx := context.Background()
	require.Error(t, DeliverPendingTeamSyncBillingEvents(ctx, 100), "missing independent log table must leave the event pending")
	var receipts int64
	require.NoError(t, logDB.Model(&model.TeamSyncLogReceipt{}).Count(&receipts).Error)
	assert.Zero(t, receipts)
	require.NoError(t, logDB.AutoMigrate(&model.Log{}))
	require.NoError(t, DeliverPendingTeamSyncBillingEvents(ctx, 100))
	// Crash after the log transaction commits but before marking the main
	// event delivered. Replay must not insert another log or charge again.
	require.NoError(t, model.DB.Model(&model.TeamSyncBillingEvent{}).
		Where("request_id = ?", first.RequestId).Update("delivered_at", 0).Error)
	require.NoError(t, DeliverPendingTeamSyncBillingEvents(ctx, 100))
	var logs []model.Log
	require.NoError(t, logDB.Where("request_id = ?", first.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, 90, logs[0].Quota)
	assert.Equal(t, "default", logs[0].Group)
	assert.Equal(t, 4, logs[0].PromptTokens)
	assert.Equal(t, 2, logs[0].CompletionTokens)
	assert.Equal(t, 3, logs[0].UseTime)
	assert.True(t, logs[0].IsStream)
	var otherData map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &otherData))
	assert.Equal(t, "team", otherData["billing_source"])
	assert.Equal(t, 1.5, otherData["model_ratio"])
	assert.Equal(t, float64(3), otherData["cache_tokens"])
	assert.Equal(t, 0.1, otherData["cache_ratio"])
	assert.Equal(t, "tiered_expr", otherData["billing_mode"])
	assert.Equal(t, "long", otherData["matched_tier"])
	assert.Equal(t, pricing["expr_b64"], otherData["expr_b64"])
	rules, ok := otherData["request_rules"].([]any)
	require.True(t, ok)
	require.Len(t, rules, 1)
	rule, ok := rules[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `header("service-tier") == "fast"`, rule["cond"])
	assert.Equal(t, float64(2), rule["multiplier"])
	assert.Equal(t, true, rule["matched"])
	require.NoError(t, logDB.Model(&model.TeamSyncLogReceipt{}).Count(&receipts).Error)
	assert.EqualValues(t, 1, receipts)
	recovered, err = model.RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	assert.Zero(t, recovered)
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 90, user.UsedQuota)
	assert.EqualValues(t, 1, user.RequestCount)
	require.NoError(t, model.MarkTeamSyncResponseState(first.RequestId, "withheld"))
	applied, credit, err := model.CreditTeamDeliveryDispute(first.RequestId, userId, "independent-log:disputed-123")
	require.NoError(t, err)
	assert.True(t, applied)
	assert.EqualValues(t, 90, credit)
	require.NoError(t, DeliverPendingTeamSyncBillingEvents(ctx, 100))
	logs = nil
	require.NoError(t, logDB.Where("request_id = ?", first.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1, "the delivery credit must not alter or duplicate the original consume log")
	assert.Equal(t, 90, logs[0].Quota)
	var originalPeriod model.TeamQuotaPeriod
	require.NoError(t, model.DB.First(&originalPeriod, first.PeriodId).Error)
	assert.Zero(t, originalPeriod.AmountUsed)
}

func TestTeamSyncRecoveryRunsWithoutTaskAdaptor(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	previousAdaptor := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = nil
	t.Cleanup(func() { GetTaskAdaptorFunc = previousAdaptor })
	previousLogConsume := common.LogConsumeEnabled
	common.LogConsumeEnabled = true
	t.Cleanup(func() { common.LogConsumeEnabled = previousLogConsume })
	channel := &model.Channel{Name: "sync-no-task-adaptor"}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() { _ = model.DB.Delete(channel).Error })
	usage, _, err := model.PreConsumeTeamForToken("sync-no-task-adaptor-"+t.Name(), teamId, userId, teamBillingTokenId(t, userId), 40)
	require.NoError(t, err)
	require.NoError(t, model.RecordTeamSyncSettlementIntent(usage.RequestId, 40, model.TeamSyncBillingMeta{
		ChannelId: channel.Id, ModelName: "local-model", Group: "default",
	}))
	RunTaskPollingOnce(context.Background(), nil)
	require.NoError(t, model.DB.First(&usage, usage.Id).Error)
	assert.Equal(t, "settled", usage.Status)
	var event model.TeamSyncBillingEvent
	require.NoError(t, model.DB.Where("request_id = ?", usage.RequestId).First(&event).Error)
	assert.NotZero(t, event.DeliveredAt)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", usage.RequestId).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestOriginalTeamHandlerSettlesAfterRecoveryWithoutDoubleCharge(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	channel := &model.Channel{Name: "recovered-before-handler"}
	require.NoError(t, model.DB.Create(channel).Error)
	t.Cleanup(func() { _ = model.DB.Delete(channel).Error })
	funding := &TeamFunding{requestId: "recovered-before-handler", teamId: teamId,
		userId: userId, tokenId: teamBillingTokenId(t, userId), meta: model.TeamSyncBillingMeta{
			ChannelId: channel.Id, Group: "default", ModelName: "local-model"}}
	require.NoError(t, funding.PreConsume(70))
	require.NoError(t, model.RecordTeamSyncSettlementIntent(funding.requestId, 90, funding.meta))
	recovered, err := model.RecoverPendingTeamSettlements(100)
	require.NoError(t, err)
	require.Equal(t, 1, recovered)
	// The original handler still holds its old preconsumed amount. A delta
	// cannot establish idempotency after another worker committed the cost.
	require.NoError(t, funding.Settle(20))
	assert.EqualValues(t, 90, funding.reserved)
	assert.Error(t, model.SettleTeamSyncUsage(funding.requestId, 80))
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", funding.requestId).First(&usage).Error)
	assert.EqualValues(t, 90, usage.FinalAmount)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 90, user.UsedQuota)
	assert.EqualValues(t, 1, user.RequestCount)
	require.NoError(t, model.DB.First(channel, channel.Id).Error)
	assert.EqualValues(t, 90, channel.UsedQuota)
	require.NoError(t, model.MarkTeamSyncResponseState(funding.requestId, "writing"))
	require.NoError(t, model.MarkTeamSyncResponseState(funding.requestId, "server_write_completed"))
}

func TestTeamSyncSettlementCounterFailureRollsBackFundingAndLogReadiness(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	usage, period, err := model.PreConsumeTeamForToken("sync-counter-failure-"+t.Name(), teamId, userId, teamBillingTokenId(t, userId), 70)
	require.NoError(t, err)
	require.NoError(t, model.RecordTeamSyncSettlementIntent(usage.RequestId, 50, model.TeamSyncBillingMeta{
		ChannelId: 999999, Group: "default", ModelName: "local-model",
	}))
	assert.Error(t, model.AdjustTeamUsage(usage.RequestId, -20, true))
	require.NoError(t, model.DB.First(usage, usage.Id).Error)
	assert.Equal(t, "reserved", usage.Status)
	assert.EqualValues(t, 70, usage.Reserved)
	require.NoError(t, model.DB.First(period, period.Id).Error)
	assert.EqualValues(t, 70, period.AmountUsed)
	var event model.TeamSyncBillingEvent
	require.NoError(t, model.DB.First(&event, "request_id = ?", usage.RequestId).Error)
	assert.False(t, event.Ready)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
}

func TestTeamBillingSessionSettlesSharedQuotaWithoutTouchingMemberWallet(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{TeamId: teamId, UserId: userId, TokenId: teamBillingTokenId(t, userId), RequestId: "team-billing-sync", TokenUnlimited: true}
	session, apiErr := NewBillingSession(c, info, 70)
	require.Nil(t, apiErr)
	require.NoError(t, session.Settle(50))
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).First(&usage).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.EqualValues(t, 50, usage.FinalAmount)
	assert.Equal(t, BillingSourceTeam, info.BillingSource)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 1000, user.Quota)
}

func TestTeamBillingGenericRefundRetainsAmbiguousUpstreamReservation(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{TeamId: teamId, UserId: userId, TokenId: teamBillingTokenId(t, userId),
		RequestId: "team-ambiguous-generic-refund", TokenUnlimited: true}
	session, apiErr := NewBillingSession(c, info, 70)
	require.Nil(t, apiErr)
	require.NoError(t, model.MarkTeamSyncUpstreamAttempt(info.RequestId))
	session.Refund(c)
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).First(&usage).Error)
	assert.Equal(t, "reserved", usage.Status)
	var period model.TeamQuotaPeriod
	require.NoError(t, model.DB.First(&period, usage.PeriodId).Error)
	assert.EqualValues(t, 70, period.AmountUsed)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 1000, user.Quota)
}

// Exercise the shared request-context-to-funding path for every synchronous
// relay format. Upstream protocol response/stream settlement is separate and
// remains behind the production-wide team credential gate.
func TestTeamFundingIdentityAcrossSynchronousRelayFormats(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		format     types.RelayFormat
		request    dto.Request
	}{
		{"openai", "/v1/chat/completions", types.RelayFormatOpenAI, nil},
		{"claude", "/v1/messages", types.RelayFormatClaude, nil},
		{"gemini", "/v1beta/models/test:generateContent", types.RelayFormatGemini, nil},
		{"responses", "/v1/responses", types.RelayFormatOpenAIResponses, &dto.OpenAIResponsesRequest{}},
		{"responses-compact", "/v1/responses/compact", types.RelayFormatOpenAIResponsesCompaction, &dto.OpenAIResponsesCompactionRequest{}},
		{"alpha-search", "/v1/alpha/search", types.RelayFormatOpenAIAlphaSearch, &dto.AlphaSearchRequest{}},
		{"image", "/v1/images/generations", types.RelayFormatOpenAIImage, nil},
		{"audio", "/v1/audio/speech", types.RelayFormatOpenAIAudio, nil},
		{"embedding", "/v1/embeddings", types.RelayFormatEmbedding, nil},
		{"rerank", "/v1/rerank", types.RelayFormatRerank, &dto.RerankRequest{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teamId, userId := teamBillingFixture(t)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, nil)
			c.Set("team_id", teamId)
			common.SetContextKey(c, constant.ContextKeyUserId, userId)
			common.SetContextKey(c, constant.ContextKeyTokenKey, "tmb_local_only")
			common.SetContextKey(c, constant.ContextKeyTokenId, teamBillingTokenId(t, userId))
			common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
			info, err := relaycommon.GenRelayInfo(c, tc.format, tc.request, nil)
			require.NoError(t, err)
			require.Equal(t, teamId, info.TeamId)
			session, apiErr := NewBillingSession(c, info, 70)
			require.Nil(t, apiErr)
			require.NoError(t, session.Settle(50))
			var usage model.TeamUsage
			require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).First(&usage).Error)
			assert.Equal(t, "settled", usage.Status)
			assert.EqualValues(t, 50, usage.FinalAmount)
			var user model.User
			require.NoError(t, model.DB.First(&user, userId).Error)
			assert.EqualValues(t, 1000, user.Quota)
		})
	}
}

func TestTeamSynchronousSettlementOverSharedLimitNeverDebitsMemberWallet(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{TeamId: teamId, UserId: userId, TokenId: teamBillingTokenId(t, userId),
		RequestId: "team-settlement-over-limit-" + t.Name(), TokenUnlimited: true}
	session, apiErr := NewBillingSession(c, info, 70)
	require.Nil(t, apiErr)
	require.Error(t, session.Settle(120), "an under-reserved synchronous response cannot be fully settled")
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).First(&usage).Error)
	assert.Equal(t, "reserved", usage.Status)
	assert.EqualValues(t, 70, usage.Reserved)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 1000, user.Quota)
}

func TestTeamSynchronousSettlementFailureDoesNotPublishPhantomUsage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		consume func(*gin.Context, *relaycommon.RelayInfo)
	}{
		{"text", func(c *gin.Context, info *relaycommon.RelayInfo) {
			PostTextConsumeQuota(c, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 1}, nil)
		}},
		{"audio", func(c *gin.Context, info *relaycommon.RelayInfo) {
			PostAudioConsumeQuota(c, info, &dto.Usage{PromptTokens: 10, CompletionTokens: 1, TotalTokens: 11}, "")
		}},
		{"realtime", func(c *gin.Context, info *relaycommon.RelayInfo) {
			PostWssConsumeQuota(c, info, "team-test", &dto.RealtimeUsage{InputTokens: 10, OutputTokens: 1, TotalTokens: 11}, "")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			teamId, userId := teamBillingFixture(t)
			channel := &model.Channel{Name: "team-sync-failure-" + t.Name()}
			require.NoError(t, model.DB.Create(channel).Error)
			t.Cleanup(func() { _ = model.DB.Delete(channel).Error })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{TeamId: teamId, UserId: userId, TokenId: teamBillingTokenId(t, userId), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id},
				RequestId: "team-text-over-limit-" + t.Name(), TokenUnlimited: true, OriginModelName: "team-test",
				StartTime: time.Now(), PriceData: hosttypes.PriceData{UsePrice: true,
					ModelPrice:     120 / common.QuotaPerUnit,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}}}
			session, apiErr := NewBillingSession(c, info, 70)
			require.Nil(t, apiErr)
			require.NotNil(t, session)
			tc.consume(c, info)
			var reservation model.TeamUsage
			require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).First(&reservation).Error)
			assert.Equal(t, "reserved", reservation.Status)
			assert.EqualValues(t, 70, reservation.Reserved)
			var user model.User
			require.NoError(t, model.DB.First(&user, userId).Error)
			assert.EqualValues(t, 1000, user.Quota)
			assert.Zero(t, user.UsedQuota)
			require.NoError(t, model.DB.First(channel, channel.Id).Error)
			assert.Zero(t, channel.UsedQuota)
			var logs int64
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND model_name = ?", userId, "team-test").Count(&logs).Error)
			assert.Zero(t, logs)
		})
	}
}

func TestTeamCredentialMissingTeamIdentityNeverFallsBackToPersonalWallet(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{TokenKey: "tmb_test", UserId: 42, RequestId: "team-missing-identity"}

	session, apiErr := NewBillingSession(c, info, 10)
	assert.Nil(t, session)
	require.NotNil(t, apiErr)
	assert.Error(t, PostConsumeQuota(info, 10, 0, false))
}

func TestTeamFundingAsyncFailureRefundsOriginalReservationWithoutWalletFallback(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	funding := &TeamFunding{requestId: "team-billing-async", teamId: teamId, userId: userId, tokenId: teamBillingTokenId(t, userId), async: true}
	require.NoError(t, funding.PreConsume(90))
	require.NoError(t, funding.Settle(0))
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", funding.requestId).First(&usage).Error)
	assert.Equal(t, "reserved", usage.Status)
	require.NoError(t, funding.Refund())
	require.NoError(t, funding.Refund())
	require.NoError(t, model.DB.First(&usage, usage.Id).Error)
	assert.Equal(t, "refunded", usage.Status)
	var period model.TeamQuotaPeriod
	require.NoError(t, model.DB.First(&period, usage.PeriodId).Error)
	assert.Zero(t, period.AmountUsed)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 1000, user.Quota)
}

func TestTeamTaskRefundNeverCreditsMemberWallet(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	funding := &TeamFunding{requestId: "team-task-refund", teamId: teamId, userId: userId, tokenId: teamBillingTokenId(t, userId), async: true}
	require.NoError(t, funding.PreConsume(90))
	require.NoError(t, funding.Settle(0))
	seedChannel(t, 883)
	t.Cleanup(func() { model.DB.Where("id = ?", 883).Delete(&model.Channel{}) })
	seedChargedAccounting(t, userId, 883, 0, 90, 1)
	task := makeTask(userId, 883, 90, 0, BillingSourceTeam, 0)
	task.PrivateData.TeamId = teamId
	task.PrivateData.TeamRequestId = funding.requestId
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { model.DB.Delete(task) })
	stale := *task
	assert.True(t, RefundTaskQuota(context.Background(), task, "upstream failed"))
	assert.True(t, RefundTaskQuota(context.Background(), &stale, "retry with stale snapshot"))
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 1000, user.Quota)
	assert.Zero(t, user.UsedQuota, "idempotent refund must not decrement accounting twice")
	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", funding.requestId).First(&usage).Error)
	assert.Equal(t, "refunded", usage.Status)
}

func TestMissingUpstreamTeamTaskRefundsOnlyAfterWinningTerminalTransition(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	funding := &TeamFunding{requestId: "team-task-missing-upstream", teamId: teamId, userId: userId, tokenId: teamBillingTokenId(t, userId), async: true}
	require.NoError(t, funding.PreConsume(90))
	seedChannel(t, 884)
	t.Cleanup(func() { model.DB.Where("id = ?", 884).Delete(&model.Channel{}) })
	seedChargedAccounting(t, userId, 884, 0, 90, 1)
	task := makeTask(userId, 884, 90, 0, BillingSourceTeam, 0)
	task.PrivateData.TeamId = teamId
	task.PrivateData.TeamRequestId = funding.requestId
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { model.DB.Delete(task) })
	stale := *task

	failMissingUpstreamTeamTask(context.Background(), task)
	failMissingUpstreamTeamTask(context.Background(), &stale)

	var usage model.TeamUsage
	require.NoError(t, model.DB.Where("request_id = ?", funding.requestId).First(&usage).Error)
	assert.Equal(t, "refunded", usage.Status)
	var period model.TeamQuotaPeriod
	require.NoError(t, model.DB.First(&period, usage.PeriodId).Error)
	assert.Zero(t, period.AmountUsed)
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), stored.Status)
	assert.Zero(t, stored.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.EqualValues(t, 1000, user.Quota)
}

func TestTeamVideoPollingRetriesInsufficientSettlementAndUsesSubmittedPrice(t *testing.T) {
	teamId, userId := teamBillingFixture(t)
	first, _, err := model.PreConsumeTeamForToken("team-video-retry", teamId, userId, teamBillingTokenId(t, userId), 70)
	require.NoError(t, err)
	other, _, err := model.PreConsumeTeamForToken("team-video-concurrent", teamId, userId, teamBillingTokenId(t, userId), 30)
	require.NoError(t, err)
	seedChannel(t, 885)
	t.Cleanup(func() { model.DB.Where("id = ?", 885).Delete(&model.Channel{}) })
	seedChargedAccounting(t, userId, 885, 0, 70, 1)
	quota := 90
	task := makeTask(userId, 885, 70, 0, BillingSourceTeam, 0)
	task.Platform = constant.TaskPlatform("kling")
	task.Status = model.TaskStatusInProgress
	task.PrivateData.TeamId, task.PrivateData.TeamRequestId = teamId, first.RequestId
	task.PrivateData.BillingContext = &model.TaskBillingContext{PerCallBilling: true, SubmitQuota: &quota}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() { model.DB.Delete(task) })
	channel := &model.Channel{Id: 885, Type: constant.ChannelTypeKling}
	adaptor := &teamSuccessPollingAdaptor{}
	err = updateVideoSingleTask(context.Background(), adaptor, channel, task.TaskID, map[string]*model.Task{task.TaskID: task})
	require.ErrorContains(t, err, "retained for retry")
	var stored model.Task
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status)
	assert.Equal(t, 70, stored.Quota)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.PrivateData.PendingFinalStatus)
	assert.EqualValues(t, 90, stored.PrivateData.PendingFinalQuota)
	assert.Equal(t, "99%", stored.Progress)
	require.NoError(t, model.DB.Model(&stored).Update("submit_time", time.Now().Add(-24*time.Hour).Unix()).Error)
	previousTimeout := constant.TaskTimeoutMinutes
	constant.TaskTimeoutMinutes = 1
	t.Cleanup(func() { constant.TaskTimeoutMinutes = previousTimeout })
	sweepTimedOutTasks(context.Background())
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), stored.Status,
		"timeout must not refund an upstream success while final charge is pending")
	var pending model.TeamUsage
	require.NoError(t, model.DB.First(&pending, first.Id).Error)
	assert.Equal(t, "reserved", pending.Status)
	require.NoError(t, model.RefundTeamUsage(other.RequestId))
	require.NoError(t, updateVideoSingleTask(context.Background(), adaptor, channel, stored.TaskID, map[string]*model.Task{stored.TaskID: &stored}))
	require.NoError(t, model.DB.First(&stored, task.ID).Error)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), stored.Status)
	assert.Equal(t, 90, stored.Quota)
	assert.Empty(t, stored.PrivateData.PendingFinalStatus)
	var usage model.TeamUsage
	require.NoError(t, model.DB.First(&usage, first.Id).Error)
	assert.Equal(t, "settled", usage.Status)
	assert.EqualValues(t, 90, usage.FinalAmount)
	var user model.User
	require.NoError(t, model.DB.First(&user, userId).Error)
	assert.Equal(t, 1000, user.Quota)
}
