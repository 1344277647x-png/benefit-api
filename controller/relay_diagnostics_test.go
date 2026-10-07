package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayErrorLogContainsDiagnosticsWithoutProviderBody(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	oldEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = oldEnabled })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("id", 7654)
	c.Set("channel_id", 91)
	c.Set(common.RequestIdKey, "diagnostics-regression")
	d := relaycommon.NewStreamDiagnostics(time.Unix(100, 0))
	d.Event(time.Unix(101, 0))
	d.Finish(time.Unix(102, 0))
	c.Set(relaycommon.StreamDiagnosticsContextKey, d)
	providerError := types.NewOpenAIError(errors.New("Authorization: Bearer synthetic-secret; echoed prompt: synthetic-private-content"), types.ErrorCodeBadResponse, http.StatusBadGateway)
	processChannelError(c, *types.NewChannelError(91, constant.ChannelTypeOpenAI, "test-channel", false, "", false), providerError)
	var log model.Log
	require.NoError(t, db.Where("request_id = ?", "diagnostics-regression").First(&log).Error)
	require.NotContains(t, log.Content+log.Other, "synthetic-secret")
	require.NotContains(t, log.Content+log.Other, "synthetic-private-content")
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	admin, ok := other["admin_info"].(map[string]interface{})
	require.True(t, ok)
	require.Contains(t, admin, "stream_diagnostics")
}
