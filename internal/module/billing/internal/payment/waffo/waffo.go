// Package waffo adapts the Waffo Pancake merchant-of-record gateway to the
// billing module: hosted checkout sessions, webhook verification and the
// order re-query used to confirm a settlement.
// https://github.com/waffo-com/waffo-pancake-sdk-go
package waffo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	pancake "github.com/waffo-com/waffo-pancake-sdk-go"

	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
)

// EventOrderCompleted is the only event that settles a PPanel order: the
// storefront sells one-time products, so a completed order is a paid order.
const EventOrderCompleted = "order.completed"

// DefaultTaxCategory classifies PPanel subscriptions for the MoR's tax
// engine when an administrator leaves the field blank.
const DefaultTaxCategory = "saas"

// KnownTaxCategory reports whether a category is one the gateway accepts.
// The gateway rejects anything else at checkout, so the value is validated
// when the payment method is saved rather than at purchase time.
func KnownTaxCategory(category string) bool {
	switch pancake.TaxCategory(category) {
	case pancake.TaxCategoryDigitalGoods, pancake.TaxCategorySaaS, pancake.TaxCategorySoftware,
		pancake.TaxCategoryEbook, pancake.TaxCategoryOnlineCourse, pancake.TaxCategoryConsulting,
		pancake.TaxCategoryProfessionalService:
		return true
	default:
		return false
	}
}

type Config struct {
	MerchantID  string
	PrivateKey  string
	StoreID     string
	ProductID   string
	TaxCategory string
	// TestMode selects the gateway environment. It decides which webhook
	// signing key a callback is verified against, so it must match the
	// mode the store is operating in.
	TestMode bool
	// BaseURL overrides the production API endpoint; tests use it.
	BaseURL string
	// WebhookPublicKey overrides the signing key webhooks are verified
	// against. The SDK ships the gateway's current keys, so this is only
	// needed when Waffo rotates them ahead of an SDK release.
	WebhookPublicKey string
}

type Client struct {
	Config
	api *pancake.Client
}

