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
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	operationsRollupHourSeconds        int64 = 3600
	operationsRollupCloseBufferSeconds int64 = 120
	operationsRollupChunkSize                = 5000
	operationsRollupMaxRowSamples            = 5
)

// OperationsRollupBucket records a sealed closed UTC hour whose logs and
// accounting journal entries have been pre-aggregated into operations_hourly_rollups.
type OperationsRollupBucket struct {
	HourBucket          int64  `gorm:"primaryKey;type:bigint"`
	LogCount            int64  `gorm:"type:bigint;not null"`
	JournalEntryCount   int64  `gorm:"type:bigint;not null"`
	CompletenessMissing bool   `gorm:"not null"`
	RolledUpAt          int64  `gorm:"type:bigint;not null"`
	BuildKey            string `gorm:"size:64;not null;default:''"`
	RetryAt             int64  `gorm:"type:bigint;not null;default:0;index"`
}

// Stored in the log database so a delayed log and its invalidation commit together.
type OperationsRollupDirtyHour struct {
	HourBucket int64  `gorm:"primaryKey;type:bigint;autoIncrement:false"`
	ChangeKey  string `gorm:"size:64;not null"`
	RetryAt    int64  `gorm:"type:bigint;not null;default:0;index"`
}

func lateOperationsLogHour(log *Log) int64 {
	if log.Type != LogTypeConsume && log.Type != LogTypeRefund && log.Type != LogTypeError {
		return 0
	}
	hour := floorOperationsHour(log.CreatedAt)
	if hour <= 0 || hour+operationsRollupHourSeconds-1 >= common.GetTimestamp()-operationsRollupCloseBufferSeconds {
		return 0
	}
	return hour
}

func markLateOperationsHour(tx *gorm.DB, hour int64) error {
	dirty := OperationsRollupDirtyHour{HourBucket: hour, ChangeKey: common.NewRequestId()}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hour_bucket"}},
		DoUpdates: clause.AssignmentColumns([]string{"change_key", "retry_at"}),
	}).Create(&dirty).Error
}

// OperationsHourlyRollup stores additive hourly metrics per report dimension so
// long-range operations reports avoid the 200,000 raw-log truncation cap while
// staying compatible across SQLite, MySQL, and PostgreSQL.
type OperationsHourlyRollup struct {
	Id                     int64   `gorm:"primaryKey"`
	HourBucket             int64   `gorm:"type:bigint;not null;uniqueIndex:idx_ops_rollup_dim,priority:1;index"`
	DayKey                 string  `gorm:"type:varchar(16);not null;index"`
	ModelName              string  `gorm:"type:varchar(191);not null;uniqueIndex:idx_ops_rollup_dim,priority:2"`
	ChannelId              int     `gorm:"not null;uniqueIndex:idx_ops_rollup_dim,priority:3"`
	UserId                 int     `gorm:"not null;uniqueIndex:idx_ops_rollup_dim,priority:4"`
	Username               string  `gorm:"type:varchar(128);not null;index"`
	GroupName              string  `gorm:"column:group_name;type:varchar(64);not null;uniqueIndex:idx_ops_rollup_dim,priority:5"`
	AfterCostEffective     bool    `gorm:"not null;uniqueIndex:idx_ops_rollup_dim,priority:6"`
	RequestCount           int64   `gorm:"type:bigint;not null"`
	SuccessCount           int64   `gorm:"type:bigint;not null"`
	ErrorCount             int64   `gorm:"type:bigint;not null"`
	TokenCount             int64   `gorm:"type:bigint;not null"`
	RevenueQuota           int64   `gorm:"type:bigint;not null"`
	RefundQuota            int64   `gorm:"type:bigint;not null"`
	RevenueUSD             float64 `gorm:"not null"`
	RefundUSD              float64 `gorm:"not null"`
	CostUSD                float64 `gorm:"not null"`
	KnownCostRequests      int64   `gorm:"type:bigint;not null"`
	UnknownCostRequests    int64   `gorm:"type:bigint;not null"`
	UnknownRevenueRequests int64   `gorm:"type:bigint;not null"`
	MatchedRequests        int64   `gorm:"type:bigint;not null"`
	PendingRequests        int64   `gorm:"type:bigint;not null"`
	MatchedQuota           int64   `gorm:"type:bigint;not null"`
	HasPartialProfit       bool    `gorm:"not null"`
	PartialProfitUSD       float64 `gorm:"not null"`
	ErrorsWithoutQuota     int64   `gorm:"type:bigint;not null"`
	ReasonsJSON            string  `gorm:"type:text"`
	PendingSamplesJSON     string  `gorm:"type:text"`
}

