package xunfei

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestXunfeiCancellationClosesUpstreamWithoutBlockedProducer(t *testing.T) {
	closed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		defer close(closed)
		_, _, _ = conn.ReadMessage()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"payload":{"choices":{"status":1,"text":[{"content":"partial"}]}}}`))
		_, _, _ = conn.ReadMessage()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	data, _, err := xunfeiMakeRequest(ctx, dto.GeneralOpenAIRequest{}, "test", "ws"+strings.TrimPrefix(server.URL, "http"), "app")
	require.NoError(t, err)
	select {
	case response := <-data:
		require.Equal(t, "partial", response.Payload.Choices.Text[0].Content)
	case <-time.After(3 * time.Second):
		t.Fatal("missing upstream partial output")
	}
	cancel()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation must close the upstream websocket")
	}
}
