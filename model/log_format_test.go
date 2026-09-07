package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

func TestFormatUserLogsStripsImageBatchErrorsButKeepsCounts(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"batch_mode":      "fanout",
		"requested_count": 4,
		"result_count":    2,
		"failed_count":    2,
		"reference_count": 3,
		"admin_info": map[string]interface{}{
			"image_batch_errors": []map[string]interface{}{{
				"index":       2,
				"status_code": 502,
				"code":        "bad_response",
				"message":     "sanitized summary",
			}},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	require.NotContains(t, parsed, "admin_info")
	require.Equal(t, "fanout", parsed["batch_mode"])
	require.Equal(t, float64(4), parsed["requested_count"])
	require.Equal(t, float64(2), parsed["result_count"])
	require.Equal(t, float64(2), parsed["failed_count"])
}
