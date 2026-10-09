package claude

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeErrorAfterOutputRetainsUsageWithoutSuccessfulTermination(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, RelayFormat: types.RelayFormatClaude,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "claude-sonnet-4"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := ClaudeStreamHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"test\",\"usage\":{\"input_tokens\":20}}}\n\n" +
			"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello world\"}}\n\n" +
			"data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"unavailable\"}}\n\n"))}, info)
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Equal(t, 20, info.PartialStreamUsage.PromptTokens)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, "anthropic", info.PartialStreamUsage.UsageSemantic)
}
