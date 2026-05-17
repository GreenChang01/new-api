package controller

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()

	// 获取支付方式
	payMethods := append([]map[string]string{}, operation_setting.PayMethods...)
	if isZPayTopUpEnabled() {
		payMethods = append(payMethods, operation_setting.ZPayMethods...)
	}
	if !complianceConfirmed {
		payMethods = []map[string]string{}
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoMinTopUp),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":      "Waffo Pancake",
				"type":      model.PaymentMethodWaffoPancake,
				"color":     "rgba(var(--semi-orange-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoPancakeMinTopUp),
			})
		}
	}

	data := gin.H{
		"enable_online_topup":              isEpayTopUpEnabled() || isZPayTopUpEnabled(),
		"enable_zpay_topup":                isZPayTopUpEnabled(),
		"enable_stripe_topup":              isStripeTopUpEnabled(),
		"enable_creem_topup":               isCreemTopUpEnabled(),
		"enable_waffo_topup":               enableWaffo,
		"enable_waffo_pancake_topup":       enableWaffoPancake,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products": setting.CreemProducts,
		"pay_methods":    payMethods,
		"min_topup": func() int {
			if isEpayTopUpEnabled() {
				return operation_setting.MinTopUp
			}
			return operation_setting.ZPayMinTopUp
		}(),
		"stripe_min_topup":        setting.StripeMinTopUp,
		"waffo_min_topup":         setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount,
		"topup_link":              common.TopUpLink,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type AmountRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type epayGatewayConfig struct {
	Address         string
	PartnerID       string
	Key             string
	Price           float64
	MinTopUp        int
	Methods         []map[string]string
	ChannelID       string
	Provider        string
	NotifyPath      string
	SubscriptionTag string
	IsZPay          bool
}

func getGatewayConfig(paymentMethod string) *epayGatewayConfig {
	if strings.HasPrefix(paymentMethod, "zpay_") {
		return &epayGatewayConfig{
			Address:         operation_setting.ZPayAddress,
			PartnerID:       operation_setting.ZPayId,
			Key:             operation_setting.ZPayKey,
			Price:           operation_setting.ZPayPrice,
			MinTopUp:        operation_setting.ZPayMinTopUp,
			Methods:         operation_setting.ZPayMethods,
			ChannelID:       operation_setting.ZPayChannelID,
			Provider:        model.PaymentProviderZPay,
			NotifyPath:      "/api/user/epay/notify",
			SubscriptionTag: "ZPAYSUB",
			IsZPay:          true,
		}
	}
	return &epayGatewayConfig{
		Address:         operation_setting.PayAddress,
		PartnerID:       operation_setting.EpayId,
		Key:             operation_setting.EpayKey,
		Price:           operation_setting.Price,
		MinTopUp:        operation_setting.MinTopUp,
		Methods:         operation_setting.PayMethods,
		Provider:        model.PaymentProviderEpay,
		NotifyPath:      "/api/user/epay/notify",
		SubscriptionTag: "SUBUSR",
	}
}

func GetEpayClient() *epay.Client {
	return getGatewayClient(getGatewayConfig(""))
}

func getGatewayClient(config *epayGatewayConfig) *epay.Client {
	if config == nil || config.Address == "" || config.PartnerID == "" || config.Key == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: config.PartnerID,
		Key:       config.Key,
	}, config.Address)
	if err != nil {
		return nil
	}
	return withUrl
}

func getZPayClient(config *epayGatewayConfig) *service.ZPayClient {
	if config == nil || !config.IsZPay {
		return nil
	}
	return service.NewZPayClient(config.Address, config.PartnerID, config.Key)
}

func getGatewayPayMoney(amount int64, group string, price float64) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getPayMoney(amount int64, group string) float64 {
	return getGatewayPayMoney(amount, group, operation_setting.Price)
}

