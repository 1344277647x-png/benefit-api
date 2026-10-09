package tencent

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

func TestTencentTruncatedStreamRetainsUsage(t *testing.T) {
	for _, withUsage := range []bool{false, true} {
		t.Run(map[bool]string{false: "estimated", true: "upstream"}[withUsage], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "hunyuan"}}
			info.SetEstimatePromptTokens(20)
			usage := ""
			if withUsage {
				usage = `,"Usage":{"PromptTokens":20,"CompletionTokens":8,"TotalTokens":28}`
			}
			_, apiErr := tencentStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
				`data: {"Choices":[{"Delta":{"Content":"Hello world"}}]` + usage + "}\n\n"))})
			require.NotNil(t, apiErr)
			require.NotNil(t, info.PartialStreamUsage)
			assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
			if withUsage {
				assert.Equal(t, 8, info.PartialStreamUsage.CompletionTokens)
			}
		})
	}
}
