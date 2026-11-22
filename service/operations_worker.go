package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"time"
)

// Restart recovery needs no in-memory queue. Multiple nodes serialize individual
// event receipts in the database; a reporting retry never calls billing code.
func StartOperationsProjection() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runOperationsProjection()
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		runOperationsHourlyRollup()
		for range ticker.C {
			runOperationsHourlyRollup()
		}
	}()
}

func runOperationsHourlyRollup() {
	defer func() {
		if recover() != nil {
			common.SysError("operations hourly rollup panic; raw log reporting remains available")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := model.ProcessOperationsHourlyRollups(ctx, 4); err != nil {
		common.SysError("operations hourly rollup deferred; raw log reporting remains available")
	}
}

func runOperationsProjection() {
	defer func() {
		if recover() != nil {
			common.SysError("operations projection panic; pending events retained")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := model.ProcessOperationsEvents(ctx, 200); err != nil {
		common.SysError("operations projection deferred; inspect pending accounting events")
	}
}
