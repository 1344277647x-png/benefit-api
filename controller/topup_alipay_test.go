package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configureAlipayForControllerTest(t *testing.T) {
	t.Helper()
	confirmPaymentComplianceForTest(t)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	publicKey, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	require.NoError(t, err)

	originalEnabled := setting.AlipayEnabled
	originalAppId := setting.AlipayAppId
	originalPrivateKey := setting.AlipayPrivateKey
	originalPublicKey := setting.AlipayPublicKey
	t.Cleanup(func() {
		setting.AlipayEnabled = originalEnabled
		setting.AlipayAppId = originalAppId
		setting.AlipayPrivateKey = originalPrivateKey
		setting.AlipayPublicKey = originalPublicKey
	})

	setting.AlipayEnabled = true
	setting.AlipayAppId = "2026000000000000"
	setting.AlipayPrivateKey = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}))
	setting.AlipayPublicKey = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKey}))
}

func TestAlipayNotifyRejectsInvalidSignature(t *testing.T) {
	configureAlipayForControllerTest(t)
	gin.SetMode(gin.TestMode)

	form := url.Values{
		"app_id":       {setting.AlipayAppId},
		"out_trade_no": {"forged-order"},
		"total_amount": {"9.99"},
		"trade_status": {"TRADE_SUCCESS"},
		"sign_type":    {"RSA2"},
		"sign":         {"not-a-valid-signature"},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/alipay/notify", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request

	AlipayNotify(context)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "fail", recorder.Body.String())
}
