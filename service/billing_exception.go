package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func recordPartialSettlementException(c *gin.Context, info *relaycommon.RelayInfo, actualQuota int) {
	requestID := info.RequestId
	if requestID == "" {
		requestID = c.GetString(common.RequestIdKey)
	}
	if requestID == "" {
		requestID = common.NewRequestId()
		info.RequestId = requestID
	}
	stage := "unknown"
	reserved := info.FinalPreConsumedQuota
	tokenReserved := 0
	if session, ok := info.Billing.(*BillingSession); ok {
		session.mu.Lock()
		reserved = session.preConsumedQuota
		tokenReserved = session.tokenConsumed
		stage = "funding_settlement_failed"
		if session.fundingSettled {
			stage = "token_adjustment_failed"
		}
		session.mu.Unlock()
	}
	details := map[string]any{"admin_info": map[string]any{
		"billing_exception": true, "status": "pending_manual_review", "request_id": requestID,
		"user_id": info.UserId, "token_id": info.TokenId, "funding_source": info.BillingSource,
		"subscription_id": info.SubscriptionId, "model": info.OriginModelName,
		"reserved_quota": reserved, "token_reserved_quota": tokenReserved,
		"expected_quota": actualQuota, "failure_stage": stage, "usage_source": info.PartialStreamUsageSource,
	}}
	event := &model.BusinessEvent{EventKey: "billing-exception:" + requestID, Category: model.BusinessEventBilling,
		Action: "partial_settlement_failed", UserId: info.UserId, RequestId: requestID,
		Content: fmt.Sprintf("流式部分结算失败，待人工对账；预扣=%d，令牌预扣=%d，应结算=%d，阶段=%s（额度单位，非成功扣费）", reserved, tokenReserved, actualQuota, stage)}
	if err := model.EnqueueBillingException(event, details); err != nil {
		event.DetailsJSON = common.MapToJsonStr(details)
		// Main DB may be the failed dependency. Try the independent log DB
		// without the cancelled client context, and always emit an alert.
		logger.LogError(c, "billing exception persistence failed; request="+common.LocalLogPreview(requestID))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := model.DeliverBusinessEvent(ctx, *event); err != nil {
			logger.LogError(c, "billing exception fallback failed; manual reconciliation required")
		}
		return
	}
	NotifyBusinessEventDelivery()
}
