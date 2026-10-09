package coze

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCozeTruncatedStreamRetainsTextUsage(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "coze"}}
	info.SetEstimatePromptTokens(20)
	_, apiErr := cozeChatStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
		"event: conversation.message.delta\ndata: {\"content\":\"Hello world\"}\n\n"))})
	require.NotNil(t, apiErr)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
	assert.Equal(t, 20, info.PartialStreamUsage.PromptTokens)
}
