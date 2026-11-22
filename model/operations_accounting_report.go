package model

import (
	"github.com/QuantumNous/new-api/common"
	"sort"
	"strconv"
)

// Batch-load immutable snapshots, never reinterpret old calls using today's
// channel settings. Missing provenance remains unknown rather than zero.
func enrichOperationsAccounting(logs []operationsReportLog, params OperationsReportParams) error {
	if DB == nil {
		return nil
	}
	var versions []ChannelCostVersion
	if err := DB.Where("effective_at <= ?", params.EndTimestamp).Order("effective_at ASC, id ASC").Find(&versions).Error; err != nil {
		return err
	}
	byChannel := map[int][]ChannelCostVersion{}
	for _, v := range versions {
		byChannel[v.ChannelId] = append(byChannel[v.ChannelId], v)
	}
	requestIds := make([]string, 0, len(logs))
	for _, log := range logs {
		if log.RequestId != "" {
			requestIds = append(requestIds, log.RequestId)
		}
	}
	income := map[string]OperationsEntry{}
	for offset := 0; offset < len(requestIds); offset += 500 {
		end := min(offset+500, len(requestIds))
		var entries []OperationsEntry
		if err := DB.Where("request_id IN ? AND kind IN ?", requestIds[offset:end], []string{"wallet_usage", "subscription_usage", "team_usage"}).Find(&entries).Error; err != nil {
			return err
		}
		for _, e := range entries {
			income[e.RequestId] = e
		}
	}
	for i := range logs {
		log := &logs[i]
		channelVersions := byChannel[log.ChannelId]
		other, err := common.StrToMap(log.Other)
		if err != nil || other == nil {
			other = map[string]interface{}{}
		}
		// Income is trusted only from the primary accounting journal.
		delete(other, "operations_income_usd")
		delete(other, "operations_income_known")
		log.IncomeReason = "income_record_missing"
		if e, ok := income[log.RequestId]; ok && log.Type == LogTypeConsume && (e.RequestUserId == log.UserId || e.RequestUserId == 0 && e.UserId == log.UserId) && e.Quota == int64(log.Quota) {
			if n, err := strconv.ParseFloat(e.ValueUSD, 64); err == nil {
				other["operations_income_usd"] = n
				other["operations_income_known"] = e.UnknownQuota == "0"
				if e.UnknownQuota != "0" {
					log.IncomeReason = "funding_source_unknown"
				}
			}
		}
		startedAt, validStart := other["operations_request_started_at"].(float64)
		validStart = validStart && startedAt > 0 && startedAt <= float64(log.CreatedAt)
		log.AfterCostEffective = validStart && len(channelVersions) > 0 && startedAt >= float64(channelVersions[0].EffectiveAt)
		log.CostReason = "billing_metadata_incomplete"
		if log.Type == LogTypeError {
			log.CostReason = "failed_cost_unknown"
		}
		if _, known := operationReportCostValue(log.Other); !known && log.Type == LogTypeConsume {
			// Old logs have no start-time snapshot. Selecting by completion time
			// could apply a rate created while a long request was in flight.
			if !validStart {
				log.CostReason = "request_start_missing"
				log.Other = common.MapToJsonStr(other)
				continue
			}
			idx := sort.Search(len(channelVersions), func(j int) bool { return channelVersions[j].EffectiveAt > int64(startedAt) }) - 1
			if len(channelVersions) == 0 {
				log.CostReason = "cost_version_missing"
			} else if idx < 0 {
				log.CostReason = "before_cost_effective"
			}
			if idx >= 0 {
				v := channelVersions[idx]
				if value, ok := OperationsEstimateCost(log.Quota, other, v.Multiplier); ok {
					other["upstream_cost_usd"] = value
					other["operations_cost_version"] = v.Id
					other["operations_cost_estimated"] = true
				} else if admin, ok := other["admin_info"].(map[string]interface{}); ok {
					if attempts, ok := admin["use_channel"].([]interface{}); ok && len(attempts) > 1 {
						log.CostReason = "retry_cost_unknown"
					}
				}
			}
		}
		log.Other = common.MapToJsonStr(other)
	}
	return nil
}

// A lost/disabled consumption log must not turn a partial report into verified
// profit. Unmatched journal entries cannot safely be assigned to a dimension.
func checkOperationsReportCompleteness(report *OperationsReport, logs []operationsReportLog, params OperationsReportParams) error {
	if DB == nil {
		return nil
	}
	seen := make(map[string]bool, len(logs))
	for _, log := range logs {
		if log.Type == LogTypeConsume {
			seen[log.RequestId] = true
		}
	}
	query := DB.Model(&OperationsEntry{}).Where("created_at >= ? AND created_at <= ? AND kind IN ?", params.StartTimestamp, params.EndTimestamp, []string{"wallet_usage", "subscription_usage", "team_usage"})
	// Unmatched entries and pending events cannot safely be attributed to a
	// filtered dimension. Keep the global guard even for filtered reports:
	// matched partial profit remains available, but full profit is unverified.
	var entries []OperationsEntry
	if err := query.Select("request_id").Limit(operationsReportMaxLogRows + 1).Find(&entries).Error; err != nil {
		return err
	}
	missing := report.Truncated || len(entries) > operationsReportMaxLogRows
	// Pending projection is never interpreted as zero cost or verified profit,
	// including when the independent consumption log was not written.
	var pending int64
	if err := DB.Model(&OperationsEvent{}).Where("created_at >= ? AND created_at <= ? AND done = ?", params.StartTimestamp, params.EndTimestamp, false).Count(&pending).Error; err != nil {
		return err
	}
	missing = missing || pending > 0
	for _, entry := range entries {
		if !seen[entry.RequestId] {
			missing = true
			break
		}
	}
	if missing {
		report.Warnings = append(report.Warnings, "Accounting journal and consumption logs are incomplete; profit is unverified.")
		report.Summary.RevenueStatus = "partial"
		report.Summary.ProfitUSD = nil
		for i := range report.Rows {
			report.Rows[i].RevenueStatus = "partial"
			report.Rows[i].ProfitUSD = nil
		}
	}
	return nil
}
