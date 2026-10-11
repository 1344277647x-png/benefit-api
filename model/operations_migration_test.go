package model

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Opt-in isolated databases; never set these variables to production DSNs.
func TestOperationsExternalDatabaseMigration(t *testing.T) {
	for _, engine := range []string{"mysql", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			dsn := os.Getenv("OPERATIONS_TEST_" + engine + "_DSN")
			if dsn == "" {
				t.Skip("isolated database not configured")
			}
			var dialector gorm.Dialector
			if engine == "mysql" {
				dialector = mysql.Open(dsn)
			} else {
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&OperationsPool{}, &OperationsEntry{}, &ChannelCostVersion{}, &OperationsState{}))
				require.NoError(t, EnsureOperationsStart(db))
			}
			var count int64
			require.NoError(t, db.Model(&OperationsState{}).Count(&count).Error)
			require.Equal(t, int64(1), count)
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				return OperationsCreditTx(tx, "migration:pool", "migration:fund", "receipt", 1, 6400, "59", "0", 1)
			}))
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				entry, err := OperationsTakeTx(tx, "migration:pool", "migration:use", "migration-request", "subscription_usage", 1, 1000, 2)
				require.NoError(t, err)
				require.Equal(t, "9.21875", entry.ValueUSD)
				return nil
			}))
		})
	}
}
