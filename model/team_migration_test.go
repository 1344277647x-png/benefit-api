package model

import (
	"context"
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

// Supply only a disposable, empty database in each DSN. Production credentials
// must never be used by this test; an omitted DSN skips the corresponding DB.
func TestTeamMigrationConfiguredDatabases(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		dbType    common.DatabaseType
		open      func(string) gorm.Dialector
	}{
		{"sqlite", "", common.DatabaseTypeSQLite, func(string) gorm.Dialector {
			return sqlite.Open("file:team_migration_sqlite?mode=memory&cache=shared")
		}},
		{"mysql", "TEST_MYSQL_DSN", common.DatabaseTypeMySQL, func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_POSTGRES_DSN", common.DatabaseTypePostgreSQL, func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(tc.env))
			if tc.env != "" && dsn == "" {
				t.Skip(tc.env + " not configured")
			}
			if tc.env != "" {
				require.True(t, strings.Contains(dsn, "127.0.0.1:") && strings.Contains(dsn, "/benefit_team_test"),
					"team migration tests require the named local disposable database")
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { _ = sqlDB.Close() })
			original, originalLog := DB, LOG_DB
			mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
			DB, LOG_DB = db, db
			common.SetDatabaseTypes(tc.dbType, tc.dbType)
			initCol()
			t.Cleanup(func() {
				DB, LOG_DB = original, originalLog
				common.SetDatabaseTypes(mainType, logType)
				initCol()
			})
			if os.Getenv("TEST_TEAM_EXPECT_EMPTY") == "1" {
				assert.False(t, db.Migrator().HasTable(&User{}), "first migration requires a genuinely empty main database")
				assert.False(t, db.Migrator().HasTable(&TeamUsage{}))
			}
			require.NoError(t, migrateDB())
			require.NoError(t, migrateDB(), "restarting with the same schema must also succeed")
			assert.True(t, db.Migrator().HasTable(&BusinessEventLogReceipt{}),
				"the main database must contain the outbox receipt table when LOG_SQL_DSN is unset")
			require.NoError(t, migrateLOGDB())
			require.NoError(t, migrateLOGDB(), "log delivery receipt migration must be repeatable")
			for _, table := range []interface{}{&Team{}, &TeamMember{}, &TeamInvitation{}, &TeamSubscription{}, &TeamQuotaPeriod{}, &TeamOrder{}, &TeamUsage{}, &TeamSyncBillingEvent{}, &TeamSyncLogReceipt{}, &TeamTaskBillingEvent{}, &TeamTaskLogReceipt{}, &BusinessEventLogReceipt{}} {
				assert.True(t, db.Migrator().HasTable(table))
			}
			assert.True(t, db.Migrator().HasIndex(&TeamUsage{}, "idx_team_usages_request_id"))
			assert.True(t, db.Migrator().HasColumn(&TeamUsage{}, "pending_final_amount"))
			assert.True(t, db.Migrator().HasColumn(&TeamUsage{}, "pending_final"))
			assert.True(t, db.Migrator().HasColumn(&TeamUsage{}, "attempted_at"))
			assert.True(t, db.Migrator().HasColumn(&TeamUsage{}, "evidence_ref"))
			assert.True(t, db.Migrator().HasColumn(&TeamSyncBillingEvent{}, "billing_details"))
			assert.True(t, db.Migrator().HasColumn(&TeamSyncBillingEvent{}, "response_state"))
			assert.True(t, db.Migrator().HasColumn(&TeamSyncBillingEvent{}, "credited_quota"))
			assert.True(t, db.Migrator().HasColumn(&TeamSyncBillingEvent{}, "credit_ref"))
			t.Run("recovery_and_log_replay", testTeamDatabaseRecoveryAndLogReplay)
		})
	}
}

