package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// RetainPartialStreamUsage preserves protocol-specific evidence, not raw bodies.
// Empty lifecycle events and heartbeats cannot create a charge. Audio remains
// outside this text-stream repair; its accounting requires a separate policy.
func RetainPartialStreamUsage(c *gin.Context, info *relaycommon.RelayInfo, usage *dto.Usage, generatedText string) {
	if info == nil || !info.IsStream || strings.Contains(strings.ToLower(info.OriginModelName), "audio") ||
		strings.Contains(strings.ToLower(info.UpstreamModelName), "audio") {
		return
	}
	if usage != nil && (usage.PromptTokensDetails.AudioTokens > 0 || usage.CompletionTokenDetails.AudioTokens > 0) {
		return
	}
	hasPricedToolCall := false
	if info.ResponsesUsageInfo != nil {
		for _, tool := range info.ResponsesUsageInfo.BuiltInTools {
			if tool != nil && tool.CallCount > 0 && tool.ToolName != dto.BuildInToolImageGeneration {
				hasPricedToolCall = true
			}
		}
	}
	if !ValidUsage(usage) && generatedText == "" && !hasPricedToolCall {
		return
	}
	if usage == nil {
		usage = &dto.Usage{}
	}
	copyUsage := *usage
	info.PartialStreamUsageSource = "upstream"
	if !ValidUsage(usage) && generatedText == "" {
		info.PartialStreamUsageSource = "observed_tool_calls"
	}
	if copyUsage.CompletionTokens == 0 && generatedText != "" {
		copyUsage.CompletionTokens = EstimateTokenByModel(info.UpstreamModelName, generatedText)
		if copyUsage.PromptTokens == 0 {
			copyUsage.PromptTokens = info.GetEstimatePromptTokens()
		}
		// Do not let an incomplete billing override discard the estimated output.
		copyUsage.BillingUsage = nil
		info.PartialStreamUsageSource = "estimated"
		common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	}
	copyUsage.TotalTokens = copyUsage.PromptTokens + copyUsage.CompletionTokens
	info.PartialStreamUsage = &copyUsage
}

// SettleFailedTextStream keeps failure handling separate from consumption.
// Returning true suppresses the blanket refund even if settlement fails: a
// funding failure must retain the reservation for reconciliation, not turn
// delivered output into a free request.
func SettleFailedTextStream(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info == nil || info.PartialStreamUsage == nil {
		return false
	}
	if c.GetBool("partial_stream_billing_attempted") {
		return true
	}
	c.Set("partial_stream_billing_attempted", true)
	PostTextConsumeQuota(c, info, info.PartialStreamUsage, []string{"流式请求未完成，按已产生用量结算"})
	return true
}
