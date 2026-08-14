package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordChannelHealthSampleSeparatesTTFTAndResponseLatency(t *testing.T) {
	truncateTables(t)
	bucketTime := time.Now().Unix()

	require.NoError(t, RecordChannelHealthSample(ChannelHealthSample{
		ChannelID:    7,
		ModelName:    "model-a",
		EndpointType: "openai",
		SampledAt:    bucketTime,
		Success:      true,
		LatencyMs:    12_000,
		TTFTMs:       500,
		HasTTFT:      true,
	}))
	require.NoError(t, RecordChannelHealthSample(ChannelHealthSample{
		ChannelID:    7,
		ModelName:    "model-a",
		EndpointType: "openai",
		SampledAt:    bucketTime,
		Success:      true,
		LatencyMs:    800,
	}))

	var bucket ChannelHealthBucket
	require.NoError(t, DB.Where("channel_id = ? AND model_name = ?", 7, "model-a").First(&bucket).Error)
	require.EqualValues(t, 2, bucket.RequestCount)
	require.EqualValues(t, 800, bucket.TotalLatencyMs)
	require.EqualValues(t, 1, bucket.LatencyCount)
	require.EqualValues(t, 500, bucket.TTFTSumMs)
	require.EqualValues(t, 1, bucket.TTFTCount)

	var snapshot ChannelHealthSnapshot
	require.NoError(t, DB.Where("channel_id = ? AND model_name = ?", 7, "model-a").First(&snapshot).Error)
	require.Equal(t, "openai", snapshot.EndpointType)
	require.EqualValues(t, bucketTime, snapshot.LastSampleAt)
}
