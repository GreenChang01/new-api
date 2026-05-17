/**
此文件为旧版支付设置文件，如需增加新的参数、变量等，请在 payment_setting.go 中添加
This file is the old version of the payment settings file. If you need to add new parameters, variables, etc., please add them in payment_setting.go
*/

package operation_setting

import (
	"github.com/QuantumNous/new-api/common"
)

var PayAddress = ""
var CustomCallbackAddress = ""
var EpayId = ""
var EpayKey = ""
var Price = 7.3
var MinTopUp = 1
var ZPayAddress = ""
var ZPayId = ""
var ZPayKey = ""
var ZPayChannelID = ""
var ZPayPrice = 7.3
var ZPayMinTopUp = 1
var USDExchangeRate = 7.3

var PayMethods = []map[string]string{
	{
		"name":  "支付宝",
		"color": "rgba(var(--semi-blue-5), 1)",
		"type":  "alipay",
	},
	{
		"name":  "微信",
		"color": "rgba(var(--semi-green-5), 1)",
		"type":  "wxpay",
	},
	{
		"name":      "自定义1",
		"color":     "black",
		"type":      "custom1",
		"min_topup": "50",
	},
}

var ZPayMethods = []map[string]string{
	{
		"name":  "Z Pay 支付宝",
		"color": "#1677FF",
		"type":  "zpay_alipay",
	},
}

func UpdatePayMethodsByJsonString(jsonString string) error {
	PayMethods = make([]map[string]string, 0)
	return common.Unmarshal([]byte(jsonString), &PayMethods)
}

func PayMethods2JsonString() string {
	jsonBytes, err := common.Marshal(PayMethods)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func UpdateZPayMethodsByJsonString(jsonString string) error {
	ZPayMethods = make([]map[string]string, 0)
	return common.Unmarshal([]byte(jsonString), &ZPayMethods)
}

func ZPayMethods2JsonString() string {
	jsonBytes, err := common.Marshal(ZPayMethods)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

func ContainsPayMethod(method string) bool {
	for _, payMethod := range PayMethods {
		if payMethod["type"] == method {
			return true
		}
	}
	return false
}

func ContainsZPayMethod(method string) bool {
	for _, payMethod := range ZPayMethods {
		if payMethod["type"] == method {
			return true
		}
	}
	return false
}