func floorOperationsHour(ts int64) int64 {
	if ts <= 0 {
		return 0
	}
	return ts - (ts % operationsRollupHourSeconds)
}

func truncateRollupDim(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

type operationsRollupDimKey struct {
	modelName          string
	channelID          int
	userID             int
	groupName          string
	afterCostEffective bool
}

type operationsRollupBuilder struct {
	row            OperationsHourlyRollup
	reasons        map[string]int64
	pendingSamples []OperationsPendingRequest
}

func operationsRollupTablesReady() bool {
	if DB == nil || LOG_DB == nil {
		return false
	}
	return !common.UsingLogDatabase(common.DatabaseTypeClickHouse) &&
		DB.Migrator().HasTable(&OperationsRollupBucket{}) && DB.Migrator().HasTable(&OperationsHourlyRollup{})
}

// RollupOperationsHour materializes one closed UTC hour [hourBucket, hourBucket+3599]
// into operations_hourly_rollups in bounded chunks, with no 200,000-row cap.
func RollupOperationsHour(ctx context.Context, hourBucket int64) error {
	_, err := buildOperationsHour(ctx, hourBucket)
	return err
}

func buildOperationsHour(ctx context.Context, hourBucket int64) (bool, error) {
	if DB == nil || LOG_DB == nil {
		return false, errors.New("databases are not initialized")
	}
	hourBucket = floorOperationsHour(hourBucket)
	if hourBucket <= 0 {
		return false, errors.New("invalid hour bucket")
	}
	hourEnd := hourBucket + operationsRollupHourSeconds - 1
	if hourEnd >= common.GetTimestamp()-operationsRollupCloseBufferSeconds {
		return false, errors.New("hour bucket is not yet closed")
	}
	if !operationsRollupTablesReady() {
		return false, nil
	}
	var state OperationsState
	if err := DB.WithContext(ctx).First(&state, 1).Error; err != nil {
		return false, err
	}
	var previous OperationsRollupBucket
	if err := DB.WithContext(ctx).Where("hour_bucket = ?", hourBucket).Find(&previous).Error; err != nil {
		return false, err
	}

	// Never seal an hour while its accounting projections are still pending.
	if DB.WithContext(ctx).Migrator().HasTable(&OperationsEvent{}) {
		var pending int64
		if err := DB.WithContext(ctx).Model(&OperationsEvent{}).
			Where("created_at >= ? AND created_at <= ? AND done = ?", hourBucket, hourEnd, false).
			Count(&pending).Error; err != nil {
			return false, err
		}
		if pending > 0 {
			return false, nil
		}
	}

	dayKey := OperationsBusinessDay(hourBucket)
	builders := make(map[operationsRollupDimKey]*operationsRollupBuilder)
	seenConsumeRequests := make(map[string]struct{})
	var totalLogs int64
	var lastID int

	enrichParams := OperationsReportParams{
		StartTimestamp: hourBucket,
		EndTimestamp:   hourEnd,
	}

	for {
		var chunk []operationsReportLog
		err := LOG_DB.WithContext(ctx).Table("logs").
			Select("id, request_id, user_id, username, created_at, type, model_name, quota, prompt_tokens, completion_tokens, channel_id, "+logGroupCol+" AS group_value, other").
			Where("created_at >= ? AND created_at <= ?", hourBucket, hourEnd).
			Where("type IN ?", []int{LogTypeConsume, LogTypeRefund, LogTypeError}).
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(operationsRollupChunkSize).
			Find(&chunk).Error
		if err != nil {
			return false, fmt.Errorf("rollup query logs for hour %d: %w", hourBucket, err)
		}
		if len(chunk) == 0 {
			break
		}
		lastID = chunk[len(chunk)-1].ID
		totalLogs += int64(len(chunk))

		if err := enrichOperationsAccounting(chunk, enrichParams); err != nil {
			return false, fmt.Errorf("rollup enrich logs for hour %d: %w", hourBucket, err)
		}

		for _, log := range chunk {
			if isOperationsProjectionAlert(log) {
				continue
			}
			if log.Type == LogTypeConsume && log.RequestId != "" {
				seenConsumeRequests[log.RequestId] = struct{}{}
			}
			dim := operationsRollupDimKey{
				modelName:          truncateRollupDim(log.ModelName, 191),
				channelID:          log.ChannelId,
				userID:             log.UserId,
				groupName:          truncateRollupDim(log.Group, 64),
				afterCostEffective: log.AfterCostEffective,
			}
			b, ok := builders[dim]
			if !ok {
				b = &operationsRollupBuilder{
					row: OperationsHourlyRollup{
						HourBucket:         hourBucket,
						DayKey:             dayKey,
						ModelName:          dim.modelName,
						ChannelId:          dim.channelID,
						UserId:             dim.userID,
						Username:           truncateRollupDim(log.Username, 128),
						GroupName:          dim.groupName,
						AfterCostEffective: dim.afterCostEffective,
					},
					reasons: make(map[string]int64),
				}
				builders[dim] = b
			} else if b.row.Username == "" && log.Username != "" {
				b.row.Username = truncateRollupDim(log.Username, 128)
			}

			switch log.Type {
			case LogTypeConsume:
				b.row.RequestCount++
				b.row.SuccessCount++
				b.row.TokenCount += int64(log.PromptTokens) + int64(log.CompletionTokens)
				if log.Quota > 0 {
					b.row.RevenueQuota += int64(log.Quota)
				}
				other, _ := common.StrToMap(log.Other)
				if income, ok := other["operations_income_usd"].(float64); ok && income >= 0 && !math.IsNaN(income) && !math.IsInf(income, 0) {
					b.row.RevenueUSD += income
					if other["operations_income_known"] != true {
						b.row.UnknownRevenueRequests++
					}
				} else {
					b.row.UnknownRevenueRequests++
				}
			case LogTypeRefund:
				if log.Quota > 0 {
					b.row.RefundQuota += int64(log.Quota)
				}
			case LogTypeError:
				b.row.RequestCount++
				b.row.ErrorCount++
			}

			if log.Type == LogTypeConsume || log.Type == LogTypeError {
				if cost, known := operationReportCostValue(log.Other); known {
					b.row.CostUSD += cost
					b.row.KnownCostRequests++
				} else {
					b.row.UnknownCostRequests++
				}
				var cov OperationsCoverage
				addOperationsCoverage(&cov, log)
				b.row.MatchedRequests += cov.MatchedRequests
				b.row.PendingRequests += cov.PendingRequests
				b.row.MatchedQuota += cov.MatchedQuota
				b.row.ErrorsWithoutQuota += cov.ErrorsWithoutQuota
				if cov.PartialProfitUSD != nil {
					b.row.HasPartialProfit = true
					b.row.PartialProfitUSD += *cov.PartialProfitUSD
				}
				for reason, count := range cov.Reasons {
					b.reasons[reason] += count
				}
				for _, sample := range cov.PendingSamples {
					if len(b.pendingSamples) < operationsRollupMaxRowSamples {
						b.pendingSamples = append(b.pendingSamples, sample)
					}
				}
			}
		}

		if len(chunk) < operationsRollupChunkSize {
			break
		}
	}

	// Verify completeness against OperationsEntry for this closed hour in bounded batches.
	var journalCount int64
	completenessMissing := false
	if DB.WithContext(ctx).Migrator().HasTable(&OperationsEntry{}) {
		lastKey := ""
		for {
			var entries []OperationsEntry
			q := DB.WithContext(ctx).Model(&OperationsEntry{}).
				Select("event_key, request_id").
				Where("created_at >= ? AND created_at <= ? AND kind IN ?", hourBucket, hourEnd, []string{"wallet_usage", "subscription_usage", "team_usage"})
			if lastKey != "" {
				q = q.Where("event_key > ?", lastKey)
			}
			if err := q.Order("event_key ASC").Limit(operationsRollupChunkSize).Find(&entries).Error; err != nil {
				return false, fmt.Errorf("rollup query journal entries for hour %d: %w", hourBucket, err)
			}
			if len(entries) == 0 {
				break
			}
			lastKey = entries[len(entries)-1].EventKey
			journalCount += int64(len(entries))
			if !completenessMissing {
				for _, e := range entries {
					if _, ok := seenConsumeRequests[e.RequestId]; !ok {
						completenessMissing = true
						break
					}
				}
			}
			if len(entries) < operationsRollupChunkSize {
				break
			}
		}
	}

	rows := make([]OperationsHourlyRollup, 0, len(builders))
	for _, b := range builders {
		if len(b.reasons) > 0 {
			if encoded, err := common.Marshal(b.reasons); err == nil {
				b.row.ReasonsJSON = string(encoded)
			}
		}
		if len(b.pendingSamples) > 0 {
			if encoded, err := common.Marshal(b.pendingSamples); err == nil {
				b.row.PendingSamplesJSON = string(encoded)
			}
		}
		rows = append(rows, b.row)
	}

	published := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current OperationsState
		if err := lockForUpdate(tx).First(&current, 1).Error; err != nil {
			return err
		}
		var currentBucket OperationsRollupBucket
		if err := tx.Where("hour_bucket = ?", hourBucket).Find(&currentBucket).Error; err != nil {
			return err
		}
		if current.RollupRevision != state.RollupRevision || currentBucket.BuildKey != previous.BuildKey {
			return nil
		}
		if err := tx.Where("hour_bucket = ?", hourBucket).Delete(&OperationsHourlyRollup{}).Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.CreateInBatches(&rows, 200).Error; err != nil {
				return err
			}
		}
		bucket := OperationsRollupBucket{
			HourBucket:          hourBucket,
			LogCount:            totalLogs,
			JournalEntryCount:   journalCount,
			CompletenessMissing: completenessMissing,
			RolledUpAt:          common.GetTimestamp(),
			BuildKey:            common.NewRequestId(),
		}
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "hour_bucket"}},
			DoUpdates: clause.AssignmentColumns([]string{"log_count", "journal_entry_count", "completeness_missing", "rolled_up_at", "build_key", "retry_at"}),
		}).Create(&bucket).Error
		published = err == nil
		return err
	})
	return published && err == nil, err
}

