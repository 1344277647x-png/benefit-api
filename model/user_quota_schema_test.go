package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestWalletQuotaSchemaFreshAndRepeatedMigration(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		kind      common.DatabaseType
		open      func(string) gorm.Dialector
	}{
		{"sqlite", "", common.DatabaseTypeSQLite, func(string) gorm.Dialector { return sqlite.Open(":memory:") }},
		{"mysql", "TEST_WALLET_MYSQL_DSN", common.DatabaseTypeMySQL, func(s string) gorm.Dialector { return mysql.Open(s) }},
		{"postgres", "TEST_WALLET_POSTGRES_DSN", common.DatabaseTypePostgreSQL, func(s string) gorm.Dialector { return postgres.Open(s) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := os.Getenv(tc.env)
			if tc.env != "" {
				if dsn == "" {
					t.Skip(tc.env + " not configured")
				}
				require.True(t, strings.Contains(dsn, "127.0.0.1") && strings.Contains(dsn, "benefit_wallet_test"), "isolated local database required")
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			require.False(t, db.Migrator().HasTable(&User{}), "test database must be fresh")
			// Calibrate the report's old tag against the actual drivers. GORM's
			// MySQL dialect may widen Go int despite an explicit type:int tag.
			legacy := struct {
				Quota int `gorm:"type:int;default:0"`
			}{}
			require.NoError(t, db.Table("legacy_wallet_probe").AutoMigrate(&legacy))
			legacyColumns, err := db.Migrator().ColumnTypes("legacy_wallet_probe")
			require.NoError(t, err)
			require.Len(t, legacyColumns, 1)
			t.Logf("legacy type:int on %s produces %s", tc.name, legacyColumns[0].DatabaseTypeName())
			require.NoError(t, ensureUserQuotaColumns(db, tc.kind))
			require.NoError(t, db.AutoMigrate(&User{}))
			require.NoError(t, ensureUserQuotaColumns(db, tc.kind))
			value := common.MaxWalletQuota
			user := User{Username: "large-wallet", Quota: value, UsedQuota: value, AffQuota: value, AffHistoryQuota: value}
			require.NoError(t, db.Create(&user).Error)
			require.NoError(t, db.AutoMigrate(&User{}))
			require.NoError(t, ensureUserQuotaColumns(db, tc.kind))
			var stored User
			require.NoError(t, db.First(&stored, user.Id).Error)
			assert.Equal(t, value, stored.Quota)
			assert.Equal(t, value, stored.UsedQuota)
			assert.Equal(t, value, stored.AffQuota)
			assert.Equal(t, value, stored.AffHistoryQuota)
			if tc.kind != common.DatabaseTypeSQLite {
				require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("quota", 0).Error)
				if tc.kind == common.DatabaseTypeMySQL {
					require.NoError(t, db.Exec("ALTER TABLE users MODIFY quota INT NOT NULL DEFAULT 0").Error)
				} else {
					require.NoError(t, db.Exec("ALTER TABLE users ALTER COLUMN quota TYPE INTEGER").Error)
				}
				require.ErrorContains(t, ensureUserQuotaColumns(db, tc.kind), "32-bit is not supported")
			}
		})
	}
}
