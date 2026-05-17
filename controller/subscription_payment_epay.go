package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type SubscriptionEpayPayRequest struct {
	PlanId        int    `json:"plan_id"`
	PaymentMethod string `json:"payment_method"`
}

func SubscriptionRequestEpay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionEpayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	config := getGatewayConfig(req.PaymentMethod)
	if !containsGatewayMethod(config, req.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}

	userId := c.GetInt("id")
	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/return")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}
	notifyUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/notify")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("%s%dNO%s", config.SubscriptionTag, userId, tradeNo)

	order := &model.SubscriptionOrder{
		UserId:          userId,
		PlanId:          plan.Id,
		Money:           plan.PriceAmount,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: config.Provider,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	var uri string
	var params map[string]string
	if config.IsZPay {
		client := getZPayClient(config)
		if client == nil {
			common.ApiErrorMsg(c, "当前管理员未配置 Z Pay 支付信息")
			return
		}
		uri, params, err = client.Purchase(&service.ZPayPurchaseArgs{
			Type:           normalizeGatewayMethod(req.PaymentMethod),
			ServiceTradeNo: tradeNo,
			Name:           fmt.Sprintf("SUB:%s", plan.Title),
			Money:          strconv.FormatFloat(plan.PriceAmount, 'f', 2, 64),
			NotifyURL:      notifyUrl.String(),
			ReturnURL:      returnUrl.String(),
			ClientIP:       c.ClientIP(),
			Device:         service.ZPayDeviceFromUserAgent(c.Request.UserAgent()),
			ChannelID:      config.ChannelID,
		})
	} else {
		client := getGatewayClient(config)
		if client == nil {
			common.ApiErrorMsg(c, "当前管理员未配置支付信息")
			return
		}
		uri, params, err = client.Purchase(&epay.PurchaseArgs{
			Type:           normalizeGatewayMethod(req.PaymentMethod),
			ServiceTradeNo: tradeNo,
			Name:           fmt.Sprintf("SUB:%s", plan.Title),
			Money:          strconv.FormatFloat(plan.PriceAmount, 'f', 2, 64),
			Device:         epay.PC,
			NotifyUrl:      notifyUrl,
			ReturnUrl:      returnUrl,
		})
	}
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, config.Provider)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func SubscriptionEpayNotify(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}

	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	tradeNo := params["out_trade_no"]
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	if order == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	config := getGatewayConfig(order.PaymentMethod)
	var verifyTradeNo string
	var verifyType string
	var verifyStatus string
	var verifyOK bool
	var verifyErr error
	if config.IsZPay {
		client := getZPayClient(config)
		if client == nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		verifyInfo, err := client.Verify(params)
		verifyErr = err
		if verifyErr == nil {
			verifyTradeNo = verifyInfo.ServiceTradeNo
			verifyType = "zpay_" + verifyInfo.Type
			verifyStatus = verifyInfo.TradeStatus
			verifyOK = verifyInfo.VerifyStatus
		}
	} else {
		client := getGatewayClient(config)
		if client == nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		verifyInfo, err := client.Verify(params)
		verifyErr = err
		if verifyErr == nil {
			verifyTradeNo = verifyInfo.ServiceTradeNo
			verifyType = verifyInfo.Type
			verifyStatus = verifyInfo.TradeStatus
			verifyOK = verifyInfo.VerifyStatus
		}
	}
	if verifyErr != nil || !verifyOK {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyStatus != epay.StatusTradeSuccess && verifyStatus != "TRADE_SUCCESS" {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	LockOrder(verifyTradeNo)
	defer UnlockOrder(verifyTradeNo)

	if config.IsZPay && params["money"] != strconv.FormatFloat(order.Money, 'f', 2, 64) {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if err := model.CompleteSubscriptionOrder(verifyTradeNo, common.GetJsonString(params), config.Provider, verifyType); err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))
}

// SubscriptionEpayReturn handles browser return after payment.
// It verifies the payload and completes the order, then redirects to console.
func SubscriptionEpayReturn(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}

	if len(params) == 0 {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}

	tradeNo := params["out_trade_no"]
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	if order == nil {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	config := getGatewayConfig(order.PaymentMethod)
	var verifyTradeNo string
	var verifyType string
	var verifyStatus string
	var verifyOK bool
	var verifyErr error
	if config.IsZPay {
		client := getZPayClient(config)
		if client == nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		verifyInfo, err := client.Verify(params)
		verifyErr = err
		if verifyErr == nil {
			verifyTradeNo = verifyInfo.ServiceTradeNo
			verifyType = "zpay_" + verifyInfo.Type
			verifyStatus = verifyInfo.TradeStatus
			verifyOK = verifyInfo.VerifyStatus
		}
	} else {
		client := getGatewayClient(config)
		if client == nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		verifyInfo, err := client.Verify(params)
		verifyErr = err
		if verifyErr == nil {
			verifyTradeNo = verifyInfo.ServiceTradeNo
			verifyType = verifyInfo.Type
			verifyStatus = verifyInfo.TradeStatus
			verifyOK = verifyInfo.VerifyStatus
		}
	}
	if verifyErr != nil || !verifyOK {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	if verifyStatus == epay.StatusTradeSuccess || verifyStatus == "TRADE_SUCCESS" {
		LockOrder(verifyTradeNo)
		defer UnlockOrder(verifyTradeNo)
		if config.IsZPay && params["money"] != strconv.FormatFloat(order.Money, 'f', 2, 64) {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		if err := model.CompleteSubscriptionOrder(verifyTradeNo, common.GetJsonString(params), config.Provider, verifyType); err != nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=success"))
		return
	}
	c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=pending"))
}
