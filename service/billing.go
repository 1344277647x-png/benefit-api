package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	BillingSourceWallet       = "wallet"
	BillingSourceSubscription = "subscription"
	BillingSourceTeam         = "team"
)

// PreConsumeBilling 根据用户计费偏好创建 BillingSession 并执行预扣费。
// 会话存储在 relayInfo.Billing 上，供后续 Settle / Refund 使用。
func PreConsumeBilling(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	if relayInfo != nil && relayInfo.QuotaClamp != nil {
		return types.NewErrorWithStatusCode(
			relayInfo.QuotaClamp,
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("pre-consume quota cannot be negative: %d", preConsumedQuota),
			types.ErrorCodeModelPriceError,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}
	session, apiErr := NewBillingSession(c, relayInfo, preConsumedQuota)
	if apiErr != nil {
		return apiErr
	}
	relayInfo.Billing = session
	return nil
}

// ---------------------------------------------------------------------------
// SettleBilling — 后结算辅助函数
// ---------------------------------------------------------------------------

// SettleBilling 执行计费结算。如果 RelayInfo 上有 BillingSession 则通过 session 结算，
// 否则回退到旧的 PostConsumeQuota 路径（兼容按次计费等场景）。
func SettleBilling(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, actualQuota int) error {
	if relayInfo.Billing != nil {
		preConsumed := relayInfo.Billing.GetPreConsumedQuota()
		delta := actualQuota - preConsumed

		if delta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else if delta < 0 {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
				logger.FormatQuota(-delta),
				logger.FormatQuota(actualQuota),
				logger.FormatQuota(preConsumed),
			))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("预扣费与实际消耗一致，无需调整：%s（按次计费）",
				logger.FormatQuota(actualQuota),
			))
		}

		if err := relayInfo.Billing.Settle(actualQuota); err != nil {
			return err
		}
		if relayInfo.BillingSource == BillingSourceTeam && !relayInfo.ForcePreConsume {
			if err := DeliverPendingTeamSyncBillingEvents(ctx, 100); err != nil {
				logger.LogError(ctx, "team sync billing log delivery deferred: "+err.Error())
			}
		}

		// 发送额度通知（订阅计费使用订阅剩余额度）
		if actualQuota != 0 {
			if relayInfo.BillingSource == BillingSourceSubscription {
				checkAndSendSubscriptionQuotaNotify(relayInfo)
			} else if relayInfo.BillingSource != BillingSourceTeam {
				checkAndSendQuotaNotify(relayInfo, actualQuota-preConsumed, preConsumed)
			}
		}
		return nil
	}

	// 回退：无 BillingSession 时使用旧路径
	quotaDelta := actualQuota - relayInfo.FinalPreConsumedQuota
	if quotaDelta != 0 {
		return PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
	}
	return nil
}

func DeliverPendingTeamSyncBillingEvents(ctx context.Context, limit int) error {
	events, err := model.PendingTeamSyncBillingEvents(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := model.DeliverTeamSyncBillingLog(ctx, event); err != nil {
			return err
		}
		if err := model.MarkTeamSyncBillingEventDelivered(ctx, event.RequestId); err != nil {
			return err
		}
	}
	return nil
}

func DeliverPendingBusinessEvents(ctx context.Context, limit int) error {
	events, err := model.PendingBusinessEvents(ctx, limit)
	if err != nil {
		return err
	}
	var deliveryErr error
	for _, event := range events {
		if err := model.DeliverBusinessEvent(ctx, event); err != nil {
			deliveryErr = err
			if markErr := model.MarkBusinessEventFailed(ctx, event); markErr != nil {
				return markErr
			}
			logger.LogError(ctx, fmt.Sprintf("business event delivery deferred: id=%d attempt=%d dead_letter=%t", event.Id, event.Attempts+1, event.Attempts+1 >= 10))
			continue
		}
		if err := model.MarkBusinessEventDelivered(ctx, event.EventKey); err != nil {
			deliveryErr = err
		}
	}
	return deliveryErr
}

