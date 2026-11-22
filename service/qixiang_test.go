package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQixiangCheckoutAndOrderQuery(t *testing.T) {
	const pid, key = "1234", "local-test-key"
	mux := http.NewServeMux()
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/mapi.php", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, "wxpay", r.PostForm.Get("type"))
		assert.Equal(t, "jump", r.PostForm.Get("device"))
		assert.Equal(t, "7.30", r.PostForm.Get("money"))
		assert.Equal(t, "QXtest", r.PostForm.Get("out_trade_no"))
		assert.Empty(t, r.PostForm.Get("key"))
		params := map[string]string{}
		for name := range r.PostForm {
			params[name] = r.PostForm.Get(name)
		}
		signature := params["sign"]
		assert.Equal(t, epay.GenerateParams(params, key)["sign"], signature)
		fmt.Fprintf(w, `{"code":1,"trade_no":"12345678901234567890","payurl":%q}`, server.URL+"/pay/submit/test/")
	})
	mux.HandleFunc("/api.php", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "act=order", r.URL.RawQuery)
		assert.NoError(t, r.ParseForm())
		assert.Equal(t, pid, r.PostForm.Get("pid"))
		assert.Equal(t, key, r.PostForm.Get("key"))
		assert.Equal(t, "QXtest", r.PostForm.Get("out_trade_no"))
		_, _ = fmt.Fprint(w, `{"code":1,"status":"1","pid":1234,"type":"wxpay","trade_no":"12345678901234567890","out_trade_no":"QXtest","money":"7.30"}`)
	})
	gateway := newQixiangGateway(server.URL, pid, key, server.Client())
	require.NotNil(t, gateway)
	payURL, err := gateway.CreateWeChatOrder(context.Background(), "QXtest", "7.30", "https://example.test/notify", "https://example.test/return")
	require.NoError(t, err)
	assert.Equal(t, server.URL+"/pay/submit/test/", payURL)
	form := url.Values{}
	for name, value := range epay.GenerateParams(map[string]string{
		"pid": pid, "type": "wxpay", "out_trade_no": "QXtest", "trade_no": "12345678901234567890",
		"money": "7.30", "name": "Benefit API 余额充值", "trade_status": "TRADE_SUCCESS",
	}, key) {
		form.Set(name, value)
	}
	notice, err := gateway.VerifyWeChatNotice(form)
	require.NoError(t, err)
	money, err := gateway.ConfirmWeChatPayment(context.Background(), notice)
	require.NoError(t, err)
	assert.Equal(t, "7.30", money)
	form.Set("money", "7.31")
	_, err = gateway.VerifyWeChatNotice(form)
	assert.Error(t, err)
	form.Set("money", "7.30")
	form.Add("money", "7.30")
	_, err = gateway.VerifyWeChatNotice(form)
	assert.Error(t, err)
	form.Set("money", "7.30")
	form.Set("unexpected", "field")
	_, err = gateway.VerifyWeChatNotice(form)
	assert.Error(t, err)
}
