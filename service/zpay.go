package service

import (
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
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

type ZPayOrderInfo struct {
	Code          int    `json:"code"`
	Msg           string `json:"msg"`
	TradeNo       string `json:"trade_no"`
	ServiceTradeNo string `json:"out_trade_no"`
	Type          string `json:"type"`
	MerchantID    string `json:"pid"`
	AddTime       string `json:"addtime"`
	EndTime       string `json:"endtime"`
	Name          string `json:"name"`
	Money         string `json:"money"`
	Status        int    `json:"status"`
	Param         string `json:"param"`
	Buyer         string `json:"buyer"`
}

type ZPayRefundResult struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
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

func (c *ZPayClient) QueryOrderByTradeNo(outTradeNo string) (*ZPayOrderInfo, error) {
	if c == nil {
		return nil, fmt.Errorf("zpay client is nil")
	}
	queryURL := fmt.Sprintf("%s/api.php?act=order&pid=%s&key=%s&out_trade_no=%s",
		c.Endpoint,
		url.QueryEscape(c.MerchantID),
		url.QueryEscape(c.Key),
		url.QueryEscape(outTradeNo),
	)
	client := GetHttpClient()
	if client == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	req, err := http.NewRequest(http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	result := &ZPayOrderInfo{}
	if err := common.Unmarshal(body, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *ZPayClient) RefundOrder(outTradeNo string, money string) (*ZPayRefundResult, error) {
	if c == nil {
		return nil, fmt.Errorf("zpay client is nil")
	}
	values := url.Values{}
	values.Set("act", "refund")
	values.Set("pid", c.MerchantID)
	values.Set("key", c.Key)
	values.Set("out_trade_no", outTradeNo)
	values.Set("money", money)
	client := GetHttpClient()
	if client == nil {
		return nil, fmt.Errorf("http client is nil")
	}
	resp, err := client.Post(c.Endpoint+"/api.php", "application/x-www-form-urlencoded", strings.NewReader(values.Encode()))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	result := &ZPayRefundResult{}
	if err := common.Unmarshal(body, result); err != nil {
		return nil, err
	}
	return result, nil
}
