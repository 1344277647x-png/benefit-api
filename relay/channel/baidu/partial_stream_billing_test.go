package baidu

import (
	"github.com/QuantumNous/new-api/constant"
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

func TestBaiduPartialOutputRetainsUsageOnFailure(t *testing.T) {
	previous := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previous })
	for _, ending := range []string{"", "data: {\"error_code\":17,\"error_msg\":\"insufficient balance\"}\n\n"} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
		info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "ernie"}}
		info.SetEstimatePromptTokens(20)
		apiErr, _ := baiduStreamHandler(c, info, &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: {\"result\":\"hello world\"}\n\n" + ending))})
		require.NotNil(t, apiErr)
		require.NotNil(t, info.PartialStreamUsage)
		assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
		assert.Equal(t, 20, info.PartialStreamUsage.PromptTokens)
	}
}