// ProcessOperationsHourlyRollups finds unsealed closed hours with logs and
// materializes up to maxHours buckets per invocation.
func ProcessOperationsHourlyRollups(ctx context.Context, maxHours int) error {
	if !operationsRollupTablesReady() {
		return nil
	}
	if maxHours <= 0 {
		maxHours = 4
	}
	var state OperationsState
	if err := DB.WithContext(ctx).First(&state, 1).Error; err != nil {
		return err
	}
	startedAt := state.StartedAt
	latestClosableEnd := floorOperationsHour(common.GetTimestamp()-operationsRollupCloseBufferSeconds) - 1
	if latestClosableEnd < startedAt {
		return nil
	}

	now := common.GetTimestamp()
	logDB := LOG_DB.WithContext(ctx)
	var late []OperationsRollupDirtyHour
	if err := logDB.Where("retry_at <= ?", now).Order("retry_at, hour_bucket").Limit(maxHours).Find(&late).Error; err != nil {
		return err
	}
	for _, hour := range late {
		published := hour.HourBucket+operationsRollupHourSeconds <= startedAt
		var err error
		if !published {
			published, err = buildOperationsHour(ctx, hour.HourBucket)
		}
		q := logDB.Where("hour_bucket = ? AND change_key = ?", hour.HourBucket, hour.ChangeKey)
		if published {
			if err := q.Delete(&OperationsRollupDirtyHour{}).Error; err != nil {
				return err
			}
		} else {
			if retryErr := q.Model(&OperationsRollupDirtyHour{}).Update("retry_at", now+60).Error; retryErr != nil {
				return retryErr
			}
		}
		if err != nil && ctx.Err() != nil {
			return err
		}
	}
	var deferred []OperationsRollupBucket
	if err := DB.WithContext(ctx).Where("rolled_up_at = 0 AND retry_at <= ?", now).Order("retry_at, hour_bucket").Limit(maxHours).Find(&deferred).Error; err != nil {
		return err
	}
	for _, hour := range deferred {
		published, err := buildOperationsHour(ctx, hour.HourBucket)
		if !published {
			if updateErr := DB.WithContext(ctx).Model(&OperationsRollupBucket{}).Where("hour_bucket = ? AND rolled_up_at = 0", hour.HourBucket).Update("retry_at", now+60).Error; updateErr != nil {
				return updateErr
			}
		}
		if err != nil && ctx.Err() != nil {
			return err
		}
	}

	cursor := max(startedAt, state.RollupCursor, 1)
	for attempt := 0; attempt < maxHours && cursor <= latestClosableEnd; attempt++ {
		var nextCreatedAt int64
		err := LOG_DB.WithContext(ctx).Table("logs").
			Select("COALESCE(MIN(created_at), 0)").
			Where("created_at >= ? AND created_at <= ?", cursor, latestClosableEnd).
			Where("type IN ?", []int{LogTypeConsume, LogTypeRefund, LogTypeError}).
			Scan(&nextCreatedAt).Error
		if err != nil {
			return err
		}
		if nextCreatedAt <= 0 {
			cursor = latestClosableEnd + 1
			break
		}
		hourBucket := floorOperationsHour(nextCreatedAt)
		cursor = hourBucket + operationsRollupHourSeconds

		var existing OperationsRollupBucket
		if err := DB.WithContext(ctx).Where("hour_bucket = ?", hourBucket).Find(&existing).Error; err != nil {
			return err
		}
		if existing.BuildKey != "" || existing.RetryAt > 0 {
			continue
		}
		published, err := buildOperationsHour(ctx, hourBucket)
		if err != nil && ctx.Err() != nil {
			return err
		}
		if !published {
			// An old-format bucket has no trustworthy build key. Turn it into retry work.
			waiting := OperationsRollupBucket{HourBucket: hourBucket, RetryAt: now + 60}
			if err := DB.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&waiting).Error; err != nil {
				return err
			}
			if err := DB.WithContext(ctx).Model(&OperationsRollupBucket{}).Where("hour_bucket = ? AND build_key = ?", hourBucket, "").Updates(map[string]any{"rolled_up_at": 0, "retry_at": now + 60}).Error; err != nil {
				return err
			}
		}
	}
	return DB.WithContext(ctx).Model(&OperationsState{}).
		Where("id = ? AND rollup_revision = ? AND rollup_cursor = ?", 1, state.RollupRevision, state.RollupCursor).
		Update("rollup_cursor", cursor).Error
}

