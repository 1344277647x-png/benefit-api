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
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	billingReconciliationDefaultLimit = 200
	billingReconciliationMaxLimit     = 1000
	billingReconciliationMaxDays      = 366
)

const (
	ReconciliationKindAll                        = "all"
	ReconciliationKindSettlementFailed           = "settlement_failed"
	ReconciliationKindPartialSettlementFailed    = "partial_settlement_failed"
	ReconciliationKindAccountingProjectionFailed = "accounting_projection_failed"
	ReconciliationKindUnsettledPreConsume        = "unsettled_pre_consume"
)

type BillingReconciliationParams struct {
	StartTimestamp int64
	EndTimestamp   int64
	UserID         int
	RequestID      string
	Kind           string
	Limit          int
}

type BillingReconciliationItem struct {
	Source           string   `json:"source"`
	Kind             string   `json:"kind"`
	EventKey         string   `json:"event_key,omitempty"`
	RequestId        string   `json:"request_id"`
	UserId           int      `json:"user_id"`
	Username         string   `json:"username,omitempty"`
	SubscriptionId   int      `json:"subscription_id,omitempty"`
	FundingSource    string   `json:"funding_source,omitempty"`
	ModelName        string   `json:"model_name,omitempty"`
	FailureStage     string   `json:"failure_stage,omitempty"`
	Status           string   `json:"status"`
	PreConsumedQuota int64    `json:"pre_consumed_quota"`
	ActualQuota      *int64   `json:"actual_quota,omitempty"`
	DiffQuota        *int64   `json:"diff_quota,omitempty"`
	TokenDiffQuota   *int64   `json:"token_diff_quota,omitempty"`
	ProjectedQuota   *int64   `json:"projected_quota,omitempty"`
	PreConsumedUSD   float64  `json:"pre_consumed_usd"`
	ActualUSD        *float64 `json:"actual_usd,omitempty"`
	DiffUSD          *float64 `json:"diff_usd,omitempty"`
	HasConsumeLog    bool     `json:"has_consume_log"`
	DeliveredAt      int64    `json:"delivered_at,omitempty"`
	DeadLetterAt     int64    `json:"dead_letter_at,omitempty"`
	CreatedAt        int64    `json:"created_at"`
	Summary          string   `json:"summary"`
}

type BillingReconciliationSummary struct {
	TotalItems               int     `json:"total_items"`
	SettlementExceptionCount int     `json:"settlement_exception_count"`
	ProjectionFailureCount   int     `json:"projection_failure_count"`
	UnsettledPreConsumeCount int     `json:"unsettled_pre_consume_count"`
	UnknownActualCount       int     `json:"unknown_actual_count"`
	KnownPreConsumedQuota    int64   `json:"known_pre_consumed_quota"`
	KnownActualQuota         int64   `json:"known_actual_quota"`
	KnownNetDiffQuota        int64   `json:"known_net_diff_quota"`
	UnknownPreConsumedQuota  int64   `json:"unknown_pre_consumed_quota"`
	KnownPreConsumedUSD      float64 `json:"known_pre_consumed_usd"`
	KnownActualUSD           float64 `json:"known_actual_usd"`
	KnownNetDiffUSD          float64 `json:"known_net_diff_usd"`
	UnknownPreConsumedUSD    float64 `json:"unknown_pre_consumed_usd"`
}

type BillingReconciliationReport struct {
	ReadOnly       bool                         `json:"read_only"`
	GeneratedAt    int64                        `json:"generated_at"`
	StartTimestamp int64                        `json:"start_timestamp"`
	EndTimestamp   int64                        `json:"end_timestamp"`
	Kind           string                       `json:"kind"`
	QuotaPerUnit   float64                      `json:"quota_per_unit"`
	Truncated      bool                         `json:"truncated"`
	Summary        BillingReconciliationSummary `json:"summary"`
	Items          []BillingReconciliationItem  `json:"items"`
}

