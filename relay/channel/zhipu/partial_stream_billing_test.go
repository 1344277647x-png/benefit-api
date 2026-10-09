package zhipu

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

func TestMissingMetaFailsAndRetainsDeliveredText(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, OriginModelName: "chatglm", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "chatglm"}}
	usage, err := zhipuStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader("data:hello world\n"))})
	require.NotNil(t, err)
	assert.Nil(t, usage)
	require.NotNil(t, info.PartialStreamUsage)
	assert.Positive(t, info.PartialStreamUsage.CompletionTokens)
	assert.NotContains(t, recorder.Body.String(), "[DONE]")
}