func invalidateOperationsRollupsFromTx(tx *gorm.DB, effectiveAt int64) error {
	if tx == nil || !tx.Migrator().HasTable(&OperationsRollupBucket{}) {
		return nil
	}
	fromHour := floorOperationsHour(effectiveAt)
	if err := tx.Model(&OperationsState{}).Where("id = ?", 1).Updates(map[string]any{
		"rollup_revision": gorm.Expr("rollup_revision + 1"),
		"rollup_cursor":   gorm.Expr("CASE WHEN rollup_cursor > ? THEN ? ELSE rollup_cursor END", fromHour, fromHour),
	}).Error; err != nil {
		return err
	}
	if err := tx.Where("hour_bucket >= ?", fromHour).Delete(&OperationsHourlyRollup{}).Error; err != nil {
		return err
	}
	return tx.Where("hour_bucket >= ?", fromHour).Delete(&OperationsRollupBucket{}).Error
}

type operationsTimeInterval struct {
	start int64
	end   int64
}

func computeUncoveredIntervals(start, end int64, sealedHours []int64) []operationsTimeInterval {
	if start > end {
		return nil
	}
	if len(sealedHours) == 0 {
		return []operationsTimeInterval{{start: start, end: end}}
	}
	sorted := make([]int64, len(sealedHours))
	copy(sorted, sealedHours)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	intervals := make([]operationsTimeInterval, 0, len(sorted)+1)
	cursor := start
	for _, h := range sorted {
		hEnd := h + operationsRollupHourSeconds - 1
		if hEnd < cursor || h > end {
			continue
		}
		if h > cursor {
			intervals = append(intervals, operationsTimeInterval{start: cursor, end: h - 1})
		}
		if hEnd+1 > cursor {
			cursor = hEnd + 1
		}
	}
	if cursor <= end {
		intervals = append(intervals, operationsTimeInterval{start: cursor, end: end})
	}
	return intervals
}

