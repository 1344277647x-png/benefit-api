package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

func TeamEpayPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var request SubscriptionEpayPayRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.PlanId <= 0 || !operation_setting.ContainsPayMethod(request.PaymentMethod) {
		common.ApiErrorMsg(c, "支付参数无效")
		return
	}
	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置易支付")
		return
	}
	returnURL, notifyURL, err := teamEpayCallbackURLs()
	if err != nil {
		common.ApiErrorMsg(c, "回调地址无效")
		return
	}
	order, err := model.CreateTeamPaymentOrder(c.GetInt("id"), request.PlanId, model.PaymentProviderEpay, request.PaymentMethod)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	_ = service.DeliverPendingBusinessEvents(c.Request.Context(), 100)
	teamEpayPaymentForm(c, client, order, returnURL, notifyURL, true)
}

func TeamEpayResume(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置易支付")
		return
	}
	order, err := model.GetPendingTeamPayment(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if order == nil {
		common.ApiError(c, model.ErrNoPendingTeamPayment)
		return
	}
	if !operation_setting.ContainsPayMethod(order.PaymentMethod) {
		common.ApiErrorMsg(c, "原订单的易支付方式已停用，请联系管理员核对支付状态")
		return
	}
	returnURL, notifyURL, err := teamEpayCallbackURLs()
	if err != nil {
		common.ApiErrorMsg(c, "回调地址无效")
		return
	}
	teamEpayPaymentForm(c, client, order, returnURL, notifyURL, false)
}

func teamEpayCallbackURLs() (*url.URL, *url.URL, error) {
	callback := service.GetCallbackAddress()
	returnURL, err := url.Parse(callback + "/api/team/epay/return")
	if err != nil {
		return nil, nil, err
	}
	notifyURL, err := url.Parse(callback + "/api/team/epay/notify")
	if err != nil {
		return nil, nil, err
	}
	return returnURL, notifyURL, nil
}

func teamEpayPaymentForm(c *gin.Context, client *epay.Client, order *model.TeamOrder, returnURL, notifyURL *url.URL, expireOnFailure bool) {
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type: order.PaymentMethod, ServiceTradeNo: order.TradeNo,
		Name: fmt.Sprintf("TEAM:%s", order.PlanTitle), Money: strconv.FormatFloat(float64(order.Money), 'f', 2, 64),
		Device: epay.PC, NotifyUrl: notifyURL, ReturnUrl: returnURL,
	})
	if err != nil {
		if expireOnFailure {
			_ = model.ExpireTeamPayment(order.TradeNo)
		}
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if !expireOnFailure {
		current, err := model.GetPendingTeamPayment(c.GetInt("id"))
		if err != nil || current == nil || current.TradeNo != order.TradeNo {
			common.ApiErrorMsg(c, "订单状态已变化，请刷新团队页面")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func verifiedTeamEpayResult(c *gin.Context) (*epay.VerifyRes, error) {
	if err := c.Request.ParseForm(); err != nil {
		return nil, err
	}
	params := make(map[string]string)
	for key, values := range c.Request.Form {
		if len(values) != 1 {
			return nil, model.ErrPaymentMethodMismatch
		}
		params[key] = values[0]
	}
	client := GetEpayClient()
	if client == nil || len(params) == 0 || params["pid"] != operation_setting.EpayId {
		return nil, model.ErrPaymentMethodMismatch
	}
	result, err := client.Verify(params)
	if err != nil || result == nil || !result.VerifyStatus || result.TradeStatus != epay.StatusTradeSuccess ||
		result.ServiceTradeNo == "" || result.TradeNo == "" {
		return nil, model.ErrPaymentMethodMismatch
	}
	return result, nil
}

func TeamEpayNotify(c *gin.Context) {
	result, err := verifiedTeamEpayResult(c)
	if err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	LockOrder(result.ServiceTradeNo)
	defer UnlockOrder(result.ServiceTradeNo)
	if err := model.CompleteTeamPayment(result.ServiceTradeNo, model.PaymentProviderEpay, result.TradeNo, result.Money, result.Type); err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	_ = service.DeliverPendingBusinessEvents(c.Request.Context(), 100)
	_, _ = c.Writer.Write([]byte("success"))
}

func TeamEpayReturn(c *gin.Context) {
	_, err := verifiedTeamEpayResult(c)
	if err != nil {
		c.Redirect(http.StatusFound, paymentReturnPath("/team?pay=fail"))
		return
	}
	// Browser returns are advisory; only the server-to-server notify may credit.
	c.Redirect(http.StatusFound, paymentReturnPath("/team?pay=pending"))
}
