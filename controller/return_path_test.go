package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestEpayReturnOnlyForwardsLocalWalletOrder(t *testing.T) {
	const order = "USR42NOAbC1231795310000"
	for _, tc := range []struct{ query, suffix string }{
		{"?order=" + order + "&trade_status=success&sign=fixture", "&order=" + order},
		{"?order=bad&out_trade_no=" + order, ""},
		{"", ""},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/user/epay/return"+tc.query, nil)
		EpayReturn(c)
		assert.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, paymentReturnPath("/wallet?pay=pending&show_history=true"+tc.suffix), w.Header().Get("Location"))
	}
}

func TestPaymentReturnPathUsesDefaultDashboardRoutes(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://dashboard.example.com/"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	assert.Equal(
		t,
		"https://dashboard.example.com/wallet?pay=success",
		paymentReturnPath("/wallet?pay=success"),
	)
	assert.Equal(
		t,
		"https://dashboard.example.com/usage-logs",
		paymentReturnPath("/usage-logs"),
	)
}
