package callbacks

import (
	"context"
	"strings"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/cryptomus"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// cryptomusBaseURL is empty in production, so the client always talks to the
// official gateway. Only tests override it. The gateway re-query below is the
// authoritative payment confirmation, so redirecting it must never be
// possible through database configuration.
var cryptomusBaseURL = ""

// CryptomusNotify authenticates and settles a Cryptomus webhook. The payload
// signature proves the gateway sent it; the settlement is still re-confirmed
// against the gateway's payment-info API before money is accepted, mirroring
// the EPay callback's defense in depth.
func (s *Service) CryptomusNotify(ctx context.Context, payload []byte) error {
	l := logger.WithContext(ctx)
	data, ok := ctx.Value(requestctx.CtxKeyPayment).(*payment.Payment)
	if !ok {
		l.Error("[CryptomusNotify] Payment not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "payment config not found")
	}
	var config payment.CryptomusConfig
	if err := config.Unmarshal([]byte(data.Config)); err != nil {
		l.Errorw("[CryptomusNotify] Unmarshal config failed", logger.Field("error", err.Error()))
		return err
	}
	if config.MerchantID == "" || config.APIKey == "" {
		return errors.New("incomplete payment configuration")
	}
	client := cryptomus.NewClient(cryptomus.Config{MerchantID: config.MerchantID, APIKey: config.APIKey, BaseURL: cryptomusBaseURL})
	if !client.VerifyNotificationSign(payload) {
		l.Error("[CryptomusNotify] Verify sign failed")
		return errors.New("verify sign failed")
	}
	notification, err := cryptomus.ParseNotification(payload)
	if err != nil {
		return err
	}
	callbackAmount, err := validateCryptomusNotification(notification)
	if err != nil {
		l.Error("[CryptomusNotify] Callback validation failed",
			logger.Field("orderNo", notification.OrderNo),
			logger.Field("status", notification.Status),
			logger.Field("error", err.Error()),
		)
		return err
	}

	orderInfo, err := s.orders.FindOneByOrderNo(ctx, notification.OrderNo)
	if err != nil {
		l.Error("[CryptomusNotify] Find order failed", logger.Field("error", err.Error()), logger.Field("orderNo", notification.OrderNo))
		return errors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order not exist: %v", notification.OrderNo)
	}
	if err := validateOrderPayment(orderInfo, data); err != nil {
		l.Error("[CryptomusNotify] Order payment binding failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}
	if orderInfo.TradeNo != "" && orderInfo.TradeNo != notification.UUID {
		return errors.New("order trade number mismatch")
	}
	if !cryptomus.PaidStatus(notification.Status) {
		if err := validatePaymentExpectation(orderInfo, callbackAmount, notification.Currency); err != nil {
			return err
		}
		// A valid lifecycle notification is not a failed payment callback.
		// Acknowledge it without settling or downgrading the local order, even
		// when delivery is out of order or a cancelled order is already closed.
		fields := []logger.LogField{
			logger.Field("orderNo", notification.OrderNo),
			logger.Field("tradeNo", notification.UUID),
			logger.Field("status", notification.Status),
			logger.Field("order_status", orderInfo.Status),
			logger.Field("is_final", notification.IsFinal),
			logger.Field("payment_amount", notification.PaymentAmount),
			logger.Field("payer_currency", notification.PayerCurrency),
		}
		switch notification.Status {
		case cryptomus.StatusWrongAmount, cryptomus.StatusLocked,
			cryptomus.StatusRefundProcess, cryptomus.StatusRefundFail, cryptomus.StatusRefundPaid:
			l.Errorw("[CryptomusNotify] Payment requires manual review", append(fields, logger.Field("requires_manual_review", true))...)
		default:
			l.Infow("[CryptomusNotify] Payment status received without settlement", fields...)
		}
		return nil
	}
	if finished, err := finishedOrderDuplicate(ctx, orderInfo, notification.UUID); err != nil {
		return err
	} else if finished {
		return nil
	}
	if err := validateOrderCanSettle(orderInfo); err != nil {
		return err
	}
	if err := validatePaymentExpectation(orderInfo, callbackAmount, notification.Currency); err != nil {
		l.Error("[CryptomusNotify] Payment amount validation failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}

	invoice, err := client.GetInvoice(notification.UUID, "")
	if err != nil {
		l.Error("[CryptomusNotify] Gateway invoice query failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}
	if err := validateQueriedCryptomusInvoice(invoice, notification, orderInfo); err != nil {
		l.Error("[CryptomusNotify] Gateway invoice validation failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}

	if err := s.settle(ctx, orderInfo, notification.UUID); err != nil {
		l.Error("[CryptomusNotify] Settle order failed", logger.Field("orderNo", notification.OrderNo), logger.Field("error", err.Error()))
		return err
	}
	l.Info("[CryptomusNotify] Notify processed", logger.Field("orderNo", notification.OrderNo))
	return nil
}

func validateCryptomusNotification(notification *cryptomus.Notification) (int64, error) {
	// The gateway also emits wallet-topup webhooks with the same shape; only
	// invoice payments may settle orders.
	if notification.Type != "" && notification.Type != "payment" {
		return 0, errors.New("unsupported notification type")
	}
	if notification.OrderNo == "" || len(notification.OrderNo) > 255 || strings.TrimSpace(notification.OrderNo) != notification.OrderNo {
		return 0, errors.New("invalid order number")
	}
	if err := settle.ValidateTradeNo(notification.UUID); err != nil {
		return 0, err
	}
	if !cryptomus.KnownStatus(notification.Status) {
		return 0, errors.New("unknown payment status")
	}
	amount, err := cryptomus.ParseMoney(notification.Amount)
	if err != nil {
		return 0, errors.New("invalid callback amount")
	}
	return amount, nil
}

func validateQueriedCryptomusInvoice(invoice *cryptomus.Invoice, notification *cryptomus.Notification, orderInfo *order.Order) error {
	if invoice == nil || !invoice.Paid() {
		return errors.New("gateway invoice is not paid")
	}
	if invoice.UUID != notification.UUID || invoice.OrderNo != orderInfo.OrderNo {
		return errors.New("gateway invoice identity mismatch")
	}
	amount, err := cryptomus.ParseMoney(invoice.Amount)
	if err != nil || amount != orderInfo.PaymentAmount || !strings.EqualFold(invoice.Currency, orderInfo.PaymentCurrency) {
		return errors.New("gateway invoice amount mismatch")
	}
	return nil
}
