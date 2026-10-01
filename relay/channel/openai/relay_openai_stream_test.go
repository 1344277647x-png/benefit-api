package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiStreamHandlerStopsOnInBandQuotaError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","choices":[{"index":0,"delta":{"content":"partial"}}]}`,
		`data: {"id":"chatcmpl_1","choices":[{"index":0,"delta":{"content":" output"}}]}`,
		`data: {"error":{"message":"credits exhausted for account 999","type":"invalid_request_error","code":"insufficient_quota"}}`,
		`data: [DONE]`,
	}, "\n\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorClassUpstreamQuotaExhausted, apiErr.GetErrorClass())
	assert.Equal(t, types.ErrorCodeUpstreamQuotaExhausted, apiErr.GetErrorCode())
	assert.Equal(t, types.UpstreamQuotaPublicMessage, apiErr.ToOpenAIError().Message)
	assert.NotContains(t, recorder.Body.String(), "credits exhausted")
	assert.NotContains(t, recorder.Body.String(), "account 999")
	assert.NotContains(t, recorder.Body.String(), "insufficient_quota")
	assert.NotNil(t, info.StreamStatus)
}

func TestOaiStreamHandlerRejectsInBandErrorWithoutPartialOutput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"},
	}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(
			`data: {"error":{"message":"quota_exceeded","type":"invalid_request_error","code":"quota_exceeded"}}` + "\n\n",
		)),
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
	}

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorClassUpstreamQuotaExhausted, apiErr.GetErrorClass())
	assert.Empty(t, recorder.Body.String())
}

func TestOaiStreamHandlerRejectsEOFWithoutDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}}
	resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"))}
	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	assert.Equal(t, types.ErrorCodeBadResponse, apiErr.GetErrorCode())
}

func TestOaiStreamHandlerDoesNotApplyTextStreamTerminalRuleToAudioModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test-audio-model"}}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`data: {"id":"chatcmpl_1","model":"test-audio-model","choices":[{"index":0,"delta":{"content":"audio"}}]}` + "\n\n")),
	}
	_, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason)
}

func TestOpenaiHandlersRecognizeErrorWithoutType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name string
		hand func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{name: "chat", hand: OpenaiHandler},
		{name: "responses", hand: OaiResponsesHandler},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/test", nil)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "test"}}
			resp := &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"credits exhausted","code":"quota_exceeded"}}`)),
			}
			usage, apiErr := tc.hand(c, info, resp)
			require.Nil(t, usage)
			require.NotNil(t, apiErr)
			assert.Equal(t, types.ErrorClassUpstreamQuotaExhausted, apiErr.GetErrorClass())
			assert.Equal(t, types.UpstreamQuotaPublicMessage, apiErr.ToOpenAIError().Message)
		})
	}
}
