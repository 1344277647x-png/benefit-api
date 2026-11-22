package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterruptedResponsesRetainsBillableUsageWithoutReportingSuccess(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello world\"}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Equal(t, 20, info.PartialStreamUsage.PromptTokens)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, "estimated", info.PartialStreamUsageSource)
}

type cancelOnFlushWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
}

func (w *cancelOnFlushWriter) Flush() {
	w.ResponseWriter.Flush()
	w.cancel()
}

func TestResponsesDisconnectDuringTerminalWriteKeepsUpstreamUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
	c.Writer = &cancelOnFlushWriter{ResponseWriter: c.Writer, cancel: cancel}
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	_, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":20,\"output_tokens\":5,\"input_tokens_details\":{\"cached_tokens\":15}}}}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Equal(t, 5, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, 15, info.PartialStreamUsage.PromptTokensDetails.CachedTokens)
	assert.Equal(t, "upstream", info.PartialStreamUsageSource)
}

func TestResponsesProtocolEventsWithoutGenerationDoNotCreateUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	_, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		": PING\n\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"test\"}}\n\n"))})
	require.NotNil(t, apiErr)
	assert.Nil(t, info.PartialStreamUsage)
}

func TestInterruptedResponsesFunctionArgumentsRetainUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"type\":\"response.function_call_arguments.delta\",\"delta\":\"{\\\"query\\\":\\\"weather\\\"}\"}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, "estimated", info.PartialStreamUsageSource)
}

func TestChatInterruptedAfterTextRetainsUsageAndError(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, RelayMode: relayconstant.RelayModeChatCompletions, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := OaiStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":8,\"total_tokens\":28},\"choices\":[]}\n\ndata: {\"error\":{\"message\":\"unavailable\",\"type\":\"server_error\"}}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Equal(t, 8, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, "upstream", info.PartialStreamUsageSource)
}

func TestChatEOFWithoutDoneRetainsEstimatedUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, RelayMode: relayconstant.RelayModeChatCompletions, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := OaiStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, "estimated", info.PartialStreamUsageSource)
}

func TestChatToResponsesTruncationRetainsGeneratedUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true,
		RelayFormat: types.RelayFormatOpenAIResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-4"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := OaiChatToResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello world\"}}]}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
}

func TestImageTruncationPreservesProviderUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _, resp, info := newImageTestContext(t,
		"data: {\"type\":\"image_generation.partial_image\",\"usage\":{\"input_tokens\":20,\"output_tokens\":8}}\n\n",
		"text/event-stream", true)
	_, apiErr := OpenaiImageStreamHandler(c, info, resp)
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Equal(t, 8, info.PartialStreamUsage.CompletionTokens)
}