// Keep only known, non-content pricing and usage fields in the durable event.
// Provider error text, request bodies and channel credentials must not be
// copied into the main billing database. Bounded administrator-authored pricing
// expressions and their matched rule traces are configuration, not input.
func teamBillingLogDetails(other map[string]interface{}) map[string]interface{} {
	allowed := []string{
		"model_ratio", "group_ratio", "completion_ratio", "model_price", "user_group_ratio",
		"cache_tokens", "cache_ratio", "cache_creation_tokens", "cache_creation_ratio",
		"cache_creation_tokens_5m", "cache_creation_ratio_5m", "cache_creation_tokens_1h",
		"cache_creation_ratio_1h", "cache_write_tokens", "input_tokens_total",
		"image_ratio", "image_output", "audio_input", "audio_output", "text_input", "text_output",
		"audio_ratio", "audio_completion_ratio", "audio_input_token_count", "audio_input_price",
	}
	details := make(map[string]interface{}, len(allowed)+3)
	for _, key := range allowed {
		if value, ok := other[key]; ok {
			switch v := value.(type) {
			case int, int64:
				details[key] = value
			case float64:
				if !math.IsNaN(v) && !math.IsInf(v, 0) {
					details[key] = v
				}
			case float32:
				if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) {
					details[key] = v
				}
			}
		}
	}
	if other["billing_mode"] == "tiered_expr" {
		details["billing_mode"] = "tiered_expr"
		snapshotIncomplete := false
		// The normal consume log uses this frozen, administrator-authored
		// expression to render the historical tier prices. Preserve the same
		// bounded configuration snapshot across independent log DB failures;
		// never copy request headers, input text or provider payloads.
		if encoded, ok := other["expr_b64"].(string); ok && len(encoded) <= 8192 {
			if expression, err := base64.StdEncoding.DecodeString(encoded); err == nil && len(expression) > 0 && len(expression) <= 6144 {
				details["expr_b64"] = encoded
			}
		}
		if _, ok := details["expr_b64"]; !ok {
			snapshotIncomplete = true
		}
		if value, ok := other["matched_tier"]; ok {
			switch v := value.(type) {
			case int, int64:
				details["matched_tier"] = value
			case float64:
				if !math.IsNaN(v) && !math.IsInf(v, 0) {
					details["matched_tier"] = v
				}
			case float32:
				if !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0) {
					details["matched_tier"] = v
				}
			case string:
				if len(v) <= 64 {
					details["matched_tier"] = v
				}
			}
		}
		if rawRules, exists := other["request_rules"]; exists {
			if rules, ok := rawRules.([]billingexpr.RequestRuleTrace); ok && len(rules) <= 16 {
				valid := true
				for _, rule := range rules {
					if len(rule.Cond) > 256 || rule.Multiplier <= 0 || math.IsNaN(rule.Multiplier) || math.IsInf(rule.Multiplier, 0) {
						valid = false
						break
					}
				}
				if valid {
					details["request_rules"] = rules
				}
			}
			if _, ok := details["request_rules"]; !ok {
				snapshotIncomplete = true
			}
		}
		if snapshotIncomplete {
			details["pricing_snapshot_incomplete"] = true
		}
	}
	if other["usage_semantic"] == "anthropic" {
		details["usage_semantic"] = "anthropic"
	}
	if admin, ok := other["admin_info"].(map[string]interface{}); ok {
		if saturation, ok := admin["quota_saturation"].(map[string]interface{}); ok {
			// NaN/Inf must not make the settlement event unmarshalable.
			anomaly := map[string]interface{}{}
			for _, key := range []string{"op", "kind"} {
				if value, ok := saturation[key].(string); ok && len(value) <= 64 {
					anomaly[key] = value
				}
			}
			if value, ok := saturation["clamped"].(int); ok {
				anomaly["clamped"] = value
			}
			if value, ok := saturation["original"].(float64); ok && !math.IsNaN(value) && !math.IsInf(value, 0) {
				anomaly["original"] = value
			}
			details["admin_info"] = map[string]interface{}{"quota_saturation": anomaly}
		}
	}
	return details
}
