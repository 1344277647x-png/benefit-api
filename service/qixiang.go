package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
)

const DefaultQixiangOrigin = "https://api.payqixiang.cn"

var qixiangMerchantID = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var qixiangOrderID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var qixiangMoney = regexp.MustCompile(`^[0-9]{1,14}(\.[0-9]{1,2})?$`)

type QixiangGateway struct {
	origin string
	pid    string
	key    string
	client *http.Client
}

var qixiangHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func ConfiguredQixiangGateway() *QixiangGateway {
	return newQixiangGateway(
		common.GetEnvOrDefaultString("QIXIANG_PAY_ORIGIN", DefaultQixiangOrigin),
		common.GetEnvOrDefaultString("QIXIANG_PAY_PID", ""),
		common.GetEnvOrDefaultString("QIXIANG_PAY_KEY", ""),
		qixiangHTTPClient,
	)
}

func newQixiangGateway(origin, pid, key string, client *http.Client) *QixiangGateway {
	u, err := url.Parse(strings.TrimSpace(origin))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		!qixiangMerchantID.MatchString(pid) || strings.TrimSpace(key) == "" {
		return nil
	}
	return &QixiangGateway{origin: "https://" + u.Host, pid: pid, key: key, client: client}
}

func (g *QixiangGateway) post(ctx context.Context, endpoint string, form url.Values, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.origin+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return errors.New("qixiang request initialization failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := g.client.Do(req)
	if err != nil {
		return errors.New("qixiang request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return errors.New("qixiang returned an unsuccessful HTTP status")
	}
	const maxResponse = 64 << 10
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil || len(body) > maxResponse || common.Unmarshal(body, result) != nil {
		return errors.New("qixiang returned an invalid response")
	}
	return nil
}

func (g *QixiangGateway) CreateWeChatOrder(ctx context.Context, tradeNo, money, notifyURL, returnURL string) (string, error) {
	form := url.Values{}
	for key, value := range epay.GenerateParams(map[string]string{
		"pid": g.pid, "type": "wxpay", "out_trade_no": tradeNo,
		"money": money, "name": "Benefit API 余额充值", "device": "jump", "clientip": "127.0.0.1",
		"notify_url": notifyURL, "return_url": returnURL,
	}, g.key) {
		form.Set(key, value)
	}
	var result struct {
		Code    json.Number `json:"code"`
		TradeNo string      `json:"trade_no"`
		PayURL  string      `json:"payurl"`
	}
	if err := g.post(ctx, "/mapi.php", form, &result); err != nil {
		return "", err
	}
	payURL, err := url.Parse(result.PayURL)
	if result.Code != "1" || !qixiangOrderID.MatchString(result.TradeNo) || err != nil ||
		payURL.User != nil || payURL.Scheme != "https" ||
		!strings.EqualFold("https://"+payURL.Host, g.origin) {
		return "", errors.New("qixiang did not return a valid checkout")
	}
	return payURL.String(), nil
}

type QixiangPaymentNotice struct {
	TradeNo         string
	ProviderTradeNo string
	Money           string
}

func (g *QixiangGateway) VerifyWeChatNotice(query url.Values) (QixiangPaymentNotice, error) {
	invalid := errors.New("invalid qixiang payment notice")
	params := make(map[string]string, len(query))
	for key, values := range query {
		switch key {
		case "pid", "trade_no", "out_trade_no", "type", "name", "money", "trade_status", "sign", "sign_type", "param":
		default:
			return QixiangPaymentNotice{}, invalid
		}
		if len(values) != 1 || strings.ContainsAny(values[0], "&\r\n\x00") {
			return QixiangPaymentNotice{}, invalid
		}
		params[key] = values[0]
	}
	if params["pid"] != g.pid || params["type"] != "wxpay" || params["trade_status"] != epay.StatusTradeSuccess ||
		params["name"] == "" || params["sign_type"] != "MD5" ||
		!qixiangOrderID.MatchString(params["trade_no"]) || !qixiangOrderID.MatchString(params["out_trade_no"]) ||
		!qixiangMoney.MatchString(params["money"]) {
		return QixiangPaymentNotice{}, invalid
	}
	sign := params["sign"]
	if subtle.ConstantTimeCompare([]byte(sign), []byte(epay.GenerateParams(params, g.key)["sign"])) != 1 {
		return QixiangPaymentNotice{}, invalid
	}
	return QixiangPaymentNotice{
		TradeNo: params["out_trade_no"], ProviderTradeNo: params["trade_no"], Money: params["money"],
	}, nil
}

func (g *QixiangGateway) ConfirmWeChatPayment(ctx context.Context, notice QixiangPaymentNotice) (string, error) {
	order, err := g.QueryPaidWeChatOrder(ctx, notice.TradeNo)
	if err != nil {
		return "", err
	}
	if order.ProviderTradeNo != notice.ProviderTradeNo {
		return "", errors.New("qixiang confirmed a different provider order")
	}
	return order.Money, nil
}

func (g *QixiangGateway) QueryPaidWeChatOrder(ctx context.Context, tradeNo string) (QixiangPaymentNotice, error) {
	if !qixiangOrderID.MatchString(tradeNo) {
		return QixiangPaymentNotice{}, errors.New("invalid qixiang order number")
	}
	var result struct {
		Code       json.Number `json:"code"`
		Status     json.Number `json:"status"`
		PID        json.Number `json:"pid"`
		TradeNo    string      `json:"trade_no"`
		OutTradeNo string      `json:"out_trade_no"`
		Type       string      `json:"type"`
		Money      string      `json:"money"`
	}
	form := url.Values{"pid": {g.pid}, "key": {g.key}, "out_trade_no": {tradeNo}}
	if err := g.post(ctx, "/api.php?act=order", form, &result); err != nil {
		return QixiangPaymentNotice{}, err
	}
	if result.Code != "1" || result.Status != "1" || string(result.PID) != g.pid || result.Type != "wxpay" ||
		result.OutTradeNo != tradeNo || !qixiangOrderID.MatchString(result.TradeNo) ||
		!qixiangMoney.MatchString(result.Money) {
		return QixiangPaymentNotice{}, errors.New("qixiang did not confirm the payment")
	}
	return QixiangPaymentNotice{TradeNo: tradeNo, ProviderTradeNo: result.TradeNo, Money: result.Money}, nil
}
