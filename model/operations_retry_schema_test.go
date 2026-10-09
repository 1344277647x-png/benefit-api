package model

import (
	"os"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOperationsRetryColumnsMigrateLegacyRows(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		open      func(string) gorm.Dialector
	}{
		{"sqlite", "", func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_OPERATIONS_MYSQL_DSN", func(d string) gorm.Dialector { return mysql.Open(d) }},
		{"postgres", "TEST_OPERATIONS_POSTGRES_DSN", func(d string) gorm.Dialector { return postgres.Open(d) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if tc.env != "" && dsn == "" {
				t.Skip("local database not configured")
			}
			if tc.env != "" {
				require.True(t, strings.Contains(dsn, "127.0.0.1") && strings.Contains(dsn, "benefit_schema_round3"))
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			defer sqlDB.Close()
			require.False(t, db.Migrator().HasTable(&BusinessEvent{}), "isolated fresh database required")
			legacy := struct {
				Id          int64
				EventKey    string
				DeliveredAt int64
			}{EventKey: "legacy"}
			require.NoError(t, db.Table("business_events").AutoMigrate(&legacy))
			require.NoError(t, db.Table("business_events").Create(&legacy).Error)
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&BusinessEvent{}, &GenerationAsset{}))
			}
			var rows []BusinessEvent
			require.NoError(t, db.Where("delivered_at = 0 AND dead_letter_at = 0 AND next_attempt_at <= ?", int64(1)).Find(&rows).Error)
			require.Len(t, rows, 1, "old events must not disappear due to NULL retry columns")
			assert.Zero(t, rows[0].Attempts)
			asset := GenerationAsset{PublicID: "asset", RelativePath: "test", ExpiresAt: 1}
			require.NoError(t, db.Create(&asset).Error)
			var count int64
			require.NoError(t, db.Model(&GenerationAsset{}).Where("cleanup_retry_at = 0").Count(&count).Error)
			assert.EqualValues(t, 1, count)
		})
	}
}
