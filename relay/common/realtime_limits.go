package common

import (
	"os"
	"strconv"
)

// These are gateway compatibility defaults, not provider-advertised limits.
// Bound complete messages (including fragments) separately in each direction.
const RealtimeMaxMessageBytes int64 = 16 << 20

func RealtimeClientMessageLimit() int64 {
	return realtimeMessageLimit("REALTIME_CLIENT_MAX_MESSAGE_BYTES", 16<<20)
}
func RealtimeUpstreamMessageLimit() int64 {
	return realtimeMessageLimit("REALTIME_UPSTREAM_MAX_MESSAGE_BYTES", 32<<20)
}

func realtimeMessageLimit(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil || value < 64<<10 || value > 64<<20 {
		return fallback
	}
	return value
}
