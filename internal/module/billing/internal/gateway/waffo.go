package gateway

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/waffo"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// waffoMinSession is the shortest hosted checkout worth opening: a buyer
// needs a moment to pay, and a shorter session would only expire under them.
const waffoMinSession = 2 * time.Minute

// waffoGateway speaks the Waffo Pancake merchant-of-record protocol. The
// store's webhook is registered when the payment method is saved, not per
// checkout, and a gateway order only exists once the buyer has paid.
type waffoGateway struct {
	client *waffo.Client
	config paymentEntity.WaffoConfig
}

func openWaffo(r *Registry, method *paymentEntity.Payment) (Gateway, error) {
	var config paymentEntity.WaffoConfig
	if err := config.Unmarshal([]byte(method.Config)); err != nil {
		return nil, fmt.Errorf("decode Waffo configuration: %w", err)
	}
	client, err := newWaffoClient(r, config)
	if err != nil {
		return nil, err
	}
	return &waffoGateway{client: client, config: config}, nil
}

func newWaffoClient(r *Registry, config paymentEntity.WaffoConfig) (*waffo.Client, error) {
	return waffo.NewClient(waffo.Config{
		MerchantID:  config.MerchantID,
		PrivateKey:  config.PrivateKey,
		StoreID:     config.StoreID,
		ProductID:   config.ProductID,
		TaxCategory: config.TaxCategory,
		TestMode:    config.TestMode,
		BaseURL:     r.waffoBaseURL,
	})
}

// normalizeWaffo trims the fields and requires every one a checkout needs.
// An unknown tax category is rejected by the gateway at purchase time, too
// late for the administrator to notice, so it is checked here.
func normalizeWaffo(raw []byte) (string, error) {
	var config paymentEntity.WaffoConfig
	if err := config.Unmarshal(raw); err != nil {
		return "", err
	}
	config.MerchantID = strings.TrimSpace(config.MerchantID)
	config.PrivateKey = strings.TrimSpace(config.PrivateKey)
	config.StoreID = strings.TrimSpace(config.StoreID)
	config.ProductID = strings.TrimSpace(config.ProductID)
	config.TaxCategory = strings.TrimSpace(config.TaxCategory)
	if config.TaxCategory == "" {
		config.TaxCategory = waffo.DefaultTaxCategory
	}
	if config.MerchantID == "" || config.PrivateKey == "" || config.StoreID == "" || config.ProductID == "" {
		return "", errors.New("incomplete payment configuration")
	}
	if !waffo.KnownTaxCategory(config.TaxCategory) {
		return "", errors.New("unknown Waffo tax category")
	}
	// The webhook id is the gateway's, recorded when the method is saved;
	// it is never taken from an administrator's request.
	config.WebhookID = ""
	content, err := config.Marshal()
	return string(content), err
}

func (g *waffoGateway) Platform() payment.Platform { return payment.Waffo }

func (g *waffoGateway) ChargeCurrency() string { return "" }

// NeedsNotifyURL is false: the callback endpoint is registered on the store
// when the payment method is saved, not passed with each checkout.
func (g *waffoGateway) NeedsNotifyURL(*order.Order) bool { return false }

// StartPayment opens a hosted checkout session for the order. The session
// ends when the order stops waiting for payment, so a buyer can never pay a
// session that outlives its order.
func (g *waffoGateway) StartPayment(ctx context.Context, c Checkout) (*dto.CheckoutOrderResponse, error) {
	remaining := order.UnpaidCloseAge - time.Since(c.Order.CreatedAt)
	if remaining < waffoMinSession {
		return nil, xerr.Errorf(xerr.ERROR, "order %s is about to close; place a new order", c.Order.OrderNo)
	}
	session, err := g.client.CreateCheckout(ctx, waffo.Order{
		OrderNo:          c.Order.OrderNo,
		Amount:           c.Charge.Amount,
		Currency:         c.Charge.Currency,
		BuyerIdentity:    waffoBuyerIdentity(c.Order),
		BuyerEmail:       c.Payer.Email,
		ReturnURL:        c.ReturnURL,
		ExpiresInSeconds: int(remaining.Seconds()),
	})
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.ERROR, "create Waffo checkout")
	}
	return &dto.CheckoutOrderResponse{Type: "url", CheckoutUrl: session.CheckoutURL}, nil
}

// waffoBuyerIdentity binds the gateway order to a PPanel identity the buyer
// cannot edit on the checkout page, unlike the email. A guest order has no
// account and is identified by its order number.
func waffoBuyerIdentity(o *order.Order) string {
	if o.UserId != 0 {
		return "user:" + strconv.FormatInt(o.UserId, 10)
	}
	return "order:" + o.OrderNo
}

// Reconcile decides whether a pending order may close. The gateway can only
// be queried by its own order id, which exists once the buyer has paid, so
// an unpaid checkout cannot be confirmed: the order waits until every
// session it could have opened has expired (StartPayment bounds them by
// UnpaidCloseAge), and a payment arriving after that is recorded as an
// unmatched payment by the callback flow.
func (g *waffoGateway) Reconcile(_ context.Context, req CloseRequest) (Reconciliation, error) {
	o := req.Order
	if o.PaymentCurrency == "" {
		return Reconciliation{RequireStableCheckout: true}, nil // checkout was never started.
	}
	if req.Explicit {
		return Reconciliation{}, nil
	}
	if time.Since(o.CreatedAt) < order.UnpaidCloseAge {
		return Reconciliation{}, fmt.Errorf("waffo order %s stays pending until its checkout sessions expire: %w", o.OrderNo, ErrUnconfirmed)
	}
	return Reconciliation{}, nil
}

