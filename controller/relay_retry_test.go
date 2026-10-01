package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryNeverSwitchesAfterStreamWriteForChannelError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("relay_stream_response_written", true)
	err := types.NewErrorWithStatusCode(io.ErrUnexpectedEOF, types.ErrorCodeBadResponse, http.StatusBadGateway)

	assert.False(t, shouldRetry(c, err, 2))
}

func TestShouldRetryNeverSwitchesWhenWriterHasBytesWithoutContextMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	_, err := c.Writer.Write([]byte("partial"))
	require.NoError(t, err)
	apiErr := types.NewErrorWithStatusCode(io.ErrUnexpectedEOF, types.ErrorCodeBadResponse, http.StatusBadGateway)

	assert.False(t, c.GetBool("relay_stream_response_written"))
	assert.True(t, relayStreamResponseWritten(c))
	assert.False(t, shouldRetry(c, apiErr, 2))
}

func TestShouldRetryDoesNotFailOverForUpstreamQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.WithOpenAIError(types.OpenAIError{
		Type:    "invalid_request_error",
		Code:    "insufficient_quota",
		Message: "credits exhausted",
	}, http.StatusPaymentRequired)

	assert.False(t, shouldRetry(c, err, 2))
}
