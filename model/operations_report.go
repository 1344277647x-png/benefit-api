/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const operationsReportMaxLogRows = 200000

// OperationsReportParams is intentionally limited to dimensions that can be
// filtered without exposing the raw log table to the administrator UI.
type OperationsReportParams struct {
	StartTimestamp int64
	EndTimestamp   int64
	ModelName      string
	Username       string
	Group          string
	ChannelID      int
	GroupBy        string
}

type OperationsReportSummary struct {
	RequestCount        int64    `json:"request_count"`
	SuccessCount        int64    `json:"success_count"`
	ErrorCount          int64    `json:"error_count"`
	ErrorRate           float64  `json:"error_rate"`
	TokenCount          int64    `json:"token_count"`
	RevenueQuota        int64    `json:"revenue_quota"`
	RefundQuota         int64    `json:"refund_quota"`
	RevenueUSD          float64  `json:"revenue_usd"`
	RefundUSD           float64  `json:"refund_usd"`
	CostUSD             float64  `json:"cost_usd"`
	CostStatus          string   `json:"cost_status"`
	UnknownCostRequests int64    `json:"unknown_cost_requests"`
	ProfitUSD           *float64 `json:"profit_usd,omitempty"`
}

type OperationsReportRow struct {
	Key                 string   `json:"key"`
	Label               string   `json:"label"`
	ModelName           string   `json:"model_name,omitempty"`
	ChannelID           int      `json:"channel_id,omitempty"`
	ChannelName         string   `json:"channel_name,omitempty"`
	UserID              int      `json:"user_id,omitempty"`
	Username            string   `json:"username,omitempty"`
	Group               string   `json:"group,omitempty"`
	RequestCount        int64    `json:"request_count"`
	SuccessCount        int64    `json:"success_count"`
	ErrorCount          int64    `json:"error_count"`
	ErrorRate           float64  `json:"error_rate"`
	TokenCount          int64    `json:"token_count"`
	RevenueQuota        int64    `json:"revenue_quota"`
	RefundQuota         int64    `json:"refund_quota"`
	RevenueUSD          float64  `json:"revenue_usd"`
	RefundUSD           float64  `json:"refund_usd"`
	CostUSD             float64  `json:"cost_usd"`
	CostStatus          string   `json:"cost_status"`
	UnknownCostRequests int64    `json:"unknown_cost_requests"`
	ProfitUSD           *float64 `json:"profit_usd,omitempty"`
}

type OperationsReportChannel struct {
	ChannelID    int     `json:"channel_id"`
	Name         string  `json:"name"`
	Balance      float64 `json:"balance"`
	UsedQuota    int64   `json:"used_quota"`
	Status       int     `json:"status"`
	ResponseTime int     `json:"response_time"`
}

type OperationsReportHealth struct {
	ChannelID        int     `json:"channel_id"`
	ModelName        string  `json:"model"`
	Status           string  `json:"status"`
	RequestCount     int64   `json:"request_count"`
	SuccessRate      float64 `json:"success_rate"`
	AverageLatencyMs int64   `json:"average_latency_ms"`
	AverageTTFTMs    int64   `json:"average_ttft_ms"`
}

type OperationsReport struct {
	GeneratedAt    int64                     `json:"generated_at"`
	StartTimestamp int64                     `json:"start_timestamp"`
	EndTimestamp   int64                     `json:"end_timestamp"`
	GroupBy        string                    `json:"group_by"`
	Truncated      bool                      `json:"truncated"`
	QuotaPerUnit   float64                   `json:"quota_per_unit"`
	Summary        OperationsReportSummary   `json:"summary"`
	Rows           []OperationsReportRow     `json:"rows"`
	ChannelIDs     []int                     `json:"channel_ids"`
	Channels       []OperationsReportChannel `json:"channels"`
	Health         []OperationsReportHealth  `json:"health"`
	Warnings       []string                  `json:"warnings,omitempty"`
}

// operationsReportLog is the smallest log projection needed for reporting.
// Keeping this separate from Log prevents accidental loading of large content
// fields when an administrator opens a long time range.
type operationsReportLog struct {
	UserId           int    `gorm:"column:user_id"`
	Username         string `gorm:"column:username"`
	CreatedAt        int64  `gorm:"column:created_at"`
	Type             int    `gorm:"column:type"`
	ModelName        string `gorm:"column:model_name"`
	Quota            int    `gorm:"column:quota"`
	PromptTokens     int    `gorm:"column:prompt_tokens"`
	CompletionTokens int    `gorm:"column:completion_tokens"`
	ChannelId        int    `gorm:"column:channel_id"`
	Group            string `gorm:"column:group_value"`
	Other            string `gorm:"column:other"`
}

