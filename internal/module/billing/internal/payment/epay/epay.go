package epay

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/infra/protocolkey"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/pkg/logger"
)

// ErrQueryNotSupported is returned when the payment gateway implements
// neither the standard EPay order query API nor the EasyPay-compatible
// fallback API — the endpoint returns HTTP 404 or answers with something
// that is not JSON at all (e.g. an HTML error page).
var ErrQueryNotSupported = errors.New("gateway does not support order query API")

// decodeQueryResponse parses a gateway query payload. A body that is not
// JSON at all is evidence the gateway does not implement the query protocol,
// so it maps to ErrQueryNotSupported instead of a fatal decode error —
// otherwise a gateway serving HTML error pages on the query path would also
// block signature-verified callbacks and close attempts.
func decodeQueryResponse(name string, body []byte, target interface{}) error {
	if err := json.Unmarshal(body, target); err != nil {
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) {
			return fmt.Errorf("%s query returned a non-JSON response: %w", name, ErrQueryNotSupported)
		}
		return fmt.Errorf("decode %s query response: %w", name, err)
	}
	return nil
}

type Client struct {
	Pid        string
	Url        string
	Key        string
	Type       string
	httpClient *http.Client
}

type Order struct {
	Name      string
	OrderNo   string
	Amount    float64
	SignType  string
	NotifyUrl string
	ReturnUrl string
}

type QueryResult struct {
	MerchantID string
	TradeNo    string
	OrderNo    string
	Type       string
	Money      string
	Paid       bool
	Message    string
	// StatusOnly reports that the gateway query API returned only a payment
	// status. Callers must not treat omitted payment details as verified.
	StatusOnly bool
}

type queryOrderResponse struct {
	Code       int             `json:"code"`
	Msg        string          `json:"msg"`
	TradeNo    string          `json:"trade_no"`
	OutTradeNo string          `json:"out_trade_no"`
	Type       string          `json:"type"`
	Money      string          `json:"money"`
	Pid        json.RawMessage `json:"pid"`
	Status     int             `json:"status"`
}

// easyPayQueryOrderResponse is used by a non-standard EPay variant. Its
// query response intentionally contains only an order status, rather than
// the full payment details returned by standard EPay's api.php endpoint.
type easyPayQueryOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data *struct {
		Status string `json:"status"`
	} `json:"data"`
}