func getGatewayMinTopup(minTopup int) int64 {
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func getMinTopup() int64 {
	return getGatewayMinTopup(operation_setting.MinTopUp)
}

func normalizeGatewayMethod(paymentMethod string) string {
	switch paymentMethod {
	case "zpay_alipay", "epay_alipay":
		return "alipay"
	case "zpay_wxpay", "epay_wxpay":
		return "wxpay"
	default:
		return paymentMethod
	}
}

func containsGatewayMethod(config *epayGatewayConfig, method string) bool {
	if config == nil {
		return false
	}
	for _, payMethod := range config.Methods {
		if payMethod["type"] == method {
			return true
		}
	}
	return false
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	config := getGatewayConfig(req.PaymentMethod)
	minTopup := getGatewayMinTopup(config.MinTopUp)
	if req.Amount < minTopup {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", minTopup)})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getGatewayPayMoney(req.Amount, group, config.Price)
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !containsGatewayMethod(config, req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/console/log"))
	notifyUrl, _ := url.Parse(callBackAddress + config.NotifyPath)
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	var uri string
	var params map[string]string
	if config.IsZPay {
		client := getZPayClient(config)
		if client == nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置 Z Pay 支付信息"})
			return
		}
		uri, params, err = client.Purchase(&service.ZPayPurchaseArgs{
			Type:           normalizeGatewayMethod(req.PaymentMethod),
			ServiceTradeNo: tradeNo,
			Name:           fmt.Sprintf("TUC%d", req.Amount),
			Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
			NotifyURL:      notifyUrl.String(),
			ReturnURL:      returnUrl.String(),
			ClientIP:       c.ClientIP(),
			Device:         service.ZPayDeviceFromUserAgent(c.Request.UserAgent()),
			ChannelID:      config.ChannelID,
		})
	} else {
		client := getGatewayClient(config)
		if client == nil {
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
			return
		}
		uri, params, err = client.Purchase(&epay.PurchaseArgs{
			Type:           normalizeGatewayMethod(req.PaymentMethod),
			ServiceTradeNo: tradeNo,
			Name:           fmt.Sprintf("TUC%d", req.Amount),
			Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
			Device:         epay.PC,
			NotifyUrl:      notifyUrl,
			ReturnUrl:      returnUrl,
		})
	}
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:          id,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   req.PaymentMethod,
		PaymentProvider: config.Provider,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付网关 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%d error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付网关 充值订单创建成功 user_id=%d trade_no=%s payment_provider=%s payment_method=%s amount=%d money=%.2f uri=%q params=%q", id, tradeNo, config.Provider, req.PaymentMethod, req.Amount, payMoney, uri, common.GetJsonString(params)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
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
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	tradeNo := params["out_trade_no"]
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	config := getGatewayConfig(topUp.PaymentMethod)
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
		verifyInfo, verifyErr := client.Verify(params)
		if verifyErr == nil {
			verifyTradeNo = verifyInfo.ServiceTradeNo
			verifyType = "zpay_" + verifyInfo.Type
			verifyStatus = verifyInfo.TradeStatus
			verifyOK = verifyInfo.VerifyStatus
		}
	} else {
		client := GetEpayClient()
		if client == nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
			_, err := c.Writer.Write([]byte("fail"))
			if err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			}
			return
		}
		verifyInfo, verifyErr := client.Verify(params)
		if verifyErr == nil {
			verifyTradeNo = verifyInfo.ServiceTradeNo
			verifyType = verifyInfo.Type
			verifyStatus = verifyInfo.TradeStatus
			verifyOK = verifyInfo.VerifyStatus
		}
	}
	if verifyErr == nil && verifyOK {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付网关 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s", verifyTradeNo, verifyType, verifyStatus, c.ClientIP()))
		_, err := c.Writer.Write([]byte("success"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("支付网关 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyTradeNo, c.ClientIP(), err.Error()))
		}
	} else {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		return
	}

	if verifyStatus == epay.StatusTradeSuccess || verifyStatus == "TRADE_SUCCESS" {
		LockOrder(verifyTradeNo)
		defer UnlockOrder(verifyTradeNo)
		if config.IsZPay {
			if topUp.PaymentProvider != model.PaymentProviderZPay {
				return
			}
			if params["money"] != strconv.FormatFloat(topUp.Money, 'f', 2, 64) {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("Z Pay 回调金额不匹配 trade_no=%s callback_money=%s order_money=%.2f client_ip=%s", verifyTradeNo, params["money"], topUp.Money, c.ClientIP()))
				return
			}
		} else if topUp.PaymentProvider != model.PaymentProviderEpay {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 trade_no=%s order_provider=%s callback_type=%s client_ip=%s", verifyTradeNo, topUp.PaymentProvider, verifyType, c.ClientIP()))
			return
		}
		if topUp.Status == common.TopUpStatusPending {
			if topUp.PaymentMethod != verifyType {
				logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付网关 实际支付方式与订单不同 trade_no=%s order_payment_method=%s actual_type=%s client_ip=%s", verifyTradeNo, topUp.PaymentMethod, verifyType, c.ClientIP()))
				topUp.PaymentMethod = verifyType
			}
			topUp.Status = common.TopUpStatusSuccess
			err := topUp.Update()
			if err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 更新充值订单失败 trade_no=%s user_id=%d client_ip=%s error=%q topup=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), err.Error(), common.GetJsonString(topUp)))
				return
			}
			//user, _ := model.GetUserById(topUp.UserId, false)
			//user.Quota += topUp.Amount * 500000
			dAmount := decimal.NewFromInt(int64(topUp.Amount))
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())
			err = model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true)
			if err != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 更新用户额度失败 trade_no=%s user_id=%d client_ip=%s quota_to_add=%d error=%q topup=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), quotaToAdd, err.Error(), common.GetJsonString(topUp)))
				return
			}
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值成功 trade_no=%s user_id=%d client_ip=%s quota_to_add=%d money=%.2f topup=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), quotaToAdd, topUp.Money, common.GetJsonString(topUp)))
			model.RecordTopupLog(topUp.UserId, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money), c.ClientIP(), topUp.PaymentMethod, "epay")
		}
	} else {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("支付网关 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s", verifyTradeNo, verifyType, verifyStatus, c.ClientIP()))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	config := getGatewayConfig(req.PaymentMethod)
	if req.Amount < getGatewayMinTopup(config.MinTopUp) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getGatewayMinTopup(config.MinTopUp))})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getGatewayPayMoney(req.Amount, group, config.Price)
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

