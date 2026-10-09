package service

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unavailablePartialFunding struct{ WalletFunding }

func (f *unavailablePartialFunding) Settle(int) error {
	return errors.New("test settlement unavailable")
}

func TestFailedStreamSettlementFailureRetainsReservationWithoutConsumeLog(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.BusinessEvent{}, &model.BusinessEventLogReceipt{}))
	truncate(t)
	seedUser(t, 721, 1000)
	seedToken(t, 722, 721, "partial-failure-key", 1000)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set(common.RequestIdKey, "partial-failure-reconcile")
	info := &relaycommon.RelayInfo{
		UserId: 721, TokenId: 722, TokenKey: "partial-failure-key", OriginModelName: "gpt-4",
		IsStream: true, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1},
		BillingSource: BillingSourceWallet,
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}
	funding := &unavailablePartialFunding{WalletFunding{userId: 721}}
	require.NoError(t, funding.PreConsume(100))
	require.NoError(t, model.DecreaseTokenQuota(722, "partial-failure-key", 100))
	info.Billing = &BillingSession{relayInfo: info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}
	RetainPartialStreamUsage(c, info, &dto.Usage{PromptTokens: 20, CompletionTokens: 10}, "")
	require.True(t, SettleFailedTextStream(c, info))
	require.True(t, SettleFailedTextStream(c, info))
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, 721).Error)
	require.NoError(t, model.DB.First(&token, 722).Error)
	assert.Equal(t, 900, user.Quota)
	assert.Zero(t, user.UsedQuota)
	assert.Zero(t, user.RequestCount)
	assert.Equal(t, 900, token.RemainQuota)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 721, model.LogTypeConsume).Count(&count).Error)
	assert.Zero(t, count)
	var exception model.BusinessEvent
	require.NoError(t, model.DB.Where("event_key = ?", "billing-exception:partial-failure-reconcile").First(&exception).Error)
	assert.Equal(t, model.BusinessEventBilling, exception.Category)
	assert.Contains(t, exception.DetailsJSON, `"reserved_quota":100`)
	assert.Contains(t, exception.DetailsJSON, `"expected_quota":40`)
	assert.Contains(t, exception.DetailsJSON, `"failure_stage":"funding_settlement_failed"`)
	assert.NotContains(t, exception.DetailsJSON, "partial-failure-key")
	// Log recovery is idempotent and does not turn the exception into a charge.
	require.NoError(t, model.DeliverBusinessEvent(context.Background(), exception))
	require.NoError(t, model.DeliverBusinessEvent(context.Background(), exception))
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", exception.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, model.LogTypeError, logs[0].Type)
	assert.Zero(t, logs[0].Quota)
	assert.Contains(t, logs[0].Other, "pending_manual_review")
}

func TestFailedTextStreamChargesSubscriptionWithoutWalletDebit(t *testing.T) {
	truncate(t)
	seedUser(t, 731, 1000)
	seedToken(t, 732, 731, "partial-subscription-key", 900)
	seedSubscription(t, 733, 731, 1000, 100)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		UserId: 731, TokenId: 732, TokenKey: "partial-subscription-key", OriginModelName: "gpt-4",
		IsStream: true, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1},
		BillingSource: BillingSourceSubscription, SubscriptionId: 733,
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}
	funding := &SubscriptionFunding{subscriptionId: 733, preConsumed: 100}
	info.Billing = &BillingSession{relayInfo: info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}
	RetainPartialStreamUsage(c, info, &dto.Usage{PromptTokens: 20, CompletionTokens: 10}, "")
	require.True(t, SettleFailedTextStream(c, info))
	require.True(t, SettleFailedTextStream(c, info))
	info.Billing.Refund(c)
	var user model.User
	var token model.Token
	var sub model.UserSubscription
	require.NoError(t, model.DB.First(&user, 731).Error)
	require.NoError(t, model.DB.First(&token, 732).Error)
	require.NoError(t, model.DB.First(&sub, 733).Error)
	assert.Equal(t, 1000, user.Quota)
	assert.Equal(t, int64(40), sub.AmountUsed)
	assert.Equal(t, 960, token.RemainQuota)
}

func TestFailedTextStreamChargesWalletAndTokenExactlyOnce(t *testing.T) {
	truncate(t)
	seedUser(t, 711, 1000)
	seedToken(t, 712, 711, "partial-test-key", 1000)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	c.Set("request_id", "partial-settlement-test")
	info := &relaycommon.RelayInfo{
		UserId: 711, TokenId: 712, TokenKey: "partial-test-key", OriginModelName: "gpt-4",
		IsStream: true, StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 1},
		BillingSource: BillingSourceWallet,
		PriceData: hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2,
			GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
	}
	funding := &WalletFunding{userId: 711}
	require.NoError(t, funding.PreConsume(100))
	require.NoError(t, model.DecreaseTokenQuota(712, "partial-test-key", 100))
	info.Billing = &BillingSession{relayInfo: info, funding: funding, preConsumedQuota: 100, tokenConsumed: 100}
	RetainPartialStreamUsage(c, info, &dto.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30}, "ignored estimate")
	require.True(t, SettleFailedTextStream(c, info))
	require.True(t, SettleFailedTextStream(c, info))
	info.Billing.Refund(c)
	var user model.User
	var token model.Token
	require.NoError(t, model.DB.First(&user, 711).Error)
	require.NoError(t, model.DB.First(&token, 712).Error)
	assert.Equal(t, 960, user.Quota)
	assert.Equal(t, 40, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	assert.Equal(t, 960, token.RemainQuota)
	assert.Equal(t, 40, token.UsedQuota)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", 711, model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	other, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	admin := other["admin_info"].(map[string]interface{})
	assert.Equal(t, "upstream", admin["partial_stream_billing"].(map[string]interface{})["usage_source"])
}

func TestPartialUsageIgnoresHeartbeatAndAudioPreservesTrustedCounters(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	RetainPartialStreamUsage(c, info, nil, "")
	assert.Nil(t, info.PartialStreamUsage)
	usage := &dto.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30}
	usage.PromptTokensDetails.CachedTokens = 15
	RetainPartialStreamUsage(c, info, usage, "extra text that must not replace upstream usage")
	require.NotNil(t, info.PartialStreamUsage)
	assert.Equal(t, 10, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, 15, info.PartialStreamUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, "upstream", info.PartialStreamUsageSource)
	info.PartialStreamUsage = nil
	info.UpstreamModelName = "gpt-4o-audio"
	RetainPartialStreamUsage(c, info, usage, "audio text")
	assert.Nil(t, info.PartialStreamUsage)
}
