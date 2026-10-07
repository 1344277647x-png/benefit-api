package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesCompletionClosesOpenUpstream(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, event := range []string{"response.completed", "response.done"} {
		t.Run(event, func(t *testing.T) {
			r, w := io.Pipe()
			t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
			done := make(chan struct{})
			go func() {
				defer close(done)
				usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: r})
				assert.Nil(t, err)
				if assert.NotNil(t, usage) {
					assert.Equal(t, 12, usage.PromptTokens)
					assert.Equal(t, 3, usage.CompletionTokens)
				}
			}()
			_, err := io.WriteString(w, ": heartbeat\n\n: heartbeat\n\ndata: {\"type\":\""+event+"\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":12,\"output_tokens\":3,\"total_tokens\":15}}}\n\n")
			require.NoError(t, err)
			// Do not close the upstream writer. The terminal event must end the handler.
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("handler waited for upstream EOF")
			}
			assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
			assert.Equal(t, 1, info.ReceivedResponseCount)
			value, ok := c.Get(relaycommon.StreamDiagnosticsContextKey)
			require.True(t, ok)
			diagnostics := value.(*relaycommon.StreamDiagnostics).Snapshot()
			assert.NotContains(t, diagnostics, "first_visible_text_ms", "heartbeats must not count as visible text")
			assert.Contains(t, diagnostics, "completed_to_return_ms")
			_, err = io.WriteString(w, "data: ignored\n\n")
			assert.Error(t, err)
		})
	}
}

func TestResponsesCompletionRejectsNonCompletedStatus(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	for _, status := range []string{"failed", "incomplete", "cancelled", "in_progress"} {
		t.Run(status, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{DisablePing: true, ChannelMeta: &relaycommon.ChannelMeta{}}
			usage, err := OaiResponsesStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"" + status + "\"}}\n\n"))})
			assert.Nil(t, usage)
			require.NotNil(t, err)
			assert.True(t, info.StreamStatus.HasErrors())
		})
	}
}
