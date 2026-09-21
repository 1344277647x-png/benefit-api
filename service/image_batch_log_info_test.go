package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendImageBatchLogInfoKeepsErrorsUnderAdminInfo(t *testing.T) {
	other := map[string]interface{}{}
	batch := &relaycommon.ImageBatchInfo{
		Mode:           "fanout",
		RequestedCount: 4,
		ResultCount:    2,
		FailedCount:    2,
		ReferenceCount: 3,
		Resolution:     "4K",
		AspectRatio:    "16:9",
		ResolvedSize:   "3840x2160",
		Errors: []relaycommon.ImageBatchError{{
			Index:      2,
			StatusCode: 502,
			Code:       "bad_response",
			Message:    "sanitized summary",
		}},
	}

	AppendImageBatchLogInfo(other, batch)

	assert.Equal(t, "fanout", other["batch_mode"])
	assert.Equal(t, 4, other["requested_count"])
	assert.Equal(t, 2, other["result_count"])
	assert.Equal(t, 2, other["failed_count"])
	assert.Equal(t, 3, other["reference_count"])
	assert.Equal(t, "4K", other["resolution"])
	assert.Equal(t, "16:9", other["aspect_ratio"])
	assert.Equal(t, "3840x2160", other["resolved_size"])
	assert.NotContains(t, other, "image_batch_errors")
	adminInfo, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, batch.Errors, adminInfo["image_batch_errors"])
}