func rollupRowKey(row OperationsHourlyRollup, groupBy string) (string, string) {
	switch groupBy {
	case "model":
		label := row.ModelName
		if label == "" {
			label = "(unknown)"
		}
		return "model:" + label, label
	case "channel":
		label := row.GroupName
		if row.ChannelId > 0 {
			label = strconv.Itoa(row.ChannelId)
		} else if label == "" {
			label = "(unknown)"
		}
		return "channel:" + label, label
	case "user":
		label := row.Username
		if label == "" {
			label = strconv.Itoa(row.UserId)
		}
		return "user:" + label, label
	case "group":
		label := row.GroupName
		if label == "" {
			label = "(default)"
		}
		return "group:" + label, label
	default:
		label := row.DayKey
		if label == "" {
			label = OperationsBusinessDay(row.HourBucket)
		}
		return "day:" + label, label
	}
}

func mergeRollupIntoAccumulator(target *operationsReportAccumulator, row OperationsHourlyRollup) {
	target.RequestCount += row.RequestCount
	target.SuccessCount += row.SuccessCount
	target.ErrorCount += row.ErrorCount
	target.TokenCount += row.TokenCount
	target.RevenueQuota += row.RevenueQuota
	target.RefundQuota += row.RefundQuota
	target.RevenueUSD += row.RevenueUSD
	target.RefundUSD += row.RefundUSD
	target.CostUSD += row.CostUSD
	target.knownCostRequests += row.KnownCostRequests
	target.UnknownCostRequests += row.UnknownCostRequests
	target.UnknownRevenueRequests += row.UnknownRevenueRequests
	target.MatchedRequests += row.MatchedRequests
	target.PendingRequests += row.PendingRequests
	target.MatchedQuota += row.MatchedQuota
	target.ErrorsWithoutQuota += row.ErrorsWithoutQuota
	if row.HasPartialProfit {
		if target.PartialProfitUSD == nil {
			target.PartialProfitUSD = new(float64)
		}
		*target.PartialProfitUSD += row.PartialProfitUSD
	}
	if strings.TrimSpace(row.ReasonsJSON) != "" {
		var reasons map[string]int64
		if err := common.UnmarshalJsonStr(row.ReasonsJSON, &reasons); err == nil && len(reasons) > 0 {
			if target.Reasons == nil {
				target.Reasons = map[string]int64{}
			}
			for k, v := range reasons {
				target.Reasons[k] += v
			}
		}
	}
	if strings.TrimSpace(row.PendingSamplesJSON) != "" && len(target.PendingSamples) < 20 {
		var samples []OperationsPendingRequest
		if err := common.UnmarshalJsonStr(row.PendingSamplesJSON, &samples); err == nil {
			for _, s := range samples {
				if len(target.PendingSamples) >= 20 {
					break
				}
				target.PendingSamples = append(target.PendingSamples, s)
			}
		}
	}
}

