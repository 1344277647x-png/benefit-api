package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingExceptionSurvivesIndependentLogDatabaseFailure(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.BusinessEvent{}, &model.BusinessEventLogReceipt{}))
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	info := &relaycommon.RelayInfo{RequestId: t.Name(), UserId: 42, TokenId: 43, BillingSource: BillingSourceWallet}
	info.Billing = &BillingSession{preConsumedQuota: 100, tokenConsumed: 100, fundingSettled: true}
	// Use an isolated log DB without tables to force delivery failure while
	// the primary outbox remains available.
	broken, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := broken.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	previous := model.LOG_DB
	t.Cleanup(func() { model.LOG_DB = previous })
	model.LOG_DB = broken
	recordPartialSettlementException(c, info, 40)
	recordPartialSettlementException(c, info, 40)
	var events []model.BusinessEvent
	require.NoError(t, model.DB.Where("request_id = ?", info.RequestId).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].DetailsJSON, "token_adjustment_failed")
	require.Error(t, model.DeliverBusinessEvent(context.Background(), events[0]))
	model.LOG_DB = previous
	require.NoError(t, model.DeliverBusinessEvent(context.Background(), events[0]))
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", info.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, model.LogTypeError, logs[0].Type)
	assert.Zero(t, logs[0].Quota)
	assert.Contains(t, logs[0].Content, "预扣=100")
	// If the primary database fails, preserve details in the independent log DB.
	primary := model.DB
	t.Cleanup(func() { model.DB = primary })
	model.DB = broken
	info.RequestId = t.Name() + "-primary-down"
	recordPartialSettlementException(c, info, 40)
	model.DB = primary
	logs = nil
	require.NoError(t, model.LOG_DB.Where("request_id = ?", info.RequestId).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Contains(t, logs[0].Other, `"expected_quota":40`)
	assert.Contains(t, logs[0].Other, "token_adjustment_failed")
}
