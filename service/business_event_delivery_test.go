package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBusinessEventPoisonDoesNotBlockHealthyNeighbor(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.BusinessEvent{}, &model.BusinessEventLogReceipt{}))
	bad := model.BusinessEvent{EventKey: t.Name() + "-bad", Category: model.BusinessEventTeam, UserId: 42, DetailsJSON: "null"}
	good := model.BusinessEvent{EventKey: t.Name() + "-good", Category: model.BusinessEventLottery, UserId: 42, DetailsJSON: "{}"}
	require.NoError(t, model.DB.Create(&bad).Error)
	require.NoError(t, model.DB.Create(&good).Error)
	require.Error(t, DeliverPendingBusinessEvents(context.Background(), 200))
	require.NoError(t, model.DB.First(&bad, bad.Id).Error)
	require.NoError(t, model.DB.First(&good, good.Id).Error)
	assert.Equal(t, 1, bad.Attempts)
	assert.Positive(t, bad.NextAttemptAt)
	assert.Positive(t, good.DeliveredAt)
	// Retry the acknowledgement path: receipt prevents duplicate log entries.
	require.NoError(t, model.DeliverBusinessEvent(context.Background(), good))
	var receipts int64
	require.NoError(t, model.LOG_DB.Model(&model.BusinessEventLogReceipt{}).Where("event_key = ?", good.EventKey).Count(&receipts).Error)
	assert.EqualValues(t, 1, receipts)
	for bad.Attempts < 10 {
		require.NoError(t, model.MarkBusinessEventFailed(context.Background(), bad))
		require.NoError(t, model.DB.First(&bad, bad.Id).Error)
	}
	assert.Positive(t, bad.DeadLetterAt)
	assert.Zero(t, bad.DeliveredAt, "dead letters retain the original event for repair")
	events, err := model.PendingBusinessEvents(context.Background(), 200)
	require.NoError(t, err)
	for _, event := range events {
		assert.NotEqual(t, bad.Id, event.Id)
	}
}
