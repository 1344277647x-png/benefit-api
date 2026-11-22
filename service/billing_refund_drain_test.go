package service

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type blockedRefundFunding struct {
	WalletFunding
	entered chan struct{}
	release chan struct{}
}

func (f *blockedRefundFunding) Refund() error {
	close(f.entered)
	<-f.release
	return nil
}

func TestShutdownDrainWaitsForScheduledBillingRefund(t *testing.T) {
	funding := &blockedRefundFunding{entered: make(chan struct{}), release: make(chan struct{})}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	session := &BillingSession{relayInfo: &relaycommon.RelayInfo{IsPlayground: true}, funding: funding, tokenConsumed: 1}
	session.Refund(c)
	select {
	case <-funding.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("refund not scheduled")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, WaitForBillingRefunds(ctx), context.Canceled)
	close(funding.release)
	ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, WaitForBillingRefunds(ctx))
	session.Refund(c) // duplicate invocation must not schedule a second refund
	require.NoError(t, WaitForBillingRefunds(ctx))
}
