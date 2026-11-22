package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func qixiangCheckoutGateway() *service.QixiangGateway {
	if !isPaymentComplianceConfirmed() || !common.GetEnvOrDefaultBool("QIXIANG_PAY_ENABLED", false) {
		return nil
	}
	return service.ConfiguredQixiangGateway()
}

func RequestQixiangPay(c *gin.Context) {
	gateway := qixiangCheckoutGateway()
	if gateway == nil {
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}

	var req EpayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PaymentMethod != model.PaymentMethodQixiangWeChat {
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	if req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("充值数量不能小于 %d", getMinTopup()))
		return
	}
	userID := c.GetInt("id")
	quota, err := validateTopUpQuota(req.Amount)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.ValidateTopUpQuotaCapacity(userID, quota); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if math.IsInf(payMoney, 0) || math.IsNaN(payMoney) || payMoney < 0.01 || payMoney >= 1e14 {
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	payAmount, err := decimal.NewFromString(strconv.FormatFloat(payMoney, 'f', 2, 64))
	if err != nil {
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	notifyURL := strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/qixiang/notify"
	returnURL := paymentReturnPath("/api/qixiang/return")
	for _, raw := range []string{notifyURL, returnURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
			common.ApiErrorMsg(c, "Payment request failed")
			return
		}
	}

	orderID, err := uuid.NewRandom()
	if err != nil {
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = decimal.NewFromInt(amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	topUp := &model.TopUp{
		UserId: userID, Amount: amount, Money: payAmount.InexactFloat64(),
		TradeNo:       "QX" + strings.ReplaceAll(orderID.String(), "-", ""),
		PaymentMethod: model.PaymentMethodQixiangWeChat, PaymentProvider: model.PaymentProviderQixiang,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("qixiang local order creation failed user_id=%d", userID))
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	returnURL += "?order=" + url.QueryEscape(topUp.TradeNo)
	payURL, err := gateway.CreateWeChatOrder(c.Request.Context(), topUp.TradeNo, payAmount.StringFixed(2), notifyURL, returnURL)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("qixiang checkout failed order=%s: %v", topUp.TradeNo, err))
		common.ApiErrorMsg(c, "Payment request failed")
		return
	}
	common.ApiSuccess(c, gin.H{"pay_url": payURL, "trade_no": topUp.TradeNo})
}

func QixiangNotify(c *gin.Context) {
	gateway := service.ConfiguredQixiangGateway()
	if gateway == nil || len(c.Request.URL.RawQuery) > 4096 {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	query, err := url.ParseQuery(c.Request.URL.RawQuery)
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	notice, err := gateway.VerifyWeChatNotice(query)
	if err != nil {
		c.String(http.StatusBadRequest, "fail")
		return
	}
	LockOrder(notice.TradeNo)
	defer UnlockOrder(notice.TradeNo)
	if err := settleQixiangNotice(c, gateway, notice); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("qixiang payment confirmation failed order=%s: %v", notice.TradeNo, err))
		c.String(http.StatusOK, "fail")
		return
	}
	c.String(http.StatusOK, "success")
}

func QixiangReturn(c *gin.Context) {
	c.Header("Referrer-Policy", "no-referrer")
	target := "/wallet?pay=pending&show_history=true"
	if order := c.Query("order"); len(order) == 34 && strings.HasPrefix(order, "QX") {
		if _, err := uuid.Parse(order[2:]); err == nil {
			target += "&order=" + url.QueryEscape(order)
		}
	}
	c.Redirect(http.StatusSeeOther, paymentReturnPath(target))
}

type qixiangPaymentConfirmer interface {
	ConfirmWeChatPayment(ctx context.Context, notice service.QixiangPaymentNotice) (string, error)
}

func settleQixiangNotice(c *gin.Context, gateway qixiangPaymentConfirmer, notice service.QixiangPaymentNotice) error {
	order := model.GetTopUpByTradeNo(notice.TradeNo)
	if order == nil {
		return model.ErrTopUpNotFound
	}
	if order.PaymentProvider != model.PaymentProviderQixiang || order.PaymentMethod != model.PaymentMethodQixiangWeChat {
		return model.ErrPaymentMethodMismatch
	}
	noticeAmount, err := decimal.NewFromString(notice.Money)
	if err != nil || !noticeAmount.IsPositive() || !noticeAmount.Equal(decimal.NewFromFloat(order.Money).Round(2)) {
		return model.ErrPaymentAmountMismatch
	}
	if order.Status == common.TopUpStatusSuccess {
		return nil
	}
	if order.Status != common.TopUpStatusPending {
		return model.ErrTopUpStatusInvalid
	}
	confirmedMoney, err := gateway.ConfirmWeChatPayment(c.Request.Context(), notice)
	if err != nil {
		return err
	}
	confirmedAmount, err := decimal.NewFromString(confirmedMoney)
	if err != nil || !confirmedAmount.Equal(noticeAmount) {
		return errors.New("qixiang confirmed a different payment amount")
	}
	return model.CompleteQixiangTopUp(order.TradeNo, confirmedMoney, c.ClientIP())
}