type operationsReportAccumulator struct {
	OperationsReportRow
	knownCostRequests int64
}

func normalizeOperationsReportGroupBy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "model", "channel", "user", "group":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "day"
	}
}

func operationReportCostValue(other string) (float64, bool) {
	if strings.TrimSpace(other) == "" {
		return 0, false
	}
	values, err := common.StrToMap(other)
	if err != nil {
		return 0, false
	}
	// These keys are deliberately explicit. A generic "cost" value is only
	// accepted when it is a scalar USD amount written by an upstream adaptor.
	for _, key := range []string{
		"upstream_cost_usd",
		"provider_cost_usd",
		"cost_usd",
		"upstream_cost",
		"provider_cost",
	} {
		value, ok := values[key]
		if !ok {
			continue
		}
		var cost float64
		switch typed := value.(type) {
		case float64:
			cost = typed
		case float32:
			cost = float64(typed)
		case int:
			cost = float64(typed)
		case int64:
			cost = float64(typed)
		case string:
			parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(typed), 64)
			if parseErr != nil {
				continue
			}
			cost = parsed
		default:
			continue
		}
		if cost >= 0 && !math.IsNaN(cost) && !math.IsInf(cost, 0) {
			return cost, true
		}
	}
	return 0, false
}

func operationReportRowKey(log operationsReportLog, groupBy string) (string, string) {
	switch groupBy {
	case "model":
		label := log.ModelName
		if label == "" {
			label = "(unknown)"
		}
		return "model:" + label, label
	case "channel":
		label := log.Group
		if log.ChannelId > 0 {
			label = strconv.Itoa(log.ChannelId)
		} else if label == "" {
			label = "(unknown)"
		}
		return "channel:" + label, label
	case "user":
		label := log.Username
		if label == "" {
			label = strconv.Itoa(log.UserId)
		}
		return "user:" + label, label
	case "group":
		label := log.Group
		if label == "" {
			label = "(default)"
		}
		return "group:" + label, label
	default:
		label := time.Unix(log.CreatedAt, 0).UTC().Format("2006-01-02")
		return "day:" + label, label
	}
}

func addOperationReportCost(row *operationsReportAccumulator, log operationsReportLog) {
	if log.Type != LogTypeConsume {
		return
	}
	cost, known := operationReportCostValue(log.Other)
	if known {
		row.CostUSD += cost
		row.knownCostRequests++
		return
	}
	row.UnknownCostRequests++
}

func finalizeOperationsReportRow(row *operationsReportAccumulator, quotaPerUnit float64) OperationsReportRow {
	result := row.OperationsReportRow
	if result.RequestCount > 0 {
		result.ErrorRate = float64(result.ErrorCount) * 100 / float64(result.RequestCount)
	}
	result.CostStatus = "unknown"
	if row.knownCostRequests > 0 && row.UnknownCostRequests == 0 {
		result.CostStatus = "known"
	} else if row.knownCostRequests > 0 {
		result.CostStatus = "partial"
	}
	if quotaPerUnit > 0 && result.CostStatus == "known" {
		profit := result.RevenueUSD - result.RefundUSD - result.CostUSD
		result.ProfitUSD = &profit
	}
	return result
}

