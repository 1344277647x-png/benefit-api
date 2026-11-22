package helper

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type controlledFlushWriter struct {
	gin.ResponseWriter
	entered, release chan struct{}
}

func (w *controlledFlushWriter) Flush() {
	close(w.entered)
	<-w.release
	w.ResponseWriter.Flush()
}

type failingResponsesWriter struct{ gin.ResponseWriter }

func (w *failingResponsesWriter) Write([]byte) (int, error) {
	return 0, errors.New("downstream write failed")
}

func TestResponsesWriteTracksFlushAndCancellation(t *testing.T) {
	for _, cancelClient := range []bool{false, true} {
		t.Run(map[bool]string{false: "slow-downstream", true: "cancel-during-flush"}[cancelClient], func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
			d := relaycommon.NewStreamDiagnostics(time.Now())
			c.Set(relaycommon.StreamDiagnosticsContextKey, d)
			writer := &controlledFlushWriter{ResponseWriter: c.Writer, entered: make(chan struct{}), release: make(chan struct{})}
			c.Writer = writer
			done := make(chan error, 1)
			go func() {
				done <- ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: "text"}, `{"type":"response.output_text.delta","delta":"text"}`)
			}()
			select {
			case <-writer.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("did not reach flush")
			}
			// A text event is not visible until the blocked Flush finishes.
			assert.NotContains(t, d.Snapshot(), "first_visible_text_ms")
			if cancelClient {
				cancel()
			}
			close(writer.release)
			select {
			case err := <-done:
				if cancelClient {
					require.Error(t, err)
					assert.NotContains(t, d.Snapshot(), "first_visible_text_ms")
				} else {
					require.NoError(t, err)
					assert.Contains(t, d.Snapshot(), "first_visible_text_ms")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("write did not finish")
			}
			assert.Contains(t, d.Snapshot(), "max_downstream_write_ms")
		})
	}
}

func TestResponsesWriteDoesNotHideDownstreamFailure(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer = &failingResponsesWriter{ResponseWriter: c.Writer}
	err := ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "response.completed"}, `{"type":"response.completed"}`)
	require.Error(t, err)
	assert.False(t, c.GetBool("relay_stream_response_written"))
}
