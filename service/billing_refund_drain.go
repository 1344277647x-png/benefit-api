package service

import (
	"context"
	"sync"
)

var refundDrain struct {
	sync.Mutex
	pending int
	done    chan struct{}
}

func beginBillingRefund() {
	refundDrain.Lock()
	defer refundDrain.Unlock()
	if refundDrain.pending == 0 {
		refundDrain.done = make(chan struct{})
	}
	refundDrain.pending++
}

func finishBillingRefund() {
	refundDrain.Lock()
	defer refundDrain.Unlock()
	refundDrain.pending--
	if refundDrain.pending == 0 {
		close(refundDrain.done)
	}
}

// Call after HTTP shutdown has drained request handlers. This waits for their
// already scheduled billing-session refunds before flushing database deltas.
func WaitForBillingRefunds(ctx context.Context) error {
	refundDrain.Lock()
	if refundDrain.pending == 0 {
		refundDrain.Unlock()
		return nil
	}
	done := refundDrain.done
	refundDrain.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