func aggregateOperationsReport(logs []operationsReportLog, params OperationsReportParams) *OperationsReport {
	groupBy := normalizeOperationsReportGroupBy(params.GroupBy)
	quotaPerUnit := common.QuotaPerUnit
	if quotaPerUnit <= 0 {
		quotaPerUnit = 1
	}
	rowsByKey := make(map[string]*operationsReportAccumulator)
	summary := operationsReportAccumulator{}
	channelIDs := make(map[int]struct{})

	for _, log := range logs {
		if log.ChannelId > 0 {
			channelIDs[log.ChannelId] = struct{}{}
		}
		key, label := operationReportRowKey(log, groupBy)
		row, ok := rowsByKey[key]
		if !ok {
			row = &operationsReportAccumulator{OperationsReportRow: OperationsReportRow{
				Key:   key,
				Label: label,
			}}
			switch groupBy {
			case "model":
				row.ModelName = log.ModelName
			case "channel":
				row.ChannelID = log.ChannelId
			case "user":
				row.UserID = log.UserId
				row.Username = log.Username
			case "group":
				row.Group = log.Group
			}
			rowsByKey[key] = row
		}
		for _, target := range []*operationsReportAccumulator{row, &summary} {
			switch log.Type {
			case LogTypeConsume:
				target.RequestCount++
				target.SuccessCount++
				target.TokenCount += int64(log.PromptTokens) + int64(log.CompletionTokens)
				if log.Quota > 0 {
					target.RevenueQuota += int64(log.Quota)
				}
			case LogTypeRefund:
				if log.Quota > 0 {
					target.RefundQuota += int64(log.Quota)
				}
			case LogTypeError:
				target.RequestCount++
				target.ErrorCount++
			}
		}
		addOperationReportCost(row, log)
		addOperationReportCost(&summary, log)
	}

	toUSD := func(quota int64) float64 { return float64(quota) / quotaPerUnit }
	rows := make([]OperationsReportRow, 0, len(rowsByKey))
	for _, row := range rowsByKey {
		row.RevenueUSD = toUSD(row.RevenueQuota)
		row.RefundUSD = toUSD(row.RefundQuota)
		rows = append(rows, finalizeOperationsReportRow(row, quotaPerUnit))
	}
	sort.Slice(rows, func(i, j int) bool {
		if groupBy == "day" {
			return rows[i].Label < rows[j].Label
		}
		if rows[i].RevenueQuota == rows[j].RevenueQuota {
			return rows[i].Label < rows[j].Label
		}
		return rows[i].RevenueQuota > rows[j].RevenueQuota
	})

	summary.RevenueUSD = toUSD(summary.RevenueQuota)
	summary.RefundUSD = toUSD(summary.RefundQuota)
	resultSummary := finalizeOperationsReportRow(&summary, quotaPerUnit)
	summaryView := OperationsReportSummary{
		RequestCount:        resultSummary.RequestCount,
		SuccessCount:        resultSummary.SuccessCount,
		ErrorCount:          resultSummary.ErrorCount,
		ErrorRate:           resultSummary.ErrorRate,
		TokenCount:          resultSummary.TokenCount,
		RevenueQuota:        resultSummary.RevenueQuota,
		RefundQuota:         resultSummary.RefundQuota,
		RevenueUSD:          resultSummary.RevenueUSD,
		RefundUSD:           resultSummary.RefundUSD,
		CostUSD:             resultSummary.CostUSD,
		CostStatus:          resultSummary.CostStatus,
		UnknownCostRequests: resultSummary.UnknownCostRequests,
		ProfitUSD:           resultSummary.ProfitUSD,
	}
	ids := make([]int, 0, len(channelIDs))
	for id := range channelIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return &OperationsReport{
		GeneratedAt:    time.Now().Unix(),
		StartTimestamp: params.StartTimestamp,
		EndTimestamp:   params.EndTimestamp,
		GroupBy:        groupBy,
		QuotaPerUnit:   quotaPerUnit,
		Summary:        summaryView,
		Rows:           rows,
		ChannelIDs:     ids,
	}
}

// GetOperationsReport reads a bounded log window and aggregates it in Go.
// This keeps the endpoint compatible with SQLite, MySQL, PostgreSQL and the
// optional ClickHouse log database while making the cost-unknown state explicit.
func GetOperationsReport(params OperationsReportParams) (*OperationsReport, error) {
	if LOG_DB == nil {
		return nil, errors.New("log database is not initialized")
	}
	if params.StartTimestamp <= 0 || params.EndTimestamp <= 0 || params.EndTimestamp < params.StartTimestamp {
		return nil, errors.New("invalid report time range")
	}
	if params.EndTimestamp-params.StartTimestamp > 366*24*60*60 {
		return nil, errors.New("report time range cannot exceed 366 days")
	}
	params.GroupBy = normalizeOperationsReportGroupBy(params.GroupBy)
	query := LOG_DB.Table("logs").Select("user_id, username, created_at, type, model_name, quota, prompt_tokens, completion_tokens, channel_id, "+logGroupCol+" AS group_value, other").
		Where("created_at >= ? AND created_at <= ?", params.StartTimestamp, params.EndTimestamp).
		Where("type IN ?", []int{LogTypeConsume, LogTypeRefund, LogTypeError})
	if params.ModelName != "" {
		query = query.Where("model_name = ?", params.ModelName)
	}
	if params.Username != "" {
		query = query.Where("username = ?", params.Username)
	}
	if params.Group != "" {
		query = query.Where(logGroupCol+" = ?", params.Group)
	}
	if params.ChannelID > 0 {
		query = query.Where("channel_id = ?", params.ChannelID)
	}

	var logs []operationsReportLog
	if err := query.Order("created_at ASC").Limit(operationsReportMaxLogRows + 1).Find(&logs).Error; err != nil {
		return nil, fmt.Errorf("query operation report logs: %w", err)
	}
	truncated := len(logs) > operationsReportMaxLogRows
	if truncated {
		logs = logs[:operationsReportMaxLogRows]
	}
	report := aggregateOperationsReport(logs, params)
	report.Truncated = truncated
	return report, nil
}
