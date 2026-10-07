package common

import (
	"sync"
	"time"
)

const StreamDiagnosticsContextKey = "relay_stream_diagnostics"
const RelaySuccessContextKey = "relay_business_success"

// StreamDiagnostics holds bounded timing data only, never request or response content.
type StreamDiagnostics struct {
	mu                                                                     sync.Mutex
	start, firstEvent, lastEvent, firstText, lastText, completed, finished time.Time
	maxEventGap, maxTextGap, maxWrite                                      time.Duration
}

func NewStreamDiagnostics(start time.Time) *StreamDiagnostics {
	if start.IsZero() {
		start = time.Now()
	}
	return &StreamDiagnostics{start: start}
}

func (d *StreamDiagnostics) Event(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.firstEvent.IsZero() {
		d.firstEvent = now
	}
	if !d.lastEvent.IsZero() && now.Sub(d.lastEvent) > d.maxEventGap {
		d.maxEventGap = now.Sub(d.lastEvent)
	}
	d.lastEvent = now
}

func (d *StreamDiagnostics) Write(start, end time.Time, text bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if end.Sub(start) > d.maxWrite {
		d.maxWrite = end.Sub(start)
	}
	if !text {
		return
	}
	if d.firstText.IsZero() {
		d.firstText = end
	}
	if !d.lastText.IsZero() && end.Sub(d.lastText) > d.maxTextGap {
		d.maxTextGap = end.Sub(d.lastText)
	}
	d.lastText = end
}

func (d *StreamDiagnostics) Completed(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.completed = now
}

func (d *StreamDiagnostics) Finish(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.finished = now
}

func (d *StreamDiagnostics) Snapshot() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()
	m := map[string]interface{}{"max_upstream_event_gap_ms": d.maxEventGap.Milliseconds(), "max_text_gap_ms": d.maxTextGap.Milliseconds(), "max_downstream_write_ms": d.maxWrite.Milliseconds()}
	if !d.firstEvent.IsZero() {
		m["first_protocol_event_ms"] = d.firstEvent.Sub(d.start).Milliseconds()
	}
	if !d.firstText.IsZero() {
		m["first_visible_text_ms"] = d.firstText.Sub(d.start).Milliseconds()
	}
	if !d.completed.IsZero() && !d.finished.IsZero() {
		m["completed_to_return_ms"] = d.finished.Sub(d.completed).Milliseconds()
	}
	return m
}
