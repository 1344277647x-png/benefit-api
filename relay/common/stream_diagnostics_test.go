package common

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestStreamDiagnosticsSeparatesProtocolTextAndWriteTiming(t *testing.T) {
	start := time.Unix(1000, 0)
	d := NewStreamDiagnostics(start)
	d.Event(start.Add(time.Second))
	d.Event(start.Add(3 * time.Second))
	// Heartbeat/protocol writes are timed but do not count as visible text.
	d.Write(start.Add(time.Second), start.Add(2*time.Second), false)
	assert.NotContains(t, d.Snapshot(), "first_visible_text_ms")
	d.Write(start.Add(3*time.Second), start.Add(4*time.Second), true)
	d.Write(start.Add(8*time.Second), start.Add(10*time.Second), true)
	d.Completed(start.Add(11 * time.Second))
	d.Finish(start.Add(12 * time.Second))
	assert.Equal(t, map[string]interface{}{
		"first_protocol_event_ms": int64(1000), "first_visible_text_ms": int64(4000),
		"max_upstream_event_gap_ms": int64(2000), "max_text_gap_ms": int64(6000),
		"max_downstream_write_ms": int64(2000), "completed_to_return_ms": int64(1000),
	}, d.Snapshot())
}