type AdminZPayOrderRequest struct {
	TradeNo string `json:"trade_no"`
}

type AdminZPayRefundRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminQueryZPayOrder(c *gin.Context) {
	var req AdminZPayOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderZPay {
		common.ApiErrorMsg(c, "该订单不是 Z Pay 订单")
		return
	}
	config := getGatewayConfig(topUp.PaymentMethod)
	if !config.IsZPay {
		config = getGatewayConfig("zpay_alipay")
	}
	client := getZPayClient(config)
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置 Z Pay 支付信息")
		return
	}
	orderInfo, err := client.QueryOrderByTradeNo(req.TradeNo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, orderInfo)
}

func AdminRefundZPayOrder(c *gin.Context) {
	var req AdminZPayRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	topUp := model.GetTopUpByTradeNo(req.TradeNo)
	if topUp == nil {
		common.ApiErrorMsg(c, "订单不存在")
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderZPay {
		common.ApiErrorMsg(c, "该订单不是 Z Pay 订单")
		return
	}
	config := getGatewayConfig(topUp.PaymentMethod)
	if !config.IsZPay {
		config = getGatewayConfig("zpay_alipay")
	}
	client := getZPayClient(config)
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置 Z Pay 支付信息")
		return
	}

	var result *service.ZPayRefundResult
	err := model.RefundZPayTopUp(req.TradeNo, c.ClientIP(), func(topUp *model.TopUp) error {
		refundResult, err := client.RefundOrder(topUp.TradeNo, strconv.FormatFloat(topUp.Money, 'f', 2, 64))
		if err != nil {
			return err
		}
		if !refundResult.Success() {
			if refundResult != nil && refundResult.Msg != "" {
				return fmt.Errorf("Z Pay 退款失败：%s", refundResult.Msg)
			}
			return fmt.Errorf("Z Pay 退款失败")
		}
		result = refundResult
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
