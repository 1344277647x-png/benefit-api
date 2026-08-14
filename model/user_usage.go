package model

import "github.com/QuantumNous/new-api/common"

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

func GetUsersUsageSummaries(userIDs []int, startTimestamp int64, endTimestamp int64) (map[int]UserUsageSummary, error) {
	result := make(map[int]UserUsageSummary, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	query := LOG_DB.Model(&Log{}).
		Select("user_id, COALESCE(SUM(prompt_tokens), 0) + COALESCE(SUM(completion_tokens), 0) AS token_used, COUNT(*) AS request_count, COALESCE(SUM(quota), 0) AS consumed_quota").
		Where("user_id IN ? AND type = ?", userIDs, LogTypeConsume)
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
	for _, row := range rows {
		result[row.UserID] = UserUsageSummary{
			TokenUsed:     row.TokenUsed,
			RequestCount:  row.RequestCount,
			ConsumedQuota: row.ConsumedQuota,
		}
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
