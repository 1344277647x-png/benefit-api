package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestEvaluateChannelHealthUsesNonStreamingLatency(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	view := EvaluateChannelHealth(now, model.ChannelHealthSnapshot{
		ChannelID:           1,
		ModelName:           "model-a",
		LastSampleAt:        now.Unix(),
		ConsecutiveFailures: 0,
	}, model.ChannelHealthWindow{
		RequestCount:   2,
		SuccessCount:   2,
		LatencyCount:   2,
		TotalLatencyMs: 22_000,
	})

	require.Equal(t, ChannelHealthDelayed, view.Status)
	require.EqualValues(t, 11_000, view.AverageLatencyMs)
	require.Zero(t, view.AverageTTFTMs)
}

func TestEvaluateChannelHealthUsesStreamingTTFT(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	view := EvaluateChannelHealth(now, model.ChannelHealthSnapshot{
		ChannelID:    1,
		ModelName:    "model-a",
		LastSampleAt: now.Unix(),
	}, model.ChannelHealthWindow{
		RequestCount: 2,
		SuccessCount: 2,
		TTFTCount:    2,
		TTFTSumMs:    22_000,
	})

	require.Equal(t, ChannelHealthDelayed, view.Status)
	require.Zero(t, view.AverageLatencyMs)
	require.EqualValues(t, 11_000, view.AverageTTFTMs)
}

func TestEvaluateChannelHealthMarksFailureStreakUnavailable(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	view := EvaluateChannelHealth(now, model.ChannelHealthSnapshot{
		ChannelID:           1,
		ModelName:           "model-a",
		LastSampleAt:        now.Unix(),
		ConsecutiveFailures: 5,
	}, model.ChannelHealthWindow{
		RequestCount: 5,
		FailureCount: 5,
	})

	require.Equal(t, ChannelHealthUnavailable, view.Status)
	require.EqualValues(t, 0, view.SuccessRate)
}

func TestEvaluateChannelHealthMarksStaleUnknown(t *testing.T) {
	now := time.Unix(2_000_000, 0)
	view := EvaluateChannelHealth(now, model.ChannelHealthSnapshot{
		ChannelID:    1,
		ModelName:    "model-a",
		LastSampleAt: now.Add(-16 * time.Minute).Unix(),
	}, model.ChannelHealthWindow{
		RequestCount: 10,
		SuccessCount: 10,
	})

	require.Equal(t, ChannelHealthUnknown, view.Status)
}
