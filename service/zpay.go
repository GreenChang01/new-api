package service

import (
	"crypto/md5"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type ZPayClient struct {
	Endpoint   string
	MerchantID string
	Key        string
}

type ZPayPurchaseArgs struct {
	Type           string
	ServiceTradeNo string
	Name           string
	Money          string
	NotifyURL      string
	ReturnURL      string
	ClientIP       string
	Device         string
	Param          string
	ChannelID      string
}

type ZPayVerifyInfo struct {
	MerchantID     string
	Name           string
	Money          string
	ServiceTradeNo string
	TradeNo        string
	Param          string
	TradeStatus    string
	Type           string
	Sign           string
	SignType       string
	VerifyStatus   bool
}

func NewZPayClient(endpoint string, merchantID string, key string) *ZPayClient {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" || merchantID == "" || key == "" {
		return nil
	}
	return &ZPayClient{
		Endpoint:   endpoint,
		MerchantID: strings.TrimSpace(merchantID),
		Key:        strings.TrimSpace(key),
	}
}

func zpaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v == "" || k == "sign" || k == "sign_type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&") + key
	sum := md5.Sum([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

func (c *ZPayClient) Purchase(args *ZPayPurchaseArgs) (string, map[string]string, error) {
	if c == nil {
		return "", nil, fmt.Errorf("zpay client is nil")
	}
	if args == nil {
		return "", nil, fmt.Errorf("purchase args is nil")
	}
	params := map[string]string{
		"name":         args.Name,
		"money":        args.Money,
		"type":         args.Type,
		"out_trade_no": args.ServiceTradeNo,
		"notify_url":   args.NotifyURL,
		"pid":          c.MerchantID,
		"return_url":   args.ReturnURL,
		"sign_type":    "MD5",
	}
	if args.Param != "" {
		params["param"] = args.Param
	}
	if args.ChannelID != "" {
		params["cid"] = args.ChannelID
	}
	if args.ClientIP != "" {
		params["clientip"] = args.ClientIP
	}
	if args.Device != "" {
		params["device"] = args.Device
	}
	params["sign"] = zpaySign(params, c.Key)
	return c.Endpoint + "/submit.php", params, nil
}

func (c *ZPayClient) Verify(params map[string]string) (*ZPayVerifyInfo, error) {
	if c == nil {
		return nil, fmt.Errorf("zpay client is nil")
	}
	expected := zpaySign(params, c.Key)
	info := &ZPayVerifyInfo{
		MerchantID:     params["pid"],
		Name:           params["name"],
		Money:          params["money"],
		ServiceTradeNo: params["out_trade_no"],
		TradeNo:        params["trade_no"],
		Param:          params["param"],
		TradeStatus:    params["trade_status"],
		Type:           params["type"],
		Sign:           params["sign"],
		SignType:       params["sign_type"],
		VerifyStatus:   strings.EqualFold(params["sign"], expected),
	}
	return info, nil
}

func ZPayDeviceFromUserAgent(userAgent string) string {
	ua := strings.ToLower(userAgent)
	if strings.Contains(ua, "android") || strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad") || strings.Contains(ua, "mobile") {
		return "mobile"
	}
	return "pc"
}

func ZPayCallbackURL(base string, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	return base + path
}

func ZPayQueryString(params map[string]string) string {
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return values.Encode()
}

func ZPayJSON(data any) string {
	return common.GetJsonString(data)
}
