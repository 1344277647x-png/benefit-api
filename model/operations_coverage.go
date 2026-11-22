package model

import (
	"github.com/QuantumNous/new-api/common"
	"math"
)

// Coverage joins income and cost on each consumption record, never on totals.
type OperationsCoverage struct {
	MatchedRequests    int64                      `json:"matched_requests"`
	PendingRequests    int64                      `json:"pending_requests"`
	MatchedQuota       int64                      `json:"matched_quota"`
	PartialProfitUSD   *float64                   `json:"partial_profit_usd"`
	ErrorsWithoutQuota int64                      `json:"errors_without_quota"`
	Reasons            map[string]int64           `json:"reasons"`
	PendingSamples     []OperationsPendingRequest `json:"pending_samples"`
}

type OperationsPendingRequest struct {
	LogID     int      `json:"log_id"`
	RequestID string   `json:"request_id"`
	ChannelID int      `json:"channel_id"`
	CreatedAt int64    `json:"created_at"`
	Reasons   []string `json:"reasons"`
}

func addOperationsCoverage(target *OperationsCoverage, log operationsReportLog) {
	if log.Type != LogTypeConsume && log.Type != LogTypeError {
		return
	}
	other, _ := common.StrToMap(log.Other)
	income, hasIncome := other["operations_income_usd"].(float64)
	hasIncome = hasIncome && income >= 0 && !math.IsNaN(income) && !math.IsInf(income, 0) && other["operations_income_known"] == true
	cost, hasCost := operationReportCostValue(log.Other)
	if log.Type == LogTypeConsume && hasIncome && hasCost {
		target.MatchedRequests++
		if log.Quota > 0 {
			target.MatchedQuota += int64(log.Quota)
		}
		if target.PartialProfitUSD == nil {
			target.PartialProfitUSD = new(float64)
		}
		*target.PartialProfitUSD += income - cost
		return
	}
	target.PendingRequests++
	reasons := []string{}
	if log.Type == LogTypeError {
		if log.Quota <= 0 {
			target.ErrorsWithoutQuota++
		}
		reasons = append(reasons, "failed_request_unverified")
	} else if !hasIncome {
		reason := log.IncomeReason
		if reason == "" {
			reason = "income_record_missing"
		}
		reasons = append(reasons, reason)
	}
	if !hasCost {
		reason := log.CostReason
		if reason == "" {
			reason = "billing_metadata_incomplete"
		}
		reasons = append(reasons, reason)
	}
	if target.Reasons == nil {
		target.Reasons = map[string]int64{}
	}
	for _, reason := range reasons {
		target.Reasons[reason]++
	}
	// Bound response size even when a time window contains 200,000 logs.
	if len(target.PendingSamples) < 20 {
		target.PendingSamples = append(target.PendingSamples, OperationsPendingRequest{LogID: log.ID, RequestID: log.RequestId, ChannelID: log.ChannelId, CreatedAt: log.CreatedAt, Reasons: reasons})
	}
}