func NormalizeBillingReconciliationKind(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ReconciliationKindAll:
		return ReconciliationKindAll, nil
	case ReconciliationKindSettlementFailed:
		return ReconciliationKindSettlementFailed, nil
	case ReconciliationKindPartialSettlementFailed:
		return ReconciliationKindPartialSettlementFailed, nil
	case ReconciliationKindAccountingProjectionFailed:
		return ReconciliationKindAccountingProjectionFailed, nil
	case ReconciliationKindUnsettledPreConsume:
		return ReconciliationKindUnsettledPreConsume, nil
	default:
		return "", errors.New("invalid reconciliation kind")
	}
}

func jsonInt64Value(m map[string]any, key string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func jsonStringValue(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// GetBillingReconciliation builds a strictly read-only reconciliation view of
// billing settlement failures, accounting projection failures, and unsettled
// subscription pre-consume records. It never writes to wallets, subscriptions,
// logs, or outbox tables.
func GetBillingReconciliation(ctx context.Context, params BillingReconciliationParams) (*BillingReconciliationReport, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	if params.StartTimestamp <= 0 || params.EndTimestamp <= 0 || params.EndTimestamp < params.StartTimestamp {
		return nil, errors.New("invalid reconciliation time range")
	}
	if params.EndTimestamp-params.StartTimestamp > billingReconciliationMaxDays*24*60*60 {
		return nil, errors.New("reconciliation time range cannot exceed 366 days")
	}
	kind, err := NormalizeBillingReconciliationKind(params.Kind)
	if err != nil {
		return nil, err
	}
	limit := params.Limit
	if limit <= 0 {
		limit = billingReconciliationDefaultLimit
	}
	if limit > billingReconciliationMaxLimit {
		limit = billingReconciliationMaxLimit
	}
	reqFilter := strings.TrimSpace(params.RequestID)

	quotaPerUnit := common.QuotaPerUnit
	if quotaPerUnit <= 0 {
		quotaPerUnit = 1
	}
	toUSD := func(q int64) float64 {
		return float64(q) / quotaPerUnit
	}

	db := DB.WithContext(ctx)
	items := make([]BillingReconciliationItem, 0)
	truncated := false
	coveredSettlementRequests := make(map[string]struct{})

	wantBusinessEvents := kind == ReconciliationKindAll ||
		kind == ReconciliationKindSettlementFailed ||
		kind == ReconciliationKindPartialSettlementFailed ||
		kind == ReconciliationKindAccountingProjectionFailed

	if wantBusinessEvents {
		actions := billingReconciliationEventActions
		if kind != ReconciliationKindAll {
			actions = []string{kind}
		}
		q := db.Model(&BusinessEvent{}).
			Where("category = ? AND action IN ?", BusinessEventBilling, actions).
			Where("created_at >= ? AND created_at <= ?", params.StartTimestamp, params.EndTimestamp)
		if params.UserID > 0 {
			q = q.Where("user_id = ?", params.UserID)
		}
		if reqFilter != "" {
			q = q.Where("request_id = ?", reqFilter)
		}
		var events []BusinessEvent
		if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&events).Error; err != nil {
			return nil, fmt.Errorf("query billing reconciliation events: %w", err)
		}
		if len(events) > limit {
			truncated = true
			events = events[:limit]
		}
		// Direct log delivery is the durable fallback when the primary outbox insert fails.
		seen := make(map[string]bool, len(events))
		for _, event := range events {
			seen[event.EventKey] = true
		}
		fromLog := map[string]bool{}
		if LOG_DB == nil {
			return nil, errors.New("billing reconciliation incomplete: log database unavailable")
		}
		logQuery := LOG_DB.WithContext(ctx).Model(&Log{}).
			Where("type = ? AND created_at BETWEEN ? AND ? AND other LIKE ?", LogTypeError, params.StartTimestamp, params.EndTimestamp, "%\"business_event\"%")
		if params.UserID > 0 {
			logQuery = logQuery.Where("user_id = ?", params.UserID)
		}
		if reqFilter != "" {
			logQuery = logQuery.Where("request_id = ?", reqFilter)
		}
		found := 0
		var beforeTime int64
		var beforeID int
		for found <= limit {
			var logs []Log
			page := logQuery.Session(&gorm.Session{})
			if beforeTime != 0 {
				page = page.Where("(created_at < ? OR (created_at = ? AND id < ?))", beforeTime, beforeTime, beforeID)
			}
			if err := page.Select("id, created_at, user_id, request_id, content, other").Order("created_at DESC, id DESC").Limit(256).Find(&logs).Error; err != nil {
				return nil, fmt.Errorf("billing reconciliation incomplete: query fallback logs: %w", err)
			}
			for _, l := range logs {
				var meta struct {
					Event struct {
						Category string
						Action   string
						Key      string `json:"event_key"`
					} `json:"business_event"`
				}
				if common.UnmarshalJsonStr(l.Other, &meta) != nil || meta.Event.Category != BusinessEventBilling || seen[meta.Event.Key] {
					continue
				}
				if meta.Event.Action != ReconciliationKindSettlementFailed && meta.Event.Action != ReconciliationKindPartialSettlementFailed {
					continue
				}
				if l.RequestId == "" || meta.Event.Key != "billing-exception:"+l.RequestId || (kind != ReconciliationKindAll && kind != meta.Event.Action) {
					continue
				}
				events = append(events, BusinessEvent{EventKey: meta.Event.Key, Action: meta.Event.Action, UserId: l.UserId, RequestId: l.RequestId, CreatedAt: l.CreatedAt, Content: l.Content, DetailsJSON: l.Other})
				seen[meta.Event.Key], fromLog[meta.Event.Key] = true, true
				found++
			}
			if len(logs) < 256 {
				break
			}
			beforeTime, beforeID = logs[len(logs)-1].CreatedAt, logs[len(logs)-1].Id
		}
		projectionIDs := make([]int64, 0)
		for _, event := range events {
			if event.Action == ReconciliationKindAccountingProjectionFailed {
				if id, err := strconv.ParseInt(strings.TrimPrefix(event.EventKey, "operations-pending:"), 10, 64); err == nil {
					projectionIDs = append(projectionIDs, id)
				}
			}
		}
		recovered := map[string]bool{}
		for offset := 0; offset < len(projectionIDs); offset += 500 {
			var ids []int64
			if err := db.Model(&OperationsEvent{}).Where("id IN ? AND done = ?", projectionIDs[offset:min(offset+500, len(projectionIDs))], true).Pluck("id", &ids).Error; err != nil {
				return nil, err
			}
			for _, id := range ids {
				recovered[fmt.Sprintf("operations-pending:%d", id)] = true
			}
		}
		for _, event := range events {
			var payload map[string]any
			if event.DetailsJSON != "" {
				_ = common.UnmarshalJsonStr(event.DetailsJSON, &payload)
			}
			adminInfo, _ := payload["admin_info"].(map[string]any)
			item := BillingReconciliationItem{
				Source:        "business_event",
				Kind:          event.Action,
				EventKey:      event.EventKey,
				RequestId:     event.RequestId,
				UserId:        event.UserId,
				FundingSource: jsonStringValue(adminInfo, "funding_source"),
				ModelName:     jsonStringValue(adminInfo, "model"),
				FailureStage:  jsonStringValue(adminInfo, "failure_stage"),
				Status:        jsonStringValue(adminInfo, "status"),
				DeliveredAt:   event.DeliveredAt,
				DeadLetterAt:  event.DeadLetterAt,
				CreatedAt:     event.CreatedAt,
				Summary:       event.Content,
			}
			if fromLog[event.EventKey] {
				item.Source = "billing_log"
			}
			if recovered[event.EventKey] {
				item.Status = "recovered"
			}
			if subID, ok := jsonInt64Value(adminInfo, "subscription_id"); ok && subID > 0 {
				item.SubscriptionId = int(subID)
			}
			if event.Action == ReconciliationKindAccountingProjectionFailed {
				if item.Status == "" {
					item.Status = "pending_accounting"
				}
				if projected, ok := jsonInt64Value(adminInfo, "quota"); ok {
					item.ProjectedQuota = &projected
				}
			} else {
				if item.Status == "" {
					item.Status = "pending_manual_review"
				}
				if reserved, ok := jsonInt64Value(adminInfo, "reserved_quota"); ok {
					item.PreConsumedQuota = reserved
					item.PreConsumedUSD = toUSD(reserved)
				}
				reserved, hasReserved := jsonInt64Value(adminInfo, "reserved_quota")
				if expected, ok := jsonInt64Value(adminInfo, "expected_quota"); ok && expected >= 0 && hasReserved && reserved >= 0 && (item.FailureStage == "funding_settlement_failed" || item.FailureStage == "token_adjustment_failed") {
					actual := expected
					diff := actual - item.PreConsumedQuota
					if item.FailureStage == "token_adjustment_failed" {
						diff = 0 // The funding transaction committed before the token adjustment.
						if held, ok := jsonInt64Value(adminInfo, "token_reserved_quota"); ok && held >= 0 {
							tokenDiff := actual - held
							item.TokenDiffQuota = &tokenDiff
						}
					}
					actualUSD := toUSD(actual)
					diffUSD := toUSD(diff)
					item.ActualQuota = &actual
					item.DiffQuota = &diff
					item.ActualUSD = &actualUSD
					item.DiffUSD = &diffUSD
				}
				if event.RequestId != "" {
					coveredSettlementRequests[event.RequestId] = struct{}{}
				}
			}
			items = append(items, item)
		}
	}

	wantPreConsume := kind == ReconciliationKindAll || kind == ReconciliationKindUnsettledPreConsume
	if wantPreConsume && db.Migrator().HasTable(&SubscriptionPreConsumeRecord{}) {
		q := db.Model(&SubscriptionPreConsumeRecord{}).
			Where("status = ? AND created_at <= ?", "consumed", common.GetTimestamp()-unsettledPreConsumeGraceSeconds).
			Where("created_at >= ? AND created_at <= ?", params.StartTimestamp, params.EndTimestamp)
		if params.UserID > 0 {
			q = q.Where("user_id = ?", params.UserID)
		}
		if reqFilter != "" {
			q = q.Where("request_id = ?", reqFilter)
		}
		var records []SubscriptionPreConsumeRecord
		if err := q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&records).Error; err != nil {
			return nil, fmt.Errorf("query unsettled subscription pre-consume records: %w", err)
		}
		if len(records) > limit {
			truncated = true
			records = records[:limit]
		}

		// Cross-reference read-only consume logs so unknown actuals remain
		// explicitly unknown instead of assuming net diff = -pre_consumed.
		type consumeLogSnapshot struct {
			RequestId string `gorm:"column:request_id"`
			UserId    int    `gorm:"column:user_id"`
			Quota     int    `gorm:"column:quota"`
			ModelName string `gorm:"column:model_name"`
		}
		consumeByRequest := make(map[string]consumeLogSnapshot)
		if len(records) > 0 {
			if LOG_DB == nil {
				return nil, errors.New("billing reconciliation incomplete: log database unavailable")
			}
			reqIDs := make([]string, 0, len(records))
			for _, rec := range records {
				if rec.RequestId != "" {
					reqIDs = append(reqIDs, rec.RequestId)
				}
			}
			for offset := 0; offset < len(reqIDs); offset += 500 {
				end := min(offset+500, len(reqIDs))
				var logs []consumeLogSnapshot
				if err := LOG_DB.WithContext(ctx).Table("logs").
					Select("request_id, user_id, quota, model_name").
					Where("type = ? AND request_id IN ?", LogTypeConsume, reqIDs[offset:end]).
					Find(&logs).Error; err != nil {
					return nil, fmt.Errorf("billing reconciliation incomplete: query consume logs: %w", err)
				}
				for _, l := range logs {
					consumeByRequest[l.RequestId] = l
				}
			}
		}

		for _, rec := range records {
			if _, covered := coveredSettlementRequests[rec.RequestId]; covered {
				continue
			}
			item := BillingReconciliationItem{
				Source:           "subscription_pre_consume",
				Kind:             ReconciliationKindUnsettledPreConsume,
				RequestId:        rec.RequestId,
				UserId:           rec.UserId,
				SubscriptionId:   rec.UserSubscriptionId,
				FundingSource:    "subscription",
				Status:           "unsettled_pre_consume",
				PreConsumedQuota: rec.PreConsumed,
				PreConsumedUSD:   toUSD(rec.PreConsumed),
				CreatedAt:        rec.CreatedAt,
				Summary:          fmt.Sprintf("Unsettled subscription pre-consume (%d quota); actual consumption unknown", rec.PreConsumed),
			}
			if logSnap, ok := consumeByRequest[rec.RequestId]; ok && logSnap.UserId == rec.UserId {
				actual := int64(logSnap.Quota)
				diff := actual - rec.PreConsumed
				actualUSD := toUSD(actual)
				diffUSD := toUSD(diff)
				item.HasConsumeLog = true
				item.ModelName = logSnap.ModelName
				item.ActualQuota = &actual
				item.DiffQuota = &diff
				item.ActualUSD = &actualUSD
				item.DiffUSD = &diffUSD
				item.Summary = fmt.Sprintf("Unsettled subscription pre-consume (%d) with consume log (%d); diff=%d", rec.PreConsumed, actual, diff)
			}
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].RequestId > items[j].RequestId
		}
		return items[i].CreatedAt > items[j].CreatedAt
	})
	if len(items) > limit {
		truncated = true
		items = items[:limit]
	}

	// Enrich usernames in one read-only query.
	if len(items) > 0 && db.Migrator().HasTable(&User{}) {
		idSet := make(map[int]struct{}, len(items))
		userIDs := make([]int, 0, len(items))
		for _, item := range items {
			if item.UserId > 0 {
				if _, seen := idSet[item.UserId]; !seen {
					idSet[item.UserId] = struct{}{}
					userIDs = append(userIDs, item.UserId)
				}
			}
		}
		if len(userIDs) > 0 {
			var users []struct {
				Id       int    `gorm:"column:id"`
				Username string `gorm:"column:username"`
			}
			if err := db.Unscoped().Table("users").Select("id, username").Where("id IN ?", userIDs).Find(&users).Error; err == nil {
				names := make(map[int]string, len(users))
				for _, u := range users {
					names[u.Id] = u.Username
				}
				for i := range items {
					items[i].Username = names[items[i].UserId]
				}
			}
		}
	}

	summary := BillingReconciliationSummary{TotalItems: len(items)}
	for _, item := range items {
		switch item.Kind {
		case ReconciliationKindSettlementFailed, ReconciliationKindPartialSettlementFailed:
			summary.SettlementExceptionCount++
			if item.ActualQuota != nil && item.DiffQuota != nil {
				summary.KnownPreConsumedQuota += item.PreConsumedQuota
				summary.KnownActualQuota += *item.ActualQuota
				summary.KnownNetDiffQuota += *item.DiffQuota
			} else {
				summary.UnknownActualCount++
				summary.UnknownPreConsumedQuota += item.PreConsumedQuota
			}
		case ReconciliationKindAccountingProjectionFailed:
			summary.ProjectionFailureCount++
		case ReconciliationKindUnsettledPreConsume:
			summary.UnsettledPreConsumeCount++
			if item.ActualQuota != nil && item.DiffQuota != nil {
				summary.KnownPreConsumedQuota += item.PreConsumedQuota
				summary.KnownActualQuota += *item.ActualQuota
				summary.KnownNetDiffQuota += *item.DiffQuota
			} else {
				summary.UnknownActualCount++
				summary.UnknownPreConsumedQuota += item.PreConsumedQuota
			}
		}
	}
	summary.KnownPreConsumedUSD = toUSD(summary.KnownPreConsumedQuota)
	summary.KnownActualUSD = toUSD(summary.KnownActualQuota)
	summary.KnownNetDiffUSD = toUSD(summary.KnownNetDiffQuota)
	summary.UnknownPreConsumedUSD = toUSD(summary.UnknownPreConsumedQuota)

	return &BillingReconciliationReport{
		ReadOnly:       true,
		GeneratedAt:    time.Now().Unix(),
		StartTimestamp: params.StartTimestamp,
		EndTimestamp:   params.EndTimestamp,
		Kind:           kind,
		QuotaPerUnit:   quotaPerUnit,
		Truncated:      truncated,
		Summary:        summary,
		Items:          items,
	}, nil
}
