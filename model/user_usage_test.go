package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUsersUsageSummariesRedisCacheAndFallback(t *testing.T) {
	truncateTables(t)
	server := useUserCacheMiniRedis(t)

	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           101,
		Type:             LogTypeConsume,
		PromptTokens:     30,
		CompletionTokens: 20,
		Quota:            500,
		CreatedAt:        1000,
	}).Error)

	// 1. Cold read queries LOG_DB and populates Redis for both active and zero-usage users.
	got, err := GetUsersUsageSummaries(t.Context(), []int{101, 102, 101}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, UserUsageSummary{TokenUsed: 50, RequestCount: 1, ConsumedQuota: 500}, got[101])
	assert.Equal(t, UserUsageSummary{}, got[102])

	ttl101 := server.TTL(userUsageSummaryCacheKey(101, 0, 0))
	assert.Greater(t, ttl101, time.Duration(0))
	assert.LessOrEqual(t, ttl101, userUsageSummaryCacheTTL)
	assert.True(t, server.Exists(userUsageSummaryCacheKey(102, 0, 0)))

	// 2. Subsequent write to LOG_DB is served from Redis cache until TTL expires.
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           101,
		Type:             LogTypeConsume,
		PromptTokens:     10,
		CompletionTokens: 5,
		Quota:            100,
		CreatedAt:        2000,
	}).Error)

	cached, err := GetUsersUsageSummaries(t.Context(), []int{101, 102}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, UserUsageSummary{TokenUsed: 50, RequestCount: 1, ConsumedQuota: 500}, cached[101])

	// 3. Partial cache hit: user 103 misses cache and queries DB, while user 101 stays cached.
	require.NoError(t, LOG_DB.Create(&Log{
		UserId:           103,
		Type:             LogTypeConsume,
		PromptTokens:     7,
		CompletionTokens: 3,
		Quota:            75,
		CreatedAt:        1500,
	}).Error)
	partial, err := GetUsersUsageSummaries(t.Context(), []int{101, 103}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, UserUsageSummary{TokenUsed: 50, RequestCount: 1, ConsumedQuota: 500}, partial[101])
	assert.Equal(t, UserUsageSummary{TokenUsed: 10, RequestCount: 1, ConsumedQuota: 75}, partial[103])

	// 4. Corrupt cached value gracefully falls back to LOG_DB.
	require.NoError(t, server.Set(userUsageSummaryCacheKey(101, 0, 0), "{broken-json"))
	recovered, err := GetUsersUsageSummaries(t.Context(), []int{101}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, UserUsageSummary{TokenUsed: 65, RequestCount: 2, ConsumedQuota: 600}, recovered[101])

	// 5. Historical closed range gets longer TTL.
	histEnd := time.Now().Unix() - 3600
	hist, err := GetUsersUsageSummaries(t.Context(), []int{101}, 500, histEnd)
	require.NoError(t, err)
	assert.Equal(t, UserUsageSummary{TokenUsed: 65, RequestCount: 2, ConsumedQuota: 600}, hist[101])
	histTTL := server.TTL(userUsageSummaryCacheKey(101, 500, histEnd))
	assert.Greater(t, histTTL, userUsageSummaryCacheTTL)
	assert.LessOrEqual(t, histTTL, userUsageSummaryHistoryCacheTTL)

	// 6. Redis outage falls back cleanly to LOG_DB without failing.
	server.Close()
	fallback, err := GetUsersUsageSummaries(t.Context(), []int{101, 103}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, UserUsageSummary{TokenUsed: 65, RequestCount: 2, ConsumedQuota: 600}, fallback[101])
	assert.Equal(t, UserUsageSummary{TokenUsed: 10, RequestCount: 1, ConsumedQuota: 75}, fallback[103])
}
