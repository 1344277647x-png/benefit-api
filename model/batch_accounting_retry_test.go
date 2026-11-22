package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBatchAccountingRetainsFailedIncrements(t *testing.T) {
	require.NoError(t, FlushBatchUpdates())
	user := User{Username: t.Name(), AffCode: t.Name(), Quota: 100}
	require.NoError(t, DB.Create(&user).Error)
	name := "test:batch_failure"
	require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(errors.New("simulated database write failure"))
		}
	}))
	t.Cleanup(func() { _ = DB.Callback().Update().Remove(name) })
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -7)
	addNewRecord(BatchUpdateTypeUsedQuota, user.Id, 7)
	addNewRecord(BatchUpdateTypeRequestCount, user.Id, 1)
	require.Error(t, FlushBatchUpdates())
	addNewRecord(BatchUpdateTypeUserQuota, user.Id, -3)
	addNewRecord(BatchUpdateTypeUsedQuota, user.Id, 3)
	require.NoError(t, DB.Callback().Update().Remove(name))
	require.NoError(t, FlushBatchUpdates())
	require.NoError(t, FlushBatchUpdates(), "second flush must not apply increments twice")
	require.NoError(t, DB.First(&user, user.Id).Error)
	assert.Equal(t, 90, user.Quota)
	assert.Equal(t, 10, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
}
