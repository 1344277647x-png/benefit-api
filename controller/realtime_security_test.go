package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealtimeOriginPolicy(t *testing.T) {
	previous, urls := system_setting.ServerAddress, common.SessionCookieTrustedURLs
	system_setting.ServerAddress = "https://api.example.com"
	common.SessionCookieTrustedURLs = []string{"https://console.example.com"}
	t.Cleanup(func() { system_setting.ServerAddress, common.SessionCookieTrustedURLs = previous, urls })
	t.Setenv("REALTIME_ALLOWED_ORIGINS", "https://client.example.com:8443")
	for _, tc := range []struct {
		origin string
		want   bool
	}{
		{"", true}, {"https://api.example.com:443", true}, {"https://console.example.com", true},
		{"https://client.example.com:8443", true}, {"https://client.example.com", false},
		{"https://api.example.com.evil.test", false}, {"null", false}, {"https://evil.test", false},
	} {
		r := httptest.NewRequest(http.MethodGet, "https://api.example.com/v1/realtime", nil)
		if tc.origin != "" {
			r.Header.Set("Origin", tc.origin)
		}
		assert.Equal(t, tc.want, realtimeOriginAllowed(r), tc.origin)
	}
}

func TestRealtimeRejectsOversizedFragmentedMessage(t *testing.T) {
	t.Setenv("REALTIME_CLIENT_MAX_MESSAGE_BYTES", "1048576")
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		conn.SetReadLimit(relaycommon.RealtimeClientMessageLimit())
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, _, err = conn.ReadMessage()
		result <- err
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	writer, err := conn.NextWriter(websocket.TextMessage)
	require.NoError(t, err)
	_, _ = writer.Write([]byte(strings.Repeat("x", int(relaycommon.RealtimeClientMessageLimit())+1)))
	_ = writer.Close()
	select {
	case err := <-result:
		require.ErrorIs(t, err, websocket.ErrReadLimit)
	case <-time.After(5 * time.Second):
		t.Fatal("oversized message was not rejected")
	}
}

func TestRealtimeDefaultAcceptsMessageAboveOldOneMiBLimit(t *testing.T) {
	t.Setenv("REALTIME_CLIENT_MAX_MESSAGE_BYTES", "")
	result := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			result <- -1
			return
		}
		defer conn.Close()
		conn.SetReadLimit(relaycommon.RealtimeClientMessageLimit())
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, message, err := conn.ReadMessage()
		if err != nil {
			result <- -1
			return
		}
		result <- len(message)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	defer conn.Close()
	message := []byte(strings.Repeat("x", 2<<20))
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, message))
	select {
	case size := <-result:
		assert.Equal(t, len(message), size)
	case <-time.After(5 * time.Second):
		t.Fatal("message above old limit was not accepted")
	}
}