// ParseCallback verifies the webhook. Only order.completed announces a
// payment: PPanel sells one-time products, so every other event concerns no
// order payment and is acknowledged without settling.
func (g *waffoGateway) ParseCallback(_ context.Context, n Notification) (*Notice, error) {
	notification, err := g.client.VerifyNotification(n.Body, n.Signature)
	if err != nil {
		return nil, errors.New("verify sign failed")
	}
	// The signature proves the payload came from Waffo, not that it came
	// from the environment this method is configured for: a test-mode event
	// settling a production order would hand out paid subscriptions.
	if notification.TestMode != g.config.TestMode {
		return nil, errors.New("webhook environment mismatch")
	}
	fields := []logger.LogField{
		logger.Field("eventId", notification.EventID),
		logger.Field("eventType", notification.EventType),
		logger.Field("gatewayOrderId", notification.OrderID),
	}
	if notification.EventType != waffo.EventOrderCompleted {
		return &Notice{OrderNo: notification.OrderNo, Ignore: true, Status: notification.EventType, Fields: fields}, nil
	}
	if notification.OrderNo == "" || len(notification.OrderNo) > 255 || strings.TrimSpace(notification.OrderNo) != notification.OrderNo {
		return nil, errors.New("invalid order number")
	}
	if err := settle.ValidateTradeNo(notification.OrderID); err != nil {
		return nil, err
	}
	// The webhook amount is the pre-tax price PPanel set; the merchant of
	// record collects tax on top of it, so it is what the order expects.
	amount, err := waffo.ParseMoney(notification.Amount)
	if err != nil {
		return nil, errors.New("invalid callback amount")
	}
	return &Notice{
		OrderNo:  notification.OrderNo,
		TradeNo:  notification.OrderID,
		Amount:   amount,
		Currency: notification.Currency,
		Paid:     true,
		Status:   notification.Status,
		Fields:   append(fields, logger.Field("taxAmount", notification.TaxAmount)),
	}, nil
}

// CheckOrder accepts any order: checkout claims no trade number, because the
// gateway order only exists after payment. A second payment of the same
// order is caught by the callback flow, which records it for a refund.
func (g *waffoGateway) CheckOrder(*order.Order, *Notice) error { return nil }

// ConfirmPayment re-reads the order from the gateway: the signature proves
// who sent the webhook, the query proves the payment settled. GraphQL
// states amounts in minor units, and only the pre-tax subtotal is
// comparable to the order.
func (g *waffoGateway) ConfirmPayment(ctx context.Context, o *order.Order, notice *Notice) error {
	gatewayOrder, err := g.client.GetOrder(ctx, notice.TradeNo)
	if err != nil {
		return err
	}
	if gatewayOrder.ID != notice.TradeNo {
		return errors.New("gateway order id mismatch")
	}
	if gatewayOrder.MerchantExternalID != o.OrderNo {
		return errors.New("gateway order number mismatch")
	}
	if gatewayOrder.TestMode != g.config.TestMode {
		return errors.New("gateway order environment mismatch")
	}
	if !gatewayOrder.Completed() {
		return errors.New("gateway order is not completed")
	}
	if !strings.EqualFold(gatewayOrder.Currency, o.PaymentCurrency) {
		return errors.New("gateway order currency mismatch")
	}
	subtotal, err := waffo.ParseMinorUnits(gatewayOrder.Subtotal.Amount)
	if err != nil {
		return errors.New("gateway order subtotal is invalid")
	}
	if subtotal != o.PaymentAmount {
		return errors.New("gateway order subtotal does not match payment expectation")
	}
	return nil
}

// SyncWaffoWebhook points the store of a Waffo payment method at its
// callback URL and returns the configuration to store, with the gateway's
// webhook id recorded in it. stored is the method's configuration before
// the change, whose webhook id is kept so the same endpoint is updated
// instead of a second one added; it is empty for a new method.
func (r *Registry) SyncWaffoWebhook(ctx context.Context, config, stored, callbackURL string) (string, error) {
	var updated paymentEntity.WaffoConfig
	if err := updated.Unmarshal([]byte(config)); err != nil {
		return "", fmt.Errorf("decode Waffo configuration: %w", err)
	}
	updated.WebhookID = WaffoWebhookID(stored)
	client, err := newWaffoClient(r, updated)
	if err != nil {
		return "", err
	}
	if updated.WebhookID, err = client.RegisterWebhook(ctx, updated.WebhookID, callbackURL); err != nil {
		return "", err
	}
	content, err := updated.Marshal()
	return string(content), err
}

// WaffoWebhookID is the webhook id recorded in a stored Waffo configuration;
// empty when there is none.
func WaffoWebhookID(stored string) string {
	if stored == "" {
		return ""
	}
	var config paymentEntity.WaffoConfig
	if err := config.Unmarshal([]byte(stored)); err != nil {
		return ""
	}
	return config.WebhookID
}

// WithWaffoWebhookID returns config with the webhook id of stored carried
// over, so a saved configuration compares equal to the stored one when only
// the gateway-assigned id differs.
func WithWaffoWebhookID(config, stored string) (string, error) {
	var updated paymentEntity.WaffoConfig
	if err := updated.Unmarshal([]byte(config)); err != nil {
		return "", err
	}
	updated.WebhookID = WaffoWebhookID(stored)
	content, err := updated.Marshal()
	return string(content), err
}
