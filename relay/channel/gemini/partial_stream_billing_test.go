package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiInterruptedTextRetainsUsageButUnpricedImageDoesNot(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, tc := range []struct {
		name     string
		parts    string
		billable bool
	}{
		{"text", `[{"text":"Hello world"}]`, true},
		{"unpriced_image", `[{"inlineData":{"mimeType":"image/png","data":"AA=="}}]`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-2.5-flash"}}
			info.SetEstimatePromptTokens(20)
			_, apiErr := geminiStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader(
				`data: {"candidates":[{"content":{"parts":` + tc.parts + `}}]}` + "\n\n"))},
				func(string, *dto.GeminiChatResponse) bool { return true })
			require.NotNil(t, apiErr)
			if tc.billable {
				require.NotNil(t, info.PartialStreamUsage)
				assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
			} else {
				assert.Nil(t, info.PartialStreamUsage)
			}
		})
	}
}
