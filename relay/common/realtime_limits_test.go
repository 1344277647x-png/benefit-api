package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRealtimeDirectionalLimitConfiguration(t *testing.T) {
	t.Setenv("REALTIME_CLIENT_MAX_MESSAGE_BYTES", "")
	t.Setenv("REALTIME_UPSTREAM_MAX_MESSAGE_BYTES", "")
	assert.EqualValues(t, 16<<20, RealtimeClientMessageLimit())
	assert.EqualValues(t, 32<<20, RealtimeUpstreamMessageLimit())
	t.Setenv("REALTIME_CLIENT_MAX_MESSAGE_BYTES", "2097152")
	assert.EqualValues(t, 2<<20, RealtimeClientMessageLimit())
	assert.EqualValues(t, 32<<20, RealtimeUpstreamMessageLimit())
	for _, invalid := range []string{"0", "-1", "invalid", "67108865"} {
		t.Setenv("REALTIME_CLIENT_MAX_MESSAGE_BYTES", invalid)
		assert.EqualValues(t, 16<<20, RealtimeClientMessageLimit())
	}
}
