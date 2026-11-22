package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	alipay "github.com/smartwalle/alipay/v3"
)

type AlipayPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

func normalizeAlipayKey(value string) string {
	return strings.TrimSpace(strings.ReplaceAll(value, `\n`, "\n"))
}

func getAlipayClient() (*alipay.Client, error) {
	client, err := alipay.New(
		strings.TrimSpace(setting.AlipayAppId),
		normalizeAlipayKey(setting.AlipayPrivateKey),
		!setting.AlipaySandbox,
	)
	if err != nil {
		return nil, fmt.Errorf("初始化支付宝客户端失败: %w", err)
	}
	if err := client.LoadAliPayPublicKey(normalizeAlipayKey(setting.AlipayPublicKey)); err != nil {
		return nil, fmt.Errorf("加载支付宝公钥失败: %w", err)
	}
	return client, nil
}

func validAlipayCallbackOrigin(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func RequestAlipayPay(c *gin.Context) {
	if !isAlipayTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付宝支付未配置或未启用"})
		return
	}

	var req AlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.PaymentMethod != model.PaymentMethodAlipayNative {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payAmount := decimal.NewFromFloat(getPayMoney(req.Amount, group)).Round(2)
	if payAmount.LessThan(decimal.NewFromFloat(0.01)) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	callbackAddress := strings.TrimRight(service.GetCallbackAddress(), "/")
	returnAddress := paymentReturnPath("/api/alipay/return")
	if !validAlipayCallbackOrigin(callbackAddress) || !validAlipayCallbackOrigin(returnAddress) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请先配置正确的服务器地址和支付回调地址"})
		return
	}

	client, err := getAlipayClient()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝客户端初始化失败 user_id=%d error=%q", userId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付宝密钥配置无效"})
		return
	}

	tradeNo := fmt.Sprintf("ALI%d%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	param := alipay.TradePagePay{}
	param.NotifyURL = callbackAddress + "/api/alipay/notify"
	param.ReturnURL = returnAddress
	param.Subject = fmt.Sprintf("%s 余额充值", common.SystemName)
	param.OutTradeNo = tradeNo
	param.TotalAmount = payAmount.StringFixed(2)
	param.ProductCode = "FAST_INSTANT_TRADE_PAY"
	param.IntegrationType = "PCWEB"

	payURL, err := client.TradePagePay(param)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝生成支付链接失败 user_id=%d trade_no=%s amount=%d error=%q", userId, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付宝失败"})
		return
	}

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = decimal.NewFromInt(amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	topUp := &model.TopUp{
		UserId:          userId,
		Amount:          amount,
		Money:           payAmount.InexactFloat64(),
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodAlipayNative,
		PaymentProvider: model.PaymentProviderAlipay,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝创建充值订单失败 user_id=%d trade_no=%s amount=%d error=%q", userId, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝充值订单创建成功 user_id=%d trade_no=%s amount=%d money=%s sandbox=%t", userId, tradeNo, req.Amount, payAmount.StringFixed(2), setting.AlipaySandbox))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    gin.H{"pay_url": payURL.String()},
	})
}

func AlipayNotify(c *gin.Context) {
	if !isAlipayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝 webhook 被拒绝 reason=webhook_disabled client_ip=%s", c.ClientIP()))
		c.String(http.StatusForbidden, "fail")
		return
	}
	if err := c.Request.ParseForm(); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝 webhook 表单解析失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.String(http.StatusBadRequest, "fail")
		return
	}

	client, err := getAlipayClient()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝 webhook 客户端初始化失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.String(http.StatusInternalServerError, "fail")
		return
	}
	notification, err := client.DecodeNotification(c.Request.Context(), c.Request.PostForm)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝 webhook 验签失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if notification.AppId != strings.TrimSpace(setting.AlipayAppId) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝 webhook AppID 不匹配 trade_no=%s callback_app_id=%s client_ip=%s", notification.OutTradeNo, notification.AppId, c.ClientIP()))
		c.String(http.StatusBadRequest, "fail")
		return
	}
	if notification.TradeStatus != alipay.TradeStatusSuccess && notification.TradeStatus != alipay.TradeStatusFinished {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝 webhook 忽略交易状态 trade_no=%s trade_status=%s client_ip=%s", notification.OutTradeNo, notification.TradeStatus, c.ClientIP()))
		c.String(http.StatusOK, "success")
		return
	}

	if err := model.CompleteAlipayTopUp(notification.OutTradeNo, notification.TotalAmount, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝充值入账失败 trade_no=%s alipay_trade_no=%s total_amount=%s client_ip=%s error=%q", notification.OutTradeNo, notification.TradeNo, notification.TotalAmount, c.ClientIP(), err.Error()))
		c.String(http.StatusInternalServerError, "fail")
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付宝充值通知处理成功 trade_no=%s alipay_trade_no=%s total_amount=%s trade_status=%s client_ip=%s", notification.OutTradeNo, notification.TradeNo, notification.TotalAmount, notification.TradeStatus, c.ClientIP()))
	c.String(http.StatusOK, "success")
}

func AlipayReturn(c *gin.Context) {
	if client, err := getAlipayClient(); err == nil {
		if err := c.Request.ParseForm(); err == nil {
			if err := client.VerifySign(c.Request.Context(), c.Request.Form); err != nil {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("支付宝同步返回验签失败 client_ip=%s error=%q", c.ClientIP(), err.Error()))
			}
		}
	}
	c.Redirect(http.StatusSeeOther, paymentReturnPath("/console/topup"))
}
