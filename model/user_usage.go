package model

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	userUsageSummaryCachePrefix     = "user:usage_summary:v1"
	userUsageSummaryCacheTTL        = 60 * time.Second
	userUsageSummaryHistoryCacheTTL = 5 * time.Minute
	userUsageSummaryHistoryBuffer   = 300
)

type UserUsageSummary struct {
	TokenUsed     int64 `json:"token_used"`
	RequestCount  int64 `json:"request_count"`
	ConsumedQuota int64 `json:"consumed_quota"`
}

type userUsageAggregate struct {
	UserID        int   `gorm:"column:user_id"`
	TokenUsed     int64 `gorm:"column:token_used"`
	RequestCount  int64 `gorm:"column:request_count"`
	ConsumedQuota int64 `gorm:"column:consumed_quota"`
}

func GetUserUsageAccounts(ctx context.Context, ids []int) ([]*User, error) {
	var users []*User
	err := DB.WithContext(ctx).Unscoped().Select("id", "request_count", "used_quota").Where("id IN ?", ids).Find(&users).Error
	return users, err
}

func userUsageSummaryCacheKey(userID int, startTimestamp int64, endTimestamp int64) string {
	return fmt.Sprintf("%s:%d:%d:%d", userUsageSummaryCachePrefix, userID, startTimestamp, endTimestamp)
}

func userUsageSummaryTTL(endTimestamp int64) time.Duration {
	if endTimestamp > 0 && endTimestamp <= time.Now().Unix()-userUsageSummaryHistoryBuffer {
		return userUsageSummaryHistoryCacheTTL
	}
	return userUsageSummaryCacheTTL
}

func GetUsersUsageSummaries(ctx context.Context, userIDs []int, startTimestamp int64, endTimestamp int64) (map[int]UserUsageSummary, error) {
	result := make(map[int]UserUsageSummary, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	uniqueIDs := make([]int, 0, len(userIDs))
	seen := make(map[int]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	if len(uniqueIDs) == 0 {
		return result, nil
	}

	missingIDs := uniqueIDs
	if common.RedisEnabled && common.RDB != nil {
		keys := make([]string, len(uniqueIDs))
		for i, id := range uniqueIDs {
			keys[i] = userUsageSummaryCacheKey(id, startTimestamp, endTimestamp)
		}
		if vals, err := common.RDB.MGet(ctx, keys...).Result(); err == nil && len(vals) == len(uniqueIDs) {
			missingIDs = make([]int, 0, len(uniqueIDs))
			for i, raw := range vals {
				if raw == nil {
					missingIDs = append(missingIDs, uniqueIDs[i])
					continue
				}
				var payload string
				switch v := raw.(type) {
				case string:
					payload = v
				case []byte:
					payload = string(v)
				default:
					missingIDs = append(missingIDs, uniqueIDs[i])
					continue
				}
				var summary UserUsageSummary
				if err := common.UnmarshalJsonStr(payload, &summary); err != nil {
					missingIDs = append(missingIDs, uniqueIDs[i])
					continue
				}
				result[uniqueIDs[i]] = summary
			}
		}
	}

	if len(missingIDs) == 0 {
		return result, nil
	}

	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select("user_id, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS token_used, COUNT(*) AS request_count, COALESCE(SUM(quota), 0) AS consumed_quota").
		Where("user_id IN ? AND type = ?", missingIDs, LogTypeConsume)
	if startTimestamp > 0 {
		query = query.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp > 0 {
		query = query.Where("created_at <= ?", endTimestamp)
	}
	var rows []userUsageAggregate
	if err := query.Group("user_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	fetched := make(map[int]UserUsageSummary, len(missingIDs))
	for _, row := range rows {
		summary := UserUsageSummary{
			TokenUsed:     row.TokenUsed,
			RequestCount:  row.RequestCount,
			ConsumedQuota: row.ConsumedQuota,
		}
		fetched[row.UserID] = summary
		result[row.UserID] = summary
	}

	if common.RedisEnabled && common.RDB != nil {
		ttl := userUsageSummaryTTL(endTimestamp)
		pipe := common.RDB.Pipeline()
		for _, id := range missingIDs {
			encoded, err := common.Marshal(fetched[id])
			if err != nil {
				continue
			}
			pipe.Set(ctx, userUsageSummaryCacheKey(id, startTimestamp, endTimestamp), string(encoded), ttl)
		}
		_, _ = pipe.Exec(ctx)
	}

	return result, nil
}

func GetPendingUsersUsage(userIDs []int) map[int]UserUsageSummary {
	result := make(map[int]UserUsageSummary, len(userIDs))
	if len(userIDs) == 0 || !common.BatchUpdateEnabled {
		return result
	}
	wanted := make(map[int]struct{}, len(userIDs))
	for _, userID := range userIDs {
		wanted[userID] = struct{}{}
	}
	batchUpdateLocks[BatchUpdateTypeUsedQuota].Lock()
	for userID, value := range batchUpdateStores[BatchUpdateTypeUsedQuota] {
		if _, ok := wanted[userID]; ok {
			summary := result[userID]
			summary.ConsumedQuota = int64(value)
			result[userID] = summary
		}
	}
	batchUpdateLocks[BatchUpdateTypeUsedQuota].Unlock()

	batchUpdateLocks[BatchUpdateTypeRequestCount].Lock()
	for userID, value := range batchUpdateStores[BatchUpdateTypeRequestCount] {
		if _, ok := wanted[userID]; ok {
			summary := result[userID]
			summary.RequestCount = int64(value)
			result[userID] = summary
		}
	}
	batchUpdateLocks[BatchUpdateTypeRequestCount].Unlock()
	return result
}
