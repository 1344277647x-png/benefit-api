package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
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
			oldDB, oldType := DB, common.MainDatabaseType()
			DB = db
			if engine == "mysql" {
				common.SetMainDatabaseType(common.DatabaseTypeMySQL)
			} else {
				common.SetMainDatabaseType(common.DatabaseTypePostgreSQL)
			}
			t.Cleanup(func() { DB = oldDB; common.SetMainDatabaseType(oldType) })
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			for i := 0; i < 2; i++ {
				require.NoError(t, db.AutoMigrate(&OperationsPool{}, &OperationsEntry{}, &ChannelCostVersion{}, &OperationsState{}, &OperationsEvent{}, &OperationsLane{}))
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
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				return enqueueOperationsTx(tx, "migration:async", "migration-async", operationsFact{Action: "usage", PoolKey: "migration:pool", UserId: 1, Quota: 1000, Kind: "subscription_usage"})
			}))
			require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
			require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
			var receipt OperationsEvent
			require.NoError(t, db.First(&receipt, "event_key = ?", "migration:async").Error)
			require.True(t, receipt.Done)
			// Exercise a real SQL failure/savepoint and subsequent recovery.
			require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
				return enqueueOperationsTx(tx, "migration:retry", "migration-retry", operationsFact{Action: "usage", PoolKey: "migration:pool", UserId: 1, Quota: 1000, Kind: "subscription_usage"})
			}))
			require.NoError(t, db.Migrator().RenameTable(&OperationsEntry{}, "operations_entries_offline"))
			require.Error(t, ProcessOperationsEvents(context.Background(), 200))
			require.NoError(t, db.Migrator().RenameTable("operations_entries_offline", "operations_entries"))
			require.NoError(t, db.Model(&OperationsEvent{}).Where("event_key = ?", "migration:retry").Update("next_attempt_at", 0).Error)
			require.NoError(t, ProcessOperationsEvents(context.Background(), 200))
			receipt = OperationsEvent{}
			require.NoError(t, db.First(&receipt, "event_key = ?", "migration:retry").Error)
			require.True(t, receipt.Done)
			require.Equal(t, 1, receipt.Attempts)
		})
	}
}