// Exercise the actual recovery query, row locks and log receipt conflict
// handling on each configured database, not just schema creation. Rolling
// back the fixture leaves the migrated schema available for restart checks.
func testTeamDatabaseRecoveryAndLogReplay(t *testing.T) {
	original, originalLog := DB, LOG_DB
	tx := original.Begin()
	require.NoError(t, tx.Error)
	DB, LOG_DB = tx, tx
	defer func() {
		_ = tx.Rollback().Error
		DB, LOG_DB = original, originalLog
	}()
	users := []User{
		{Username: "migration-blocked", AffCode: "migration-blocked", Quota: 1000},
		{Username: "migration-ready", AffCode: "migration-ready", Quota: 1000},
	}
	require.NoError(t, tx.Create(&users).Error)
	// The billing invariant needs a real channel row, without exercising the
	// channel's provider-specific JSON serializer in this isolated fixture.
	require.NoError(t, tx.Model(&Channel{}).Create(map[string]any{
		"name": "migration-recovery", "key": "local-fixture", "channel_info": "{}", "used_quota": 0,
	}).Error)
	var channel Channel
	require.NoError(t, tx.Select("id", "used_quota").Where("name = ?", "migration-recovery").First(&channel).Error)
	usages := make([]TeamUsage, 2)
	for i := range users {
		team := Team{OwnerId: users[i].Id, Name: users[i].Username, Status: TeamStatusActive}
		require.NoError(t, tx.Create(&team).Error)
		reserved, final := int64(100), int64(110)
		if i == 1 {
			reserved, final = 30, 40
		}
		now := GetDBTimestamp()
		sub := TeamSubscription{TeamId: team.Id, SeatLimit: 1, AmountTotal: 100,
			StartTime: now - 60, EndTime: now + 3600, Status: TeamStatusActive}
		require.NoError(t, tx.Create(&sub).Error)
		period := TeamQuotaPeriod{TeamSubscriptionId: sub.Id, StartTime: sub.StartTime,
			EndTime: sub.EndTime, AmountTotal: 100, AmountUsed: reserved}
		require.NoError(t, tx.Create(&period).Error)
		usages[i] = TeamUsage{RequestId: users[i].Username, TeamId: team.Id, MemberUserId: users[i].Id,
			PeriodId: period.Id, Reserved: reserved, Status: "reserved", AttemptedAt: GetDBTimestamp()}
		require.NoError(t, tx.Create(&usages[i]).Error)
		require.NoError(t, RecordTeamSyncSettlementIntent(usages[i].RequestId, final, TeamSyncBillingMeta{
			ChannelId: channel.Id, Group: "default", ModelName: "local-model",
			PromptTokens: 8, CompletionTokens: 2, BillingDetails: `{"cache_tokens":4,"cache_ratio":0.1,"group_ratio":1.5}`,
		}))
	}
	recovered, err := RecoverPendingTeamSettlements(1)
	require.NoError(t, err)
	assert.Equal(t, 1, recovered, "an unaffordable older request must not starve another team")
	recovered, err = RecoverPendingTeamSettlements(1)
	require.NoError(t, err)
	assert.Zero(t, recovered)
	require.NoError(t, tx.First(&usages[0], usages[0].Id).Error)
	assert.Equal(t, "reserved", usages[0].Status)
	require.NoError(t, tx.First(&usages[1], usages[1].Id).Error)
	assert.Equal(t, "settled", usages[1].Status)
	assert.EqualValues(t, 40, usages[1].FinalAmount)
	var event TeamSyncBillingEvent
	require.NoError(t, tx.Where("request_id = ?", usages[1].RequestId).First(&event).Error)
	for i := 0; i < 2; i++ {
		require.NoError(t, DeliverTeamSyncBillingLog(context.Background(), event))
	}
	var logs []Log
	require.NoError(t, tx.Where("request_id = ?", event.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1, "replaying a committed receipt must not duplicate the log")
	assert.Equal(t, 40, logs[0].Quota)
	assert.Contains(t, logs[0].Other, `"cache_tokens":4`)
	require.NoError(t, tx.First(&users[1], users[1].Id).Error)
	assert.Equal(t, 1000, users[1].Quota)
	assert.EqualValues(t, 40, users[1].UsedQuota)
	assert.Equal(t, 1, users[1].RequestCount)
	require.NoError(t, tx.Select("id", "used_quota").First(&channel, channel.Id).Error)
	assert.EqualValues(t, 40, channel.UsedQuota)
	require.NoError(t, MarkTeamSyncResponseState(usages[1].RequestId, "withheld"))
	applied, credit, err := CreditTeamDeliveryDispute(usages[1].RequestId, users[0].Id, "local-review:dispute-123")
	require.NoError(t, err)
	assert.True(t, applied)
	assert.EqualValues(t, 40, credit)
	applied, credit, err = CreditTeamDeliveryDispute(usages[1].RequestId, users[0].Id, "local-review:dispute-123")
	require.NoError(t, err)
	assert.False(t, applied)
	assert.EqualValues(t, 40, credit)
	var readyPeriod TeamQuotaPeriod
	require.NoError(t, tx.First(&readyPeriod, usages[1].PeriodId).Error)
	assert.Zero(t, readyPeriod.AmountUsed)
	var afterCredit []Log
	require.NoError(t, tx.Where("request_id = ?", event.RequestId).Find(&afterCredit).Error)
	require.Len(t, afterCredit, 1, "an account credit must not duplicate or erase the original consume log")
	assert.Equal(t, 40, afterCredit[0].Quota)
}
