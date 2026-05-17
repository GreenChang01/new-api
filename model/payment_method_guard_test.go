package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertUserForPaymentGuardTest(t *testing.T, id int, quota int) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: "payment_guard_user",
		Status:   common.UserStatusEnabled,
		Quota:    quota,
	}
	require.NoError(t, DB.Create(user).Error)
}

func insertSubscriptionPlanForPaymentGuardTest(t *testing.T, id int) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:            id,
		Title:         "Guard Plan",
		PriceAmount:   9.99,
		Currency:      "USD",
		DurationUnit:  SubscriptionDurationMonth,
		DurationValue: 1,
		Enabled:       true,
		TotalAmount:   1000,
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func insertSubscriptionOrderForPaymentGuardTest(t *testing.T, tradeNo string, userID int, planID int, paymentProvider string) {
	t.Helper()
	order := &SubscriptionOrder{
		UserId:          userID,
		PlanId:          planID,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, order.Insert())
}

func insertTopUpForPaymentGuardTest(t *testing.T, tradeNo string, userID int, paymentProvider string) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   paymentProvider,
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func insertSuccessfulTopUpForRefundTest(t *testing.T, tradeNo string, userID int, paymentProvider string, amount int64) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   "zpay_alipay",
		PaymentProvider: paymentProvider,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      time.Now().Unix(),
		CompleteTime:    time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())
}

func getTopUpStatusForPaymentGuardTest(t *testing.T, tradeNo string) string {
	t.Helper()
	topUp := GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	return topUp.Status
}

func countUserSubscriptionsForPaymentGuardTest(t *testing.T, userID int) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&UserSubscription{}).Where("user_id = ?", userID).Count(&count).Error)
	return count
}

func getUserQuotaForPaymentGuardTest(t *testing.T, userID int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userID).First(&user).Error)
	return user.Quota
}

func TestRechargeWaffoPancake_RejectsMismatchedPaymentMethod(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 101, 0)
	insertTopUpForPaymentGuardTest(t, "waffo-pancake-guard", 101, PaymentProviderStripe)

	err := RechargeWaffoPancake("waffo-pancake-guard")
	require.Error(t, err)

	topUp := GetTopUpByTradeNo("waffo-pancake-guard")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, 0, getUserQuotaForPaymentGuardTest(t, 101))
}

func TestUpdatePendingTopUpStatus_RejectsMismatchedPaymentProvider(t *testing.T) {
	testCases := []struct {
		name                    string
		tradeNo                 string
		storedPaymentProvider   string
		expectedPaymentProvider string
		targetStatus            string
	}{
		{
			name:                    "stripe expire",
			tradeNo:                 "stripe-expire-guard",
			storedPaymentProvider:   PaymentProviderCreem,
			expectedPaymentProvider: PaymentProviderStripe,
			targetStatus:            common.TopUpStatusExpired,
		},
		{
			name:                    "waffo failed",
			tradeNo:                 "waffo-failed-guard",
			storedPaymentProvider:   PaymentProviderStripe,
			expectedPaymentProvider: PaymentProviderWaffo,
			targetStatus:            common.TopUpStatusFailed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			insertUserForPaymentGuardTest(t, 150, 0)
			insertTopUpForPaymentGuardTest(t, tc.tradeNo, 150, tc.storedPaymentProvider)

			err := UpdatePendingTopUpStatus(tc.tradeNo, tc.expectedPaymentProvider, tc.targetStatus)
			require.ErrorIs(t, err, ErrPaymentMethodMismatch)
			assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, tc.tradeNo))
		})
	}
}

func TestRefundZPayTopUp_RejectsPendingOrder(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 501, 2000000)
	insertTopUpForPaymentGuardTest(t, "zpay-pending-refund", 501, PaymentProviderZPay)

	called := false
	err := RefundZPayTopUp("zpay-pending-refund", "127.0.0.1", func(topUp *TopUp) error {
		called = true
		return nil
	})

	require.Error(t, err)
	assert.False(t, called)
	assert.Equal(t, common.TopUpStatusPending, getTopUpStatusForPaymentGuardTest(t, "zpay-pending-refund"))
	assert.Equal(t, 2000000, getUserQuotaForPaymentGuardTest(t, 501))
}

func TestRefundZPayTopUp_SuccessDeductsQuotaAndMarksRefunded(t *testing.T) {
	truncateTables(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	insertUserForPaymentGuardTest(t, 502, 2000000)
	insertSuccessfulTopUpForRefundTest(t, "zpay-success-refund", 502, PaymentProviderZPay, 2)

	err := RefundZPayTopUp("zpay-success-refund", "127.0.0.1", func(topUp *TopUp) error {
		require.Equal(t, "zpay-success-refund", topUp.TradeNo)
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, common.TopUpStatusRefunded, getTopUpStatusForPaymentGuardTest(t, "zpay-success-refund"))
	assert.Equal(t, 1000000, getUserQuotaForPaymentGuardTest(t, 502))
}

func TestRefundZPayTopUp_RejectsDuplicateRefund(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 503, 2000000)
	topUp := &TopUp{
		UserId:          503,
		Amount:          2,
		Money:           9.99,
		TradeNo:         "zpay-duplicate-refund",
		PaymentMethod:   "zpay_alipay",
		PaymentProvider: PaymentProviderZPay,
		Status:          common.TopUpStatusRefunded,
		CreateTime:      time.Now().Unix(),
		CompleteTime:    time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	called := false
	err := RefundZPayTopUp("zpay-duplicate-refund", "127.0.0.1", func(topUp *TopUp) error {
		called = true
		return nil
	})

	require.Error(t, err)
	assert.False(t, called)
	assert.Equal(t, common.TopUpStatusRefunded, getTopUpStatusForPaymentGuardTest(t, "zpay-duplicate-refund"))
	assert.Equal(t, 2000000, getUserQuotaForPaymentGuardTest(t, 503))
}

func TestRefundZPayTopUp_RejectsInsufficientQuota(t *testing.T) {
	truncateTables(t)

	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	insertUserForPaymentGuardTest(t, 504, 999999)
	insertSuccessfulTopUpForRefundTest(t, "zpay-insufficient-refund", 504, PaymentProviderZPay, 2)

	called := false
	err := RefundZPayTopUp("zpay-insufficient-refund", "127.0.0.1", func(topUp *TopUp) error {
		called = true
		return nil
	})

	require.Error(t, err)
	assert.False(t, called)
	assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, "zpay-insufficient-refund"))
	assert.Equal(t, 999999, getUserQuotaForPaymentGuardTest(t, 504))
}

func TestCompleteSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 202, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 301)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-guard-order", 202, plan.Id, PaymentProviderStripe)

	err := CompleteSubscriptionOrder("sub-guard-order", `{"provider":"epay"}`, PaymentProviderEpay, "alipay")
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-guard-order")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
	assert.Zero(t, countUserSubscriptionsForPaymentGuardTest(t, 202))

	topUp := GetTopUpByTradeNo("sub-guard-order")
	assert.Nil(t, topUp)
}

func TestExpireSubscriptionOrder_RejectsMismatchedPaymentProvider(t *testing.T) {
	truncateTables(t)

	insertUserForPaymentGuardTest(t, 303, 0)
	plan := insertSubscriptionPlanForPaymentGuardTest(t, 401)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-expire-guard", 303, plan.Id, PaymentProviderStripe)

	err := ExpireSubscriptionOrder("sub-expire-guard", PaymentProviderCreem)
	require.ErrorIs(t, err, ErrPaymentMethodMismatch)

	order := GetSubscriptionOrderByTradeNo("sub-expire-guard")
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}
