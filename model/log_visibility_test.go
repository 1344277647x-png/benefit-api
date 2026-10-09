package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserLogQueriesHideErrorsAndPreserveAdminRecords(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previous := LOG_DB
	previousType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previous
		common.SetLogDatabaseType(previousType)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&Log{}))
	fixtures := []Log{
		{UserId: 1, TokenId: 7, Type: LogTypeConsume, RequestId: "consume"},
		{UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "failure"},
		{UserId: 1, TokenId: 7, Type: LogTypeRefund, RequestId: "refund"},
		{UserId: 2, TokenId: 8, Type: LogTypeConsume, RequestId: "other-user"},
	}
	require.NoError(t, db.Create(&fixtures).Error)

	logs, total, err := GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 0, 1, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "refund", logs[0].RequestId)
	logs, total, err = GetUserLogs(1, LogTypeUnknown, 0, 0, "", "", 1, 1, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "consume", logs[0].RequestId)
	for _, query := range []struct {
		logType   int
		requestID string
	}{
		{LogTypeError, ""}, {LogTypeUnknown, "failure"},
	} {
		logs, total, err = GetUserLogs(1, query.logType, 0, 0, "", "", 0, 10, "", query.requestID, "")
		require.NoError(t, err)
		assert.Empty(t, logs)
		assert.Zero(t, total)
	}
	logs, err = GetLogByTokenId(7)
	require.NoError(t, err)
	require.Len(t, logs, 2)
	assert.Equal(t, "refund", logs[0].RequestId)
	assert.Equal(t, "consume", logs[1].RequestId)
	logs, total, err = GetAllLogs(LogTypeError, 0, 0, "", "", "", 0, 10, 0, "", "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "failure", logs[0].RequestId)
	var stored int64
	require.NoError(t, db.Model(&Log{}).Count(&stored).Error)
	assert.EqualValues(t, 4, stored)
}
