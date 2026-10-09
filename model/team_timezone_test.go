package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTeamPeriodTimezoneAndLegacyUsagePreserved(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&TeamSubscription{}, &TeamQuotaPeriod{}))
	sub := TeamSubscription{StartTime: time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC).Unix(), EndTime: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC).Unix(), ResetPeriod: SubscriptionResetDaily, AmountTotal: 100}
	require.NoError(t, DB.Create(&sub).Error)
	now := sub.StartTime + 3600
	oldZone := time.Local
	t.Cleanup(func() { time.Local = oldZone })
	time.Local = time.UTC
	start, end := teamSubscriptionPeriodBounds(&sub, now)
	time.Local = time.FixedZone("other", -5*3600)
	start2, end2 := teamSubscriptionPeriodBounds(&sub, now)
	assert.Equal(t, start, start2)
	assert.Equal(t, end, end2)
	assert.Equal(t, time.Date(2026, 10, 9, 16, 0, 0, 0, time.UTC).Unix(), end)
	legacy := TeamQuotaPeriod{TeamSubscriptionId: sub.Id, StartTime: sub.StartTime, EndTime: sub.StartTime + 24*3600, AmountTotal: 100, AmountUsed: 73}
	require.NoError(t, DB.Create(&legacy).Error)
	period, err := currentTeamPeriodTx(DB, &sub, now)
	require.NoError(t, err)
	assert.Equal(t, legacy.Id, period.Id)
	assert.EqualValues(t, 73, period.AmountUsed)
	period, err = currentTeamPeriodTx(DB, &sub, legacy.EndTime)
	require.NoError(t, err)
	assert.Equal(t, legacy.EndTime, period.StartTime, "new period cannot overlap the legacy tail")
}
