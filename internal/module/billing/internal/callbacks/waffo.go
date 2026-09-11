package callbacks

import (
	"context"
	"strings"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/waffo"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// waffoBaseURL is empty in production, so the client always talks to the
// official gateway. Only tests override it. The gateway re-query below is the
// authoritative payment confirmation, so redirecting it must never be
// possible through database configuration.
var waffoBaseURL = ""

// WaffoNotify authenticates and settles a Waffo webhook. The signature proves
// the gateway sent the payload; the settlement is still re-confirmed against
// the gateway's order query before money is accepted, mirroring the Cryptomus
// callback's defense in depth.
//
// PPanel sells one-time products through Waffo, so order.completed is the only
// event that settles an order. Every other documented event is acknowledged
// and logged without touching the local order.
func (s *Service) WaffoNotify(ctx context.Context, payload []byte, signature string) error {
	l := logger.WithContext(ctx)
	data, ok := ctx.Value(requestctx.CtxKeyPayment).(*payment.Payment)
	if !ok {
		l.Error("[WaffoNotify] Payment not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "payment config not found")
	}
	var config payment.WaffoConfig
	if err := config.Unmarshal([]byte(data.Config)); err != nil {
		l.Errorw("[WaffoNotify] Unmarshal config failed", logger.Field("error", err.Error()))
		return err
	}
	client, err := waffo.NewClient(waffo.Config{
		MerchantID:  config.MerchantID,
		PrivateKey:  config.PrivateKey,
		StoreID:     config.StoreID,
		ProductID:   config.ProductID,
		TaxCategory: config.TaxCategory,
		TestMode:    config.TestMode,
		BaseURL:     waffoBaseURL,
	})
	if err != nil {
		l.Errorw("[WaffoNotify] Build client failed", logger.Field("error", err.Error()))
		return err
	}
	notification, err := client.VerifyNotification(payload, signature)
	if err != nil {
		l.Errorw("[WaffoNotify] Verify signature failed", logger.Field("error", err.Error()))
		return errors.New("verify sign failed")
	}
	// The signature only proves the payload came from Waffo, not from the
	// environment this payment method is configured for. A test-mode event
	// settling a production order would hand out paid subscriptions for free.
	if notification.TestMode != config.TestMode {
		l.Errorw("[WaffoNotify] Environment mismatch",
			logger.Field("eventTestMode", notification.TestMode),
			logger.Field("configTestMode", config.TestMode),
		)
		return errors.New("webhook environment mismatch")
	}
	if notification.EventType != waffo.EventOrderCompleted {
		l.Infow("[WaffoNotify] Event received without settlement",
			logger.Field("eventId", notification.EventID),
			logger.Field("eventType", notification.EventType),
			logger.Field("orderNo", notification.OrderNo),
		)
		return nil
	}
	if notification.OrderNo == "" {
		l.Errorw("[WaffoNotify] Event carries no PPanel order number", logger.Field("eventId", notification.EventID))
		return errors.New("callback has no order number")
	}
	callbackAmount, err := waffo.ParseMoney(notification.Amount)
	if err != nil {
		l.Errorw("[WaffoNotify] Callback amount invalid",
			logger.Field("orderNo", notification.OrderNo),
			logger.Field("amount", notification.Amount),
			logger.Field("error", err.Error()),
		)
		return err
	}

	orderInfo, err := s.orders.FindOneByOrderNo(ctx, notification.OrderNo)
	if err != nil {
		l.Errorw("[WaffoNotify] Find order failed", logger.Field("error", err.Error()), logger.Field("orderNo", notification.OrderNo))
		return errors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order not exist: %v", notification.OrderNo)
	}
	if err := validateOrderPayment(orderInfo, data); err != nil {
		l.Errorw("[WaffoNotify] Order payment binding failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}
	if finished, err := finishedOrderDuplicate(ctx, orderInfo, notification.OrderID); err != nil {
		return err
	} else if finished {
		return nil
	}
	if err := validateOrderCanSettle(orderInfo); err != nil {
		// The buyer can open more than one checkout session for the same
		// order, and the gateway has no order-scoped lock to prevent paying
		// both. A second completed order is money taken for goods already
		// delivered, so it needs a human refund rather than a retry.
		l.Errorw("[WaffoNotify] Completed payment cannot settle this order",
			logger.Field("orderNo", notification.OrderNo),
			logger.Field("gatewayOrderId", notification.OrderID),
			logger.Field("order_status", orderInfo.Status),
			logger.Field("requires_manual_review", true),
			logger.Field("error", err.Error()),
		)
		return err
	}
	if err := validatePaymentExpectation(orderInfo, callbackAmount, notification.Currency); err != nil {
		l.Errorw("[WaffoNotify] Payment amount validation failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}

	gatewayOrder, err := client.GetOrder(ctx, notification.OrderID)
	if err != nil {
		l.Errorw("[WaffoNotify] Gateway order query failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}
	if err := validateQueriedWaffoOrder(gatewayOrder, notification, orderInfo, config.TestMode); err != nil {
		l.Errorw("[WaffoNotify] Gateway order validation failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}

	if err := s.settle(ctx, orderInfo, notification.OrderID); err != nil {
		l.Errorw("[WaffoNotify] Settle order failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}
	l.Info("[WaffoNotify] Notify processed", logger.Field("orderNo", notification.OrderNo))
	return nil
}

// validateQueriedWaffoOrder confirms the gateway's own record of the order
// agrees with the callback and with what the checkout expected to be paid.
// The subtotal is the pre-tax price PPanel set; the total the buyer paid is
// higher by the tax the merchant of record collects, so only the subtotal is
// comparable to the order amount. GraphQL states amounts in minor units,
// unlike the webhook's decimal strings.
func validateQueriedWaffoOrder(gatewayOrder *waffo.GatewayOrder, notification *waffo.Notification, orderInfo *order.Order, testMode bool) error {
	if gatewayOrder.ID != notification.OrderID {
		return errors.New("gateway order id mismatch")
	}
	if gatewayOrder.MerchantExternalID != orderInfo.OrderNo {
		return errors.New("gateway order number mismatch")
	}
	if gatewayOrder.TestMode != testMode {
		return errors.New("gateway order environment mismatch")
	}
	if !gatewayOrder.Completed() {
		return errors.New("gateway order is not completed")
	}
	if !strings.EqualFold(gatewayOrder.Currency, orderInfo.PaymentCurrency) {
		return errors.New("gateway order currency mismatch")
	}
	subtotal, err := waffo.ParseMinorUnits(gatewayOrder.Subtotal.Amount)
	if err != nil {
		return errors.New("gateway order subtotal is invalid")
	}
	if subtotal != orderInfo.PaymentAmount {
		return errors.New("gateway order subtotal does not match payment expectation")
	}
	return nil
}