func getOperationsReportWithRollups(params OperationsReportParams, startedAt int64) (*OperationsReport, bool, error) {
	if !operationsRollupTablesReady() {
		return nil, false, nil
	}
	firstFullHour := floorOperationsHour(params.StartTimestamp)
	if firstFullHour < params.StartTimestamp {
		firstFullHour += operationsRollupHourSeconds
	}
	lastFullHour := floorOperationsHour(params.EndTimestamp)
	if lastFullHour+operationsRollupHourSeconds-1 > params.EndTimestamp {
		lastFullHour -= operationsRollupHourSeconds
	}
	if firstFullHour > lastFullHour {
		return nil, false, nil
	}

	var dirtyHours []int64
	if err := LOG_DB.Model(&OperationsRollupDirtyHour{}).
		Where("hour_bucket BETWEEN ? AND ?", firstFullHour, lastFullHour).
		Pluck("hour_bucket", &dirtyHours).Error; err != nil {
		return nil, false, err
	}
	dirty := make(map[int64]bool, len(dirtyHours))
	for _, h := range dirtyHours {
		dirty[h] = true
	}
	var buckets []OperationsRollupBucket
	var allRows []OperationsHourlyRollup
	opts := &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	if DB.Dialector.Name() == "sqlite" {
		opts = nil
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("hour_bucket BETWEEN ? AND ? AND rolled_up_at > 0 AND build_key <> ?", firstFullHour, lastFullHour, "").Order("hour_bucket ASC").Find(&buckets).Error; err != nil {
			return err
		}
		hours := make([]int64, 0, len(buckets))
		for _, b := range buckets {
			if !dirty[b.HourBucket] {
				hours = append(hours, b.HourBucket)
			}
		}
		for offset := 0; offset < len(hours); offset += 500 {
			q := tx.Model(&OperationsHourlyRollup{}).Where("hour_bucket IN ?", hours[offset:min(offset+500, len(hours))])
			if params.ModelName != "" {
				q = q.Where("model_name = ?", params.ModelName)
			}
			if params.Username != "" {
				q = q.Where("username = ?", params.Username)
			}
			if params.Group != "" {
				q = q.Where("group_name = ?", params.Group)
			}
			if params.ChannelID > 0 {
				q = q.Where("channel_id = ?", params.ChannelID)
			}
			var rows []OperationsHourlyRollup
			if err := q.Find(&rows).Error; err != nil {
				return err
			}
			allRows = append(allRows, rows...)
		}
		return nil
	}, opts)
	if err != nil {
		return nil, false, err
	}
	if len(buckets) == 0 {
		return nil, false, nil
	}

	sealedHours := make([]int64, 0, len(buckets))
	completenessMissing := false
	for _, b := range buckets {
		if dirty[b.HourBucket] {
			continue
		}
		sealedHours = append(sealedHours, b.HourBucket)
		if b.CompletenessMissing {
			completenessMissing = true
		}
	}
	if len(sealedHours) == 0 {
		return nil, false, nil
	}

	groupBy := normalizeOperationsReportGroupBy(params.GroupBy)
	quotaPerUnit := common.QuotaPerUnit
	if quotaPerUnit <= 0 {
		quotaPerUnit = 1
	}

	rowsByKey := make(map[string]*operationsReportAccumulator)
	summary := operationsReportAccumulator{}
	channelIDs := make(map[int]struct{})
	var excludedRequests int64

	// 1. Aggregate all sealed full hours from operations_hourly_rollups (bounded only by dimensions, never 200k logs).
	for _, r := range allRows {
		if params.AfterCostEffective && !r.AfterCostEffective {
			excludedRequests += r.RequestCount
			continue
		}
		if r.ChannelId > 0 {
			channelIDs[r.ChannelId] = struct{}{}
		}
		key, label := rollupRowKey(r, groupBy)
		acc, ok := rowsByKey[key]
		if !ok {
			acc = &operationsReportAccumulator{OperationsReportRow: OperationsReportRow{
				Key:   key,
				Label: label,
			}}
			switch groupBy {
			case "model":
				acc.ModelName = r.ModelName
			case "channel":
				acc.ChannelID = r.ChannelId
			case "user":
				acc.UserID = r.UserId
				acc.Username = r.Username
			case "group":
				acc.Group = r.GroupName
			}
			rowsByKey[key] = acc
		}
		mergeRollupIntoAccumulator(acc, r)
		mergeRollupIntoAccumulator(&summary, r)
	}

	// 2. Aggregate remaining uncovered intervals from raw LOG_DB logs (typically only leading/trailing partial hours).
	uncovered := computeUncoveredIntervals(params.StartTimestamp, params.EndTimestamp, sealedHours)
	truncated := false
	remainingLogBudget := operationsReportMaxLogRows

	for _, iv := range uncovered {
		if remainingLogBudget <= 0 {
			truncated = true
			break
		}
		query := LOG_DB.Table("logs").
			Select("id, request_id, user_id, username, created_at, type, model_name, quota, prompt_tokens, completion_tokens, channel_id, "+logGroupCol+" AS group_value, other").
			Where("created_at >= ? AND created_at <= ?", iv.start, iv.end).
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
		if err := query.Order("created_at ASC").Limit(remainingLogBudget + 1).Find(&logs).Error; err != nil {
			return nil, false, fmt.Errorf("query uncovered operation report logs: %w", err)
		}
		if len(logs) > remainingLogBudget {
			truncated = true
			logs = logs[:remainingLogBudget]
		}
		remainingLogBudget -= len(logs)

		ivParams := params
		ivParams.StartTimestamp = iv.start
		ivParams.EndTimestamp = iv.end
		if err := enrichOperationsAccounting(logs, ivParams); err != nil {
			return nil, false, err
		}

		seen := make(map[string]bool, len(logs))
		for _, log := range logs {
			if isOperationsProjectionAlert(log) {
				continue
			}
			if log.Type == LogTypeConsume {
				seen[log.RequestId] = true
			}
			if params.AfterCostEffective && !log.AfterCostEffective {
				if log.Type == LogTypeConsume || log.Type == LogTypeError {
					excludedRequests++
				}
				continue
			}
			if log.ChannelId > 0 {
				channelIDs[log.ChannelId] = struct{}{}
			}
			key, label := operationReportRowKey(log, groupBy)
			acc, ok := rowsByKey[key]
			if !ok {
				acc = &operationsReportAccumulator{OperationsReportRow: OperationsReportRow{
					Key:   key,
					Label: label,
				}}
				switch groupBy {
				case "model":
					acc.ModelName = log.ModelName
				case "channel":
					acc.ChannelID = log.ChannelId
				case "user":
					acc.UserID = log.UserId
					acc.Username = log.Username
				case "group":
					acc.Group = log.Group
				}
				rowsByKey[key] = acc
			}
			for _, target := range []*operationsReportAccumulator{acc, &summary} {
				switch log.Type {
				case LogTypeConsume:
					target.RequestCount++
					target.SuccessCount++
					target.TokenCount += int64(log.PromptTokens) + int64(log.CompletionTokens)
					if log.Quota > 0 {
						target.RevenueQuota += int64(log.Quota)
					}
					other, _ := common.StrToMap(log.Other)
					if income, ok := other["operations_income_usd"].(float64); ok && income >= 0 && !math.IsNaN(income) && !math.IsInf(income, 0) {
						target.RevenueUSD += income
						if other["operations_income_known"] != true {
							target.UnknownRevenueRequests++
						}
					} else {
						target.UnknownRevenueRequests++
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
			addOperationReportCost(acc, log)
			addOperationReportCost(&summary, log)
			addOperationsCoverage(&acc.OperationsCoverage, log)
			addOperationsCoverage(&summary.OperationsCoverage, log)
		}

		// Verify completeness for this uncovered interval.
		if !completenessMissing && DB.Migrator().HasTable(&OperationsEntry{}) {
			var entries []OperationsEntry
			if err := DB.Model(&OperationsEntry{}).
				Select("request_id").
				Where("created_at >= ? AND created_at <= ? AND kind IN ?", iv.start, iv.end, []string{"wallet_usage", "subscription_usage", "team_usage"}).
				Limit(operationsReportMaxLogRows + 1).
				Find(&entries).Error; err != nil {
				return nil, false, err
			}
			if len(entries) > operationsReportMaxLogRows {
				completenessMissing = true
			} else {
				for _, e := range entries {
					if !seen[e.RequestId] {
						completenessMissing = true
						break
					}
				}
			}
		}
	}

	// Global pending projection check across the entire requested window.
	if !completenessMissing && DB.Migrator().HasTable(&OperationsEvent{}) {
		var pending int64
		if err := DB.Model(&OperationsEvent{}).
			Where("created_at >= ? AND created_at <= ? AND done = ?", params.StartTimestamp, params.EndTimestamp, false).
			Count(&pending).Error; err != nil {
			return nil, false, err
		}
		if pending > 0 {
			completenessMissing = true
		}
	}

	rows := make([]OperationsReportRow, 0, len(rowsByKey))
	for _, acc := range rowsByKey {
		rows = append(rows, finalizeOperationsReportRow(acc, quotaPerUnit))
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

	resultSummary := finalizeOperationsReportRow(&summary, quotaPerUnit)
	ids := make([]int, 0, len(channelIDs))
	for id := range channelIDs {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	report := &OperationsReport{
		AfterCostEffective:  params.AfterCostEffective,
		ExcludedRequests:    excludedRequests,
		AccountingStartedAt: startedAt,
		GeneratedAt:         time.Now().Unix(),
		StartTimestamp:      params.StartTimestamp,
		EndTimestamp:        params.EndTimestamp,
		GroupBy:             groupBy,
		Truncated:           truncated,
		PreAggregated:       true,
		RollupHoursUsed:     len(sealedHours),
		QuotaPerUnit:        quotaPerUnit,
		Summary: OperationsReportSummary{
			OperationsCoverage:     resultSummary.OperationsCoverage,
			UnknownRevenueRequests: resultSummary.UnknownRevenueRequests,
			RevenueStatus:          resultSummary.RevenueStatus,
			RequestCount:           resultSummary.RequestCount,
			SuccessCount:           resultSummary.SuccessCount,
			ErrorCount:             resultSummary.ErrorCount,
			ErrorRate:              resultSummary.ErrorRate,
			TokenCount:             resultSummary.TokenCount,
			RevenueQuota:           resultSummary.RevenueQuota,
			RefundQuota:            resultSummary.RefundQuota,
			RevenueUSD:             resultSummary.RevenueUSD,
			RefundUSD:              resultSummary.RefundUSD,
			CostUSD:                resultSummary.CostUSD,
			CostStatus:             resultSummary.CostStatus,
			UnknownCostRequests:    resultSummary.UnknownCostRequests,
			ProfitUSD:              resultSummary.ProfitUSD,
		},
		Rows:       rows,
		ChannelIDs: ids,
		Channels:   []OperationsReportChannel{},
		Health:     []OperationsReportHealth{},
	}

	if truncated || completenessMissing {
		report.Warnings = append(report.Warnings, "Accounting journal and consumption logs are incomplete; profit is unverified.")
		report.Summary.RevenueStatus = "partial"
		report.Summary.ProfitUSD = nil
		for i := range report.Rows {
			report.Rows[i].RevenueStatus = "partial"
			report.Rows[i].ProfitUSD = nil
		}
	}

	if err := enrichOperationsFundingTotals(report, params); err != nil {
		return nil, false, err
	}
	return report, true, nil
}
