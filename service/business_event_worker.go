package service

import (
	"context"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

var businessEventWorkerOnce sync.Once
var businessEventWake = make(chan struct{}, 1)

// A bounded notifier keeps global log delivery outside user request latency.
// The durable outbox remains the source of truth if the process exits.
func NotifyBusinessEventDelivery() {
	select {
	case businessEventWake <- struct{}{}:
	default:
	}
}

func StartBusinessEventDelivery() {
	businessEventWorkerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-businessEventWake:
				case <-ticker.C:
				}
				deliverBusinessEventBatch()
			}
		}()
	})
	NotifyBusinessEventDelivery()
}

func deliverBusinessEventBatch() {
	defer func() {
		if recover() != nil {
			common.SysError("business event delivery worker recovered from panic")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := DeliverPendingBusinessEvents(ctx, 200); err != nil {
		common.SysError("business event delivery failed; inspect pending and dead-letter events")
	}
}
