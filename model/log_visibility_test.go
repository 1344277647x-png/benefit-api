package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUserLogQueriesHideErrorsAndPreserveAdminRecords(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) { verifyUserLogVisibility(t, dialect) })
	}
}

func verifyUserLogVisibility(t *testing.T, dialect string) {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(":memory:")
	case "mysql":
		dsn := os.Getenv("TEST_LOG_VISIBILITY_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_LOG_VISIBILITY_MYSQL_DSN is not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_LOG_VISIBILITY_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_LOG_VISIBILITY_POSTGRES_DSN is not configured")
		}
		driver = postgres.Open(dsn)
	}
	db, err := gorm.Open(driver, &gorm.Config{})
	require.NoError(t, err)
	previous := LOG_DB
	previousType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseType(dialect))
	t.Cleanup(func() {
		LOG_DB = previous
		common.SetLogDatabaseType(previousType)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
	var version string
	versionQuery := "SELECT version()"
	if dialect == "sqlite" {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database: %s %s", dialect, version)
	require.NoError(t, db.AutoMigrate(&Log{}, &AuditLog{}))
	var existing int64
	require.NoError(t, db.Model(&Log{}).Count(&existing).Error)
	require.Zero(t, existing, "visibility tests require an empty isolated log database")
	fixtures := []Log{
		{UserId: 1, TokenId: 7, Type: LogTypeConsume, RequestId: "consume"},
		{UserId: 1, TokenId: 7, Type: LogTypeError, RequestId: "failure"},
		{UserId: 1, TokenId: 7, Type: LogTypeRefund, RequestId: "refund"},
		{UserId: 2, TokenId: 8, Type: LogTypeConsume, RequestId: "other-user"},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "quota-add", Other: `{"op":{"action":"user.quota_add"}}`, Content: "Localized adjustment"},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "quota-subtract", Other: "{\n  \"op\": {\"action\": \"user.quota_subtract\"}\n}"},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "quota-override", Other: `{"op":{"action":"user.quota_override"}}`},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "legacy-add", Content: "Increased user quota by 500"},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "legacy-subtract", Content: "Decreased user quota by 500", Other: "malformed"},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "legacy-override", Content: "Overrode user quota from 1000 to 500"},
		{UserId: 1, TokenId: 7, Type: LogTypeManage, RequestId: "manage-adjustment", Other: `{"op":{"action":"user.quota_add"}}`},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "payment", Content: "Paid recharge", Other: `{"op":{"action":"payment.recharge"},"admin_info":{"payment_method":"alipay"}}`},
		{UserId: 1, TokenId: 7, Type: LogTypeTopup, RequestId: "legacy-payment", Content: "Paid recharge", Other: "malformed"},
	}
	for i := range fixtures {
		fixtures[i].CreatedAt = int64(i + 1)
	}
	require.NoError(t, db.Create(&fixtures).Error)

	visible := []string{"legacy-payment", "payment", "refund", "consume"}
	for offset, requestID := range visible {
		logs, total, err := GetUserLogs(1, LogFilter{}, offset, 1)
		require.NoError(t, err)
		assert.EqualValues(t, len(visible), total)
		require.Len(t, logs, 1)
		assert.Equal(t, requestID, logs[0].RequestId)
	}
	logs, total, err := GetUserLogs(1, LogFilter{}, len(visible), 1)
	require.NoError(t, err)
	assert.EqualValues(t, len(visible), total)
	assert.Empty(t, logs)
	for _, query := range []LogFilter{
		{Type: LogTypeError}, {Type: LogTypeManage}, {RequestId: "failure"},
		{RequestId: "quota-add"}, {RequestId: "quota-subtract"}, {RequestId: "quota-override"},
		{Type: LogTypeTopup, RequestId: "legacy-add"}, {RequestId: "legacy-subtract"}, {RequestId: "legacy-override"},
	} {
		logs, total, err = GetUserLogs(1, query, 0, 10)
		require.NoError(t, err)
		assert.Empty(t, logs)
		assert.Zero(t, total)
		require.NoError(t, StreamUserLogs(t.Context(), 1, query, func(*Log) error {
			t.Error("a hidden administrator record reached a personal export")
			return nil
		}))
	}
	logs, total, err = GetUserLogs(1, LogFilter{Type: LogTypeTopup}, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	logs, err = GetLogByTokenId(7)
	require.NoError(t, err)
	var tokenRequests []string
	for _, log := range logs {
		tokenRequests = append(tokenRequests, log.RequestId)
	}
	assert.Equal(t, visible, tokenRequests)
	var exportRequests []string
	require.NoError(t, StreamUserLogs(t.Context(), 1, LogFilter{}, func(log *Log) error {
		exportRequests = append(exportRequests, log.RequestId)
		return nil
	}))
	assert.Equal(t, visible, exportRequests)
	logs, total, err = GetAllLogs(LogFilter{Type: LogTypeError}, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	assert.Equal(t, "failure", logs[0].RequestId)
	logs, total, err = GetAllLogs(LogFilter{}, 0, 30)
	require.NoError(t, err)
	assert.EqualValues(t, len(fixtures), total)
	assert.Len(t, logs, len(fixtures))
	var adminExport []string
	require.NoError(t, StreamAllLogs(t.Context(), LogFilter{}, common.RoleRootUser, func(log *Log) error {
		adminExport = append(adminExport, log.RequestId)
		return nil
	}))
	assert.Len(t, adminExport, len(fixtures))
	assert.Contains(t, adminExport, "quota-subtract")
	var stored []Log
	require.NoError(t, db.Order("id").Find(&stored).Error)
	assert.Equal(t, fixtures, stored, "visibility changes must never edit or remove the original records")

	for _, action := range []string{"user.quota_add", "user.quota_subtract", "user.quota_override", "user.login"} {
		require.NoError(t, db.Create(&AuditLog{EventId: action, UserId: 1, ActorRole: common.RoleAdminUser, Action: action}).Error)
	}
	for _, view := range []struct {
		self bool
		role int
	}{{true, common.RoleCommonUser}, {true, common.RoleRootUser}, {false, common.RoleCommonUser}} {
		audits, total, err := GetAuditLogs(AuditLogFilter{UserId: 1, SelfView: view.self}, 0, 20, view.role)
		require.NoError(t, err)
		assert.EqualValues(t, 1, total)
		require.Len(t, audits, 1)
		assert.Equal(t, "user.login", audits[0].Action)
	}
	audits, total, err := GetAuditLogs(AuditLogFilter{}, 0, 20, common.RoleAdminUser)
	require.NoError(t, err)
	assert.EqualValues(t, 4, total)
	assert.Len(t, audits, 4)
}