func NewClient(config Config) (*Client, error) {
	if config.MerchantID == "" || config.PrivateKey == "" {
		return nil, errors.New("incomplete Waffo configuration")
	}
	if config.TaxCategory == "" {
		config.TaxCategory = DefaultTaxCategory
	}
	api, err := pancake.New(pancake.Config{
		MerchantID:  config.MerchantID,
		PrivateKey:  config.PrivateKey,
		BaseURL:     config.BaseURL,
		Environment: config.environment(),
		HTTPClient:  &http.Client{Timeout: 15 * time.Second},
		WebhookPublicKey: pancake.WebhookPublicKeys{
			Shared: config.WebhookPublicKey,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create Waffo client: %w", err)
	}
	return &Client{Config: config, api: api}, nil
}

func (c Config) environment() pancake.Environment {
	if c.TestMode {
		return pancake.EnvironmentTest
	}
	return pancake.EnvironmentProd
}

// Order describes the checkout to create. Amount is in minor units of
// Currency and is the pre-tax price: as merchant of record the gateway adds
// the buyer's tax on top of it.
type Order struct {
	OrderNo string
	Amount  int64
	// BuyerIdentity binds the gateway order to a PPanel identity that the
	// buyer cannot edit on the checkout page.
	BuyerIdentity string
	BuyerEmail    string
	Currency      string
	ReturnURL     string
	// ExpiresInSeconds keeps the session from outliving the order's close
	// window; zero keeps the gateway default (45 minutes).
	ExpiresInSeconds int
}

// Session is a created hosted checkout. The gateway order number only exists
// once the buyer pays, so a session carries no trade number to claim.
type Session struct {
	SessionID   string
	CheckoutURL string
	ExpiresAt   string
}

// CreateCheckout opens a hosted checkout session for the order. The price is
// sent as a snapshot override so a single dashboard product can carry every
// PPanel order amount.
func (c *Client) CreateCheckout(ctx context.Context, order Order) (*Session, error) {
	if order.OrderNo == "" || order.Amount <= 0 || order.Currency == "" {
		return nil, errors.New("invalid Waffo order")
	}
	if c.ProductID == "" {
		return nil, errors.New("waffo product id is not configured")
	}
	if order.BuyerIdentity == "" {
		return nil, errors.New("waffo checkout requires a buyer identity")
	}
	params := pancake.AuthenticatedCheckoutParams{
		CreateCheckoutSessionParams: pancake.CreateCheckoutSessionParams{
			ProductID: c.ProductID,
			Currency:  strings.ToUpper(order.Currency),
			PriceSnapshot: &pancake.PriceSnapshot{
				Amount:      FormatMoney(order.Amount),
				TaxCategory: pancake.TaxCategory(c.TaxCategory),
			},
			Metadata:                map[string]string{"order_no": order.OrderNo},
			OrderMerchantExternalID: optional(order.OrderNo),
		},
		BuyerIdentity: order.BuyerIdentity,
	}
	if order.BuyerEmail != "" {
		params.BuyerEmail = optional(order.BuyerEmail)
	}
	if order.ReturnURL != "" {
		params.SuccessURL = optional(order.ReturnURL)
	}
	if order.ExpiresInSeconds > 0 {
		params.ExpiresInSeconds = &order.ExpiresInSeconds
	}
	result, err := c.api.Checkout.Authenticated.Create(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("create Waffo checkout: %w", err)
	}
	if result.CheckoutURL == "" {
		return nil, errors.New("waffo checkout has no url")
	}
	return &Session{
		SessionID:   result.SessionID,
		CheckoutURL: result.CheckoutURL,
		ExpiresAt:   result.ExpiresAt,
	}, nil
}

// GatewayOrder is the gateway's own view of a paid order, used to confirm a
// webhook before money is accepted. Subtotal is the pre-tax price; Total is
// what the buyer paid, tax included.
type GatewayOrder struct {
	ID                 string `json:"id"`
	Status             string `json:"status"`
	Currency           string `json:"currency"`
	TestMode           bool   `json:"testMode"`
	MerchantExternalID string `json:"orderMerchantExternalId"`
	Subtotal           Money  `json:"subtotal"`
	TaxAmount          Money  `json:"taxAmount"`
	Total              Money  `json:"total"`
}

// Money is the gateway's amount type: a decimal display string plus its
// currency.
type Money struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// The id argument is String, not the ID scalar the SDK's GraphQL guide shows;
// the gateway rejects `$id: ID!` with `Unknown type "ID"`.
const orderQuery = `query ($id: String!) {
  onetimeOrder(id: $id) {
    id status currency testMode orderMerchantExternalId
    subtotal { amount currency }
    taxAmount { amount currency }
    total { amount currency }
  }
}`

// GetOrder reads a one-time order back from the gateway. A webhook signature
// proves who sent the payload; this proves the payment is really settled.
func (c *Client) GetOrder(ctx context.Context, orderID string) (*GatewayOrder, error) {
	if orderID == "" {
		return nil, errors.New("order lookup requires an order id")
	}
	response, err := c.api.GraphQL.Query(ctx, pancake.GraphQLParams{
		Query:     orderQuery,
		Variables: map[string]any{"id": orderID},
	})
	if err != nil {
		return nil, fmt.Errorf("query Waffo order: %w", err)
	}
	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("query Waffo order: %s", response.Errors[0].Message)
	}
	var payload struct {
		OnetimeOrder *GatewayOrder `json:"onetimeOrder"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		return nil, fmt.Errorf("decode Waffo order: %w", err)
	}
	if payload.OnetimeOrder == nil || payload.OnetimeOrder.ID == "" {
		return nil, errors.New("waffo order not found")
	}
	return payload.OnetimeOrder, nil
}

// Completed reports whether the gateway considers the order settled.
func (o *GatewayOrder) Completed() bool {
	return o.Status == string(pancake.OnetimeOrderStatusCompleted)
}

// Notification is the subset of a verified webhook event the settlement path
// needs. Amount is the pre-tax price; the buyer additionally pays TaxAmount.
type Notification struct {
	EventID   string
	EventType string
	TestMode  bool
	OrderID   string
	OrderNo   string
	Status    string
	Currency  string
	Amount    string
	TaxAmount string
	PaymentID string
}

// VerifyNotification authenticates a webhook payload against the environment
// the payment method is configured for and flattens it into a Notification.
// The payload must be the raw request body: re-encoded JSON breaks the
// signature.
func (c *Client) VerifyNotification(payload []byte, signatureHeader string) (*Notification, error) {
	event, err := c.api.Webhooks.Verify(string(payload), signatureHeader, &pancake.VerifyWebhookOptions{
		Environment: c.environment(),
	})
	if err != nil {
		return nil, err
	}
	var data pancake.WebhookEventData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return nil, fmt.Errorf("decode Waffo event data: %w", err)
	}
	notification := &Notification{
		EventID:   event.ID,
		EventType: event.EventType,
		TestMode:  event.Mode == pancake.EnvironmentTest,
		OrderID:   data.OrderID,
		Currency:  data.Currency,
		Amount:    data.Amount,
		TaxAmount: data.TaxAmount,
	}
	if data.OrderMerchantExternalID != nil {
		notification.OrderNo = *data.OrderMerchantExternalID
	}
	// A store created before the external-id field existed, or a checkout
	// opened by an older PPanel, only carries the order number in metadata.
	if notification.OrderNo == "" {
		notification.OrderNo = data.OrderMetadata["order_no"]
	}
	if data.OrderStatus != nil {
		notification.Status = *data.OrderStatus
	}
	if data.PaymentID != nil {
		notification.PaymentID = *data.PaymentID
	}
	return notification, nil
}

// RegisterWebhook points the store at PPanel's callback endpoint. Waffo keeps
// webhooks per environment, so a store running both modes needs one per mode.
// It returns the gateway's webhook id, which is stored in the payment config
// so a later save updates that endpoint instead of adding a second one.
func (c *Client) RegisterWebhook(ctx context.Context, webhookID, callbackURL string) (string, error) {
	if callbackURL == "" {
		return "", errors.New("waffo webhook needs a callback url")
	}
	events := []pancake.WebhookEventType{pancake.WebhookEventTypeOrderCompleted}
	if webhookID != "" {
		_, err := c.api.Webhooks.Update(ctx, pancake.UpdateWebhookParams{
			ID:     webhookID,
			URL:    optional(callbackURL),
			Events: events,
		})
		if err == nil {
			return webhookID, nil
		}
		// Only a gone endpoint (deleted in the dashboard, store recreated)
		// justifies registering a fresh one. Treating a transient failure the
		// same way leaves the store with two endpoints delivering every event
		// twice, while the config remembers only the newest id.
		if !isWebhookGone(err) {
			return "", fmt.Errorf("update Waffo webhook: %w", err)
		}
	}
	if c.StoreID == "" {
		return "", errors.New("waffo store id is not configured")
	}
	result, err := c.api.Webhooks.Add(ctx, pancake.AddWebhookParams{
		StoreID:  c.StoreID,
		Channel:  pancake.WebhookChannelHTTP,
		URL:      callbackURL,
		Events:   events,
		TestMode: c.TestMode,
	})
	if err != nil {
		return "", fmt.Errorf("register Waffo webhook: %w", err)
	}
	return result.Webhook.ID, nil
}

// isWebhookGone reports whether the gateway rejected the update because the
// endpoint no longer exists, as opposed to any other failure.
func isWebhookGone(err error) bool {
	var apiErr *pancake.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	if apiErr.Status != http.StatusNotFound && apiErr.Status != http.StatusBadRequest {
		return false
	}
	for _, notice := range apiErr.Errors {
		if strings.Contains(strings.ToLower(notice.Message), "not found") {
			return true
		}
	}
	return false
}

// FormatMoney renders a minor-unit amount as the decimal display string the
// gateway expects. PPanel holds every currency in hundredths, so a
// zero-decimal currency such as JPY is priced a hundred times too low here —
// the same limitation the other gateways carry.
func FormatMoney(amount int64) string {
	if amount < 0 {
		return "0.00"
	}
	return fmt.Sprintf("%d.%02d", amount/100, amount%100)
}

// ParseMoney converts a webhook display amount to integer minor units.
func ParseMoney(value string) (int64, error) {
	return payment.ParseAmount(value)
}

// ParseMinorUnits converts a GraphQL money amount to integer minor units.
//
// The two gateway surfaces disagree on how money is spelled: a webhook
// reports USD 12.34 as the display string "12.34", while the GraphQL API
// reports the same amount as "1234". Reading one with the other's parser is
// off by a factor of 100 and still looks like a valid amount, so a value
// carrying a decimal point is rejected here rather than silently accepted:
// it would mean the GraphQL convention changed.
func ParseMinorUnits(value string) (int64, error) {
	if value == "" || len(value) > 19 || strings.ContainsAny(value, ".,") {
		return 0, errors.New("invalid minor-unit amount")
	}
	amount, err := strconv.ParseInt(value, 10, 64)
	if err != nil || amount < 0 {
		return 0, errors.New("invalid minor-unit amount")
	}
	return amount, nil
}

func optional(value string) *string {
	return &value
}