func NewClient(pid, url, key string, Type string) *Client {
	return &Client{
		Pid:  pid,
		Url:  url,
		Key:  key,
		Type: Type,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// CreatePayUrl builds the browser redirect URL for EPay's submit.php endpoint.
func (c *Client) CreatePayUrl(order Order) (string, error) {
	endpoint, err := c.endpoint("submit.php")
	if err != nil {
		return "", err
	}
	params := c.orderParams(order)
	params["sign"] = c.createSign(params)
	params["sign_type"] = "MD5"
	return endpoint.String() + "?" + encodeParams(params), nil
}

func (c *Client) createSign(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if params[k] != "" && k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	queryString := strings.Join(parts, "&")
	text := queryString + c.Key
	return protocolkey.Md5Encode(text, false)
}

func (c *Client) VerifySign(params map[string]string) bool {
	expected := c.createSign(params)
	received := strings.ToLower(params["sign"])
	if len(expected) != len(received) || len(received) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(received)) == 1
}

// QueryOrder obtains payment details directly from the gateway. It first uses
// the standard EPay api.php protocol. When that endpoint is absent, it falls
// back to a known EasyPay-compatible POST protocol that exposes only status.
func (c *Client) QueryOrder(orderNo string) (*QueryResult, error) {
	if orderNo == "" {
		return nil, errors.New("order number is empty")
	}
	result, err := c.queryStandardOrder(orderNo)
	if !errors.Is(err, ErrQueryNotSupported) {
		return result, err
	}
	return c.queryEasyPayOrder(orderNo)
}

// queryStandardOrder implements EPay's GET api.php?act=order protocol.
func (c *Client) queryStandardOrder(orderNo string) (*QueryResult, error) {
	endpoint, err := c.endpoint("api.php")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("act", "order")
	query.Set("pid", c.Pid)
	query.Set("key", c.Key)
	query.Set("out_trade_no", orderNo)
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("create gateway query request failed")
	}
	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("gateway query request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrQueryNotSupported
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("query gateway returned HTTP %d", resp.StatusCode)
	}
	const maxQueryResponseSize = 1 << 20
	value, err := io.ReadAll(io.LimitReader(resp.Body, maxQueryResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read query response: %w", err)
	}
	if len(value) > maxQueryResponseSize {
		return nil, errors.New("query response is too large")
	}
	var response queryOrderResponse
	if err := decodeQueryResponse("gateway", value, &response); err != nil {
		return nil, err
	}
	if response.Code != 1 {
		return nil, fmt.Errorf("gateway order lookup failed: code=%d", response.Code)
	}
	merchantID, err := rawString(response.Pid)
	if err != nil {
		return nil, fmt.Errorf("decode merchant id: %w", err)
	}
	return &QueryResult{
		MerchantID: merchantID,
		TradeNo:    response.TradeNo,
		OrderNo:    response.OutTradeNo,
		Type:       response.Type,
		Money:      response.Money,
		Paid:       response.Status == 1,
		Message:    response.Msg,
	}, nil
}

// queryEasyPayOrder implements the status-only fallback used by a known
// modified EPay gateway. It is attempted only after standard api.php returns
// 404, so existing EPay integrations keep their normal protocol.
func (c *Client) queryEasyPayOrder(orderNo string) (*QueryResult, error) {
	endpoint, err := c.endpoint("api/EasyPay/queryOrder")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), strings.NewReader(url.Values{
		"orderNo": []string{orderNo},
	}.Encode()))
	if err != nil {
		return nil, errors.New("create EasyPay query request failed")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := c.httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("EasyPay query request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrQueryNotSupported
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("EasyPay query returned HTTP %d", resp.StatusCode)
	}
	const maxQueryResponseSize = 1 << 20
	value, err := io.ReadAll(io.LimitReader(resp.Body, maxQueryResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read EasyPay query response: %w", err)
	}
	if len(value) > maxQueryResponseSize {
		return nil, errors.New("EasyPay query response is too large")
	}
	var response easyPayQueryOrderResponse
	if err := decodeQueryResponse("EasyPay", value, &response); err != nil {
		return nil, err
	}
	if response.Code != 1 {
		return nil, fmt.Errorf("EasyPay order lookup failed: code=%d", response.Code)
	}
	if response.Data == nil || strings.TrimSpace(response.Data.Status) == "" {
		return nil, errors.New("EasyPay query response has no order status")
	}
	return &QueryResult{
		Paid:       strings.EqualFold(response.Data.Status, "success"),
		Message:    response.Msg,
		StatusOnly: true,
	}, nil
}

// QueryOrderStatus is kept for callers that only need a status boolean.
func (c *Client) QueryOrderStatus(orderNo string) bool {
	result, err := c.QueryOrder(orderNo)
	if err != nil {
		logger.Error("[Epay] QueryOrderStatus error", logger.Field("orderNo", orderNo), logger.Field("error", err.Error()))
		return false
	}
	return result.Paid
}

// FormatMoney returns the exact two-decimal amount sent to an EPay-compatible
// gateway. It intentionally preserves the historical truncation behaviour.
func FormatMoney(amount float64) string {
	return payment.FormatFloat(amount, 2)
}

// ParseMoney converts a non-negative decimal amount to its integer minor unit.
// It rejects floats, exponents and values with more than two decimal places.
func ParseMoney(value string) (int64, error) {
	return payment.ParseAmount(value)
}

func rawString(value json.RawMessage) (string, error) {
	if len(value) == 0 || string(value) == "null" {
		return "", errors.New("merchant id is missing")
	}
	var text string
	if value[0] == '"' {
		if err := json.Unmarshal(value, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	var number json.Number
	if err := json.Unmarshal(value, &number); err != nil {
		return "", err
	}
	return number.String(), nil
}

func (c *Client) endpoint(script string) (*url.URL, error) {
	endpoint, err := url.Parse(c.Url)
	if err != nil {
		return nil, fmt.Errorf("parse payment endpoint: %w", err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" || endpoint.Host == "" {
		return nil, errors.New("unsupported payment endpoint")
	}
	if endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return nil, errors.New("payment endpoint must not include query or fragment")
	}
	path := strings.TrimRight(endpoint.Path, "/")
	for _, knownScript := range []string{"submit.php", "api.php"} {
		if strings.HasSuffix(path, "/"+knownScript) {
			path = strings.TrimSuffix(path, "/"+knownScript)
			break
		}
	}
	endpoint.Path = path + "/" + script
	endpoint.RawPath = ""
	return endpoint, nil
}

func (c *Client) orderParams(order Order) map[string]string {
	return map[string]string{
		"money":        FormatMoney(order.Amount),
		"name":         order.Name,
		"notify_url":   order.NotifyUrl,
		"out_trade_no": order.OrderNo,
		"pid":          c.Pid,
		"type":         c.Type,
		"return_url":   order.ReturnUrl,
	}
}

func encodeParams(params map[string]string) string {
	values := make(url.Values, len(params))
	for key, value := range params {
		values.Set(key, value)
	}
	return values.Encode()
}
