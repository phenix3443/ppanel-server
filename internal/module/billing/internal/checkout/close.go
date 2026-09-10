package checkout

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	payment2 "github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/alipay"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/cryptomus"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/epay"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/stripe"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/pkg/errors"
)

const orderTypeSubscribe uint8 = 1

// ErrGatewayUnconfirmed reports that a gateway order could not be confirmed
// safe to close, so the order intentionally stays pending.
// Schedulers treat it as an expected outcome, not a per-order failure.
var ErrGatewayUnconfirmed = stderrors.New("gateway could not confirm the order as paid")

// Close closes a pending order: the billing transaction releases the coupon
// reservation and refunds the gift deduction, then the reserved plan
// inventory returns in its own subscription-domain transaction (ADR-001
// step 2). Orders whose gateway checkout already collected money are settled
// instead of closed.
func (s *Service) Close(ctx context.Context, req *dto.CloseOrderRequest) error {
	log := logger.WithContext(ctx)
	// Find order information by order number
	orderInfo, err := s.deps.Orders.FindOneByOrderNo(ctx, req.OrderNo)
	if err != nil {
		log.Errorw("[CloseOrder] Find order info failed",
			logger.Field("error", err.Error()),
			logger.Field("orderNo", req.OrderNo),
		)
		return nil
	}
	// Public callers are authenticated by the route. Queue workers use a
	// context without a user and are the only internal callers allowed to close
	// any expired order.
	currentUser, userInitiated := ctx.Value(requestctx.CtxKeyUser).(*user.User)
	userInitiated = userInitiated && currentUser != nil
	if userInitiated && orderInfo.UserId != currentUser.Id {
		return errors.New("order does not belong to the current user")
	}
	// If the order status is not 1, it means that the order has been closed or paid
	if orderInfo.Status != 1 {
		log.Infow("[CloseOrder] Order status is not 1",
			logger.Field("orderNo", req.OrderNo),
			logger.Field("status", orderInfo.Status),
		)
		if orderInfo.Status == 3 {
			// Resume a restoration lost between the close commit and the
			// inventory transaction; RestoreInventoryOnce no-ops when the
			// order never reserved or already restored.
			return s.restoreReservedInventory(ctx, orderInfo)
		}
		return nil
	}
	settled, err := s.settleOrCancelGatewayOrder(ctx, orderInfo, userInitiated)
	if err != nil {
		return err
	}
	if settled {
		return nil
	}

	var closed bool
	err = s.deps.Store.InBillingTx(ctx, func(txStore repository.BillingStore) error {
		if payment2.ParsePlatform(orderInfo.Method) == payment2.Cryptomus {
			// Checkout persists its payment expectation before creating an
			// invoice. Serialize this recheck with that write so a "checkout
			// never started" snapshot cannot close an in-flight invoice.
			current, err := txStore.Order().FindOneByOrderNoForUpdate(ctx, req.OrderNo)
			if err != nil {
				return err
			}
			if current.Status != 1 {
				return nil
			}
			if current.PaymentCurrency != orderInfo.PaymentCurrency || current.PaymentAmount != orderInfo.PaymentAmount ||
				current.TradeNo != orderInfo.TradeNo || current.PaymentId != orderInfo.PaymentId || current.Method != orderInfo.Method {
				return fmt.Errorf("Cryptomus checkout changed during cancellation: %w", ErrGatewayUnconfirmed)
			}
		}
		// Only the still-pending order may be closed.  A payment callback can
		// race this task, so an unconditional status write would otherwise turn
		// a paid order back into a closed order.
		closed, err = txStore.Order().UpdateOrderStatusFrom(ctx, req.OrderNo, 1, 3)
		if err != nil {
			log.Errorw("[CloseOrder] Update order status failed",
				logger.Field("error", err.Error()),
				logger.Field("orderNo", req.OrderNo),
			)
			return err
		}
		if !closed {
			return nil
		}
		if orderInfo.Coupon != "" && orderInfo.CouponReserved {
			if err := txStore.Coupon().ReleaseUsage(ctx, orderInfo.Coupon); err != nil {
				return err
			}
		}
		// Keep closed guest orders for payment audit and reconciliation.  Deleting
		// them used to discard evidence of a late provider payment and, because
		// of the early return, also skipped restoration of reserved inventory.
		// refund deduction amount to user deduction balance
		if orderInfo.GiftAmount > 0 {
			userInfo, err := txStore.Wallet().FindOneForUpdate(ctx, orderInfo.UserId)
			if err != nil {
				log.Errorw("[CloseOrder] Find user info failed",
					logger.Field("error", err.Error()),
					logger.Field("user_id", orderInfo.UserId),
				)
				return err
			}
			deduction := userInfo.GiftAmount + orderInfo.GiftAmount
			userInfo.GiftAmount = deduction
			err = txStore.Wallet().UpdateBalanceFields(ctx, userInfo)
			if err != nil {
				log.Errorw("[CloseOrder] Refund deduction amount failed",
					logger.Field("error", err.Error()),
					logger.Field("uid", orderInfo.UserId),
					logger.Field("deduction", orderInfo.GiftAmount),
				)
				return err
			}
			// Record the deduction refund log
			giftLog := logEntity.Gift{
				Type:        logEntity.GiftTypeIncrease,
				OrderNo:     orderInfo.OrderNo,
				SubscribeId: 0,
				Amount:      orderInfo.GiftAmount,
				Balance:     deduction,
				Remark:      "Order cancellation refund",
				Timestamp:   timeutil.Now().UnixMilli(),
			}
			content, _ := giftLog.Marshal()

			err = txStore.Log().Insert(ctx, &logEntity.SystemLog{
				Id:       0,
				Type:     logEntity.TypeGift.Uint8(),
				Date:     timeutil.Now().Format(time.DateOnly),
				ObjectID: userInfo.UserId,
				Content:  string(content),
			})
			if err != nil {
				log.Errorw("[CloseOrder] Record cancellation refund log failed",
					logger.Field("error", err.Error()),
					logger.Field("uid", orderInfo.UserId),
					logger.Field("deduction", orderInfo.GiftAmount),
				)
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Errorf("[CloseOrder] Transaction failed: %v", err.Error())
		return err
	}
	if !closed {
		return nil
	}
	// The reserved plan inventory returns in its own subscription-domain
	// transaction (ADR-001 step 2). A crash before this point is resumed by
	// the retried close task via the status==3 branch above.
	return s.restoreReservedInventory(ctx, orderInfo)
}

// restoreReservedInventory returns the closed order's reserved inventory
// unit. Only new subscription purchases reserve plan inventory; renewals and
// traffic resets reference a plan too, but never consumed stock, and the
// reserve marker check inside RestoreInventoryOnce keeps them (and stock-out
// compensation closes) from adding stock that was never taken.
func (s *Service) restoreReservedInventory(ctx context.Context, orderInfo *order.Order) error {
	if orderInfo.Type != orderTypeSubscribe || orderInfo.SubscribeId <= 0 {
		return nil
	}
	if err := s.deps.Inventory.Restore(ctx, orderInfo.OrderNo, orderInfo.SubscribeId); err != nil {
		logger.WithContext(ctx).Errorw("[CloseOrder] Restore subscribe inventory failed",
			logger.Field("error", err.Error()),
			logger.Field("subscribeId", orderInfo.SubscribeId),
			logger.Field("orderNo", orderInfo.OrderNo),
		)
		return err
	}
	return nil
}

// settleOrCancelGatewayOrder ensures that closing locally cannot leave an
// active provider checkout able to charge the user after stock and coupons
// have been released. userInitiated selects the legacy owner-cancellation
// policy for EPay/Alipay; Cryptomus requires confirmation for every caller.
func (s *Service) settleOrCancelGatewayOrder(ctx context.Context, orderInfo *order.Order, userInitiated bool) (bool, error) {
	switch payment2.ParsePlatform(orderInfo.Method) {
	case payment2.Stripe:
		return s.settleOrCancelStripeOrder(ctx, orderInfo)
	case payment2.EPay:
		return s.settleEPayOrder(ctx, orderInfo, userInitiated)
	case payment2.AlipayF2F:
		return s.settleAlipayOrder(ctx, orderInfo, userInitiated)
	case payment2.Cryptomus:
		return s.settleCryptomusOrder(ctx, orderInfo)
	default:
		return false, nil
	}
}

func (s *Service) settleOrCancelStripeOrder(ctx context.Context, orderInfo *order.Order) (bool, error) {
	if orderInfo.TradeNo == "" {
		return false, nil
	}
	paymentConfig, err := s.deps.Payments.FindOne(ctx, orderInfo.PaymentId)
	if err != nil {
		return false, err
	}
	config := payment.StripeConfig{}
	if err := json.Unmarshal([]byte(paymentConfig.Config), &config); err != nil {
		return false, err
	}
	client := stripe.NewClient(stripe.Config{
		PublicKey:     config.PublicKey,
		SecretKey:     config.SecretKey,
		WebhookSecret: config.WebhookSecret,
	})
	stripeOrder := &stripe.Order{
		OrderNo:   orderInfo.OrderNo,
		Subscribe: "", // subscribe metadata is informational; immutable payment fields below are authoritative.
		Amount:    orderInfo.Amount,
		Currency:  s.deps.CurrencyUnit(),
		Payment:   config.Payment,
	}
	paid, err := client.VerifyPaymentIntent(stripeOrder, orderInfo.TradeNo)
	if err != nil {
		return false, err
	}
	if paid {
		if err := s.settleVerifiedPayment(ctx, orderInfo, orderInfo.TradeNo); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := client.CancelPaymentIntent(orderInfo.TradeNo); err == nil {
		return false, nil
	}

	// A payment can finish between the status query and cancellation.  Recheck
	// once so that case is settled rather than closed locally.
	paid, err = client.VerifyPaymentIntent(stripeOrder, orderInfo.TradeNo)
	if err != nil {
		return false, err
	}
	if !paid {
		return false, fmt.Errorf("cancel Stripe payment intent %s failed", orderInfo.TradeNo)
	}
	if err := s.settleVerifiedPayment(ctx, orderInfo, orderInfo.TradeNo); err != nil {
		return false, err
	}
	return true, nil
}

// Cryptomus invoices cannot be cancelled through the API, but they expire on
// their own after the checkout lifetime and report a final state. Closing is
// safe only when the gateway confirms no money was collected: a paid invoice
// is settled instead. Active, underpaid, AML-frozen and refund invoices stay
// pending for reconciliation or manual resolution. A user's cancellation
// request cannot invalidate the gateway invoice or forfeit received funds.
func (s *Service) settleCryptomusOrder(ctx context.Context, orderInfo *order.Order) (bool, error) {
	if orderInfo.PaymentCurrency == "" && orderInfo.TradeNo == "" {
		return false, nil // checkout was never started; safe to close.
	}
	paymentConfig, err := s.deps.Payments.FindOne(ctx, orderInfo.PaymentId)
	if err != nil {
		return false, err
	}
	config := payment.CryptomusConfig{}
	if err := json.Unmarshal([]byte(paymentConfig.Config), &config); err != nil {
		return false, err
	}
	client := cryptomus.NewClient(cryptomus.Config{MerchantID: config.MerchantID, APIKey: config.APIKey})
	// The trade number is claimed right after invoice creation, but a checkout
	// may have crashed between the two steps; the order-number lookup still
	// finds the invoice the gateway holds for this order.
	invoice, err := client.GetInvoice(orderInfo.TradeNo, orderInfo.OrderNo)
	if err != nil {
		// Even an explicit payment-not-found answer cannot close a started
		// checkout: invoice creation may still be in flight after a timeout,
		// before its UUID was persisted locally.
		return false, fmt.Errorf("cannot safely expire Cryptomus order %s: %v: %w", orderInfo.OrderNo, err, ErrGatewayUnconfirmed)
	}
	// Validate identity and the immutable amount before trusting any state,
	// including a cancellation that would release stock and wallet deductions.
	amount, err := cryptomus.ParseMoney(invoice.Amount)
	if err != nil || invoice.OrderNo != orderInfo.OrderNo ||
		(orderInfo.TradeNo != "" && invoice.UUID != orderInfo.TradeNo) ||
		amount != orderInfo.PaymentAmount || !strings.EqualFold(invoice.Currency, orderInfo.PaymentCurrency) {
		return false, fmt.Errorf("Cryptomus order %s query does not match payment expectation: %w", orderInfo.OrderNo, ErrGatewayUnconfirmed)
	}
	if invoice.Paid() {
		if err := s.settleVerifiedPayment(ctx, orderInfo, invoice.UUID); err != nil {
			return false, err
		}
		return true, nil
	}
	// is_final alone does not mean unpaid (e.g. refund_fail). Only an
	// explicitly cancelled invoice with zero received funds is safe to close.
	paidAmount, amountErr := cryptomus.ParseMoney(invoice.PaymentAmount)
	if invoice.IsFinal && invoice.State() == cryptomus.StatusCancel && amountErr == nil && paidAmount == 0 {
		return false, nil
	}
	return false, fmt.Errorf("cannot safely expire Cryptomus order %s with invoice status %q: %w", orderInfo.OrderNo, invoice.State(), ErrGatewayUnconfirmed)
}

// The gateway creates an Alipay face-to-face trade only when the buyer scans
// the QR code, so a missing trade proves no money was collected and the close
// may proceed. Any existing trade must be reconciled before the local close
// releases stock and coupons: a paid trade is settled instead of cancelled —
// a lost payment notification would otherwise void an order the customer
// already paid for — and a scanned-but-unpaid trade is closed at the gateway
// first so its QR code cannot collect money afterwards.
func (s *Service) settleAlipayOrder(ctx context.Context, orderInfo *order.Order, userInitiated bool) (bool, error) {
	if orderInfo.PaymentCurrency == "" {
		return false, nil // checkout was never started; safe to close.
	}
	paymentConfig, err := s.deps.Payments.FindOne(ctx, orderInfo.PaymentId)
	if err != nil {
		return false, err
	}
	config := payment.AlipayF2FConfig{}
	if err := config.Unmarshal([]byte(paymentConfig.Config)); err != nil {
		return false, err
	}
	client := alipay.NewClient(alipay.Config{
		AppId:      config.AppId,
		PrivateKey: config.PrivateKey,
		PublicKey:  config.PublicKey,
		Sandbox:    config.Sandbox,
		Gateway:    config.Gateway,
	})
	if client == nil {
		return false, stderrors.New("initialize Alipay client failed")
	}
	trade, err := client.QueryTrade(ctx, orderInfo.OrderNo)
	if stderrors.Is(err, alipay.ErrTradeNotExist) {
		return false, nil // the QR code was never scanned; no money was collected.
	}
	if err != nil {
		if userInitiated {
			logger.WithContext(ctx).Infow("[CloseOrder] user-requested close of Alipay order without gateway confirmation",
				logger.Field("orderNo", orderInfo.OrderNo),
				logger.Field("queryError", err.Error()),
			)
			return false, nil
		}
		return false, fmt.Errorf("cannot safely expire Alipay order %s: %v: %w", orderInfo.OrderNo, err, ErrGatewayUnconfirmed)
	}
	if trade.Status.Paid() {
		return s.settleQueriedAlipayTrade(ctx, orderInfo, trade)
	}
	if trade.Status == alipay.Closed {
		return false, nil // the gateway already voided the trade without payment.
	}
	// WAIT_BUYER_PAY: the buyer scanned but has not paid. Void the trade so
	// the QR code cannot collect money after the local close releases stock
	// and coupons; the gateway rejects the close once the trade is paid.
	closeErr := client.CloseTrade(ctx, orderInfo.OrderNo)
	if closeErr == nil || stderrors.Is(closeErr, alipay.ErrTradeNotExist) {
		return false, nil
	}
	// A payment can finish between the query and the close attempt. Recheck
	// once so that case is settled rather than closed locally.
	trade, err = client.QueryTrade(ctx, orderInfo.OrderNo)
	if err == nil && trade.Status.Paid() {
		return s.settleQueriedAlipayTrade(ctx, orderInfo, trade)
	}
	if userInitiated {
		return false, nil // the owner explicitly forfeits the unconfirmed trade.
	}
	return false, fmt.Errorf("cannot safely expire Alipay order %s: gateway close failed: %v: %w", orderInfo.OrderNo, closeErr, ErrGatewayUnconfirmed)
}

// settleQueriedAlipayTrade settles an order whose trade the gateway reports
// as paid, after checking the signed query response against the payment
// expectation persisted at checkout time.
func (s *Service) settleQueriedAlipayTrade(ctx context.Context, orderInfo *order.Order, trade *alipay.Trade) (bool, error) {
	if trade.OrderNo != orderInfo.OrderNo || trade.Amount != orderInfo.PaymentAmount || trade.TradeNo == "" {
		return false, fmt.Errorf("Alipay order %s query does not match payment expectation", orderInfo.OrderNo)
	}
	if err := s.settleVerifiedPayment(ctx, orderInfo, trade.TradeNo); err != nil {
		return false, err
	}
	return true, nil
}

// EPay-compatible gateways have no standard cancellation API. Once a payment
// URL has been issued, retaining the pending reservation is safer than closing
// locally and accepting a later customer charge with no fulfillment. Gateways
// with an order-query endpoint are reconciled here; unsupported or unavailable
// gateways remain pending for retry/manual resolution instead of losing funds.
// A user-initiated cancellation is the exception: the owner explicitly gives
// up the order, so absent any evidence of payment the close proceeds. A late
// callback on the closed order is still rejected and leaves an audit trail.
func (s *Service) settleEPayOrder(ctx context.Context, orderInfo *order.Order, userInitiated bool) (bool, error) {
	if orderInfo.PaymentCurrency == "" {
		return false, nil // checkout was never started; safe to close.
	}
	paymentConfig, err := s.deps.Payments.FindOne(ctx, orderInfo.PaymentId)
	if err != nil {
		return false, err
	}
	config := payment.EPayConfig{}
	if err := json.Unmarshal([]byte(paymentConfig.Config), &config); err != nil {
		return false, err
	}
	result, err := epay.NewClient(config.Pid, config.Url, config.Key, config.Type).QueryOrder(orderInfo.OrderNo)
	if err != nil {
		if userInitiated {
			logger.WithContext(ctx).Infow("[CloseOrder] user-requested close of EPay order without gateway confirmation",
				logger.Field("orderNo", orderInfo.OrderNo),
				logger.Field("queryError", err.Error()),
			)
			return false, nil
		}
		return false, fmt.Errorf("cannot safely expire EPay order %s: %v: %w", orderInfo.OrderNo, err, ErrGatewayUnconfirmed)
	}
	if !result.Paid {
		if userInitiated {
			return false, nil
		}
		return false, fmt.Errorf("cannot safely expire unpaid EPay order %s; gateway does not provide cancellation: %w", orderInfo.OrderNo, ErrGatewayUnconfirmed)
	}
	if result.StatusOnly {
		return false, fmt.Errorf("cannot safely reconcile paid EPay order %s: gateway query has no transaction details", orderInfo.OrderNo)
	}
	amount, err := epay.ParseMoney(result.Money)
	if err != nil || result.OrderNo != orderInfo.OrderNo || result.MerchantID != config.Pid || result.Type != config.Type || amount != orderInfo.PaymentAmount || result.TradeNo == "" {
		return false, fmt.Errorf("EPay order %s query does not match payment expectation", orderInfo.OrderNo)
	}
	if err := s.settleVerifiedPayment(ctx, orderInfo, result.TradeNo); err != nil {
		return false, err
	}
	return true, nil
}
