// Package callbacks implements the payment gateway callback subdomain of the
// billing module: it authenticates EPay/Stripe/Alipay/Cryptomus notifications,
// verifies them against the order's immutable payment expectation, re-confirms
// with the gateway and settles the payment. Only the module facade may reach it.
package callbacks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

type Service struct {
	orders repository.OrderRepo
	queue  settle.Queue
}

func NewService(orders repository.OrderRepo, queue settle.Queue) *Service {
	return &Service{orders: orders, queue: queue}
}

func (s *Service) settle(ctx context.Context, orderInfo *order.Order, tradeNo string) error {
	return settle.VerifiedPayment(ctx, s.orders, s.queue, orderInfo, tradeNo)
}

func validateOrderPayment(orderInfo *order.Order, paymentConfig *payment.Payment) error {
	if orderInfo.PaymentId != paymentConfig.Id {
		return errors.New("payment method mismatch")
	}
	if orderInfo.Method != paymentConfig.Platform {
		return errors.New("payment platform mismatch")
	}
	return nil
}

func validatePaymentExpectation(orderInfo *order.Order, amount int64, currency string) error {
	if orderInfo.PaymentCurrency == "" {
		return errors.New("payment amount snapshot is missing; restart checkout")
	}
	if orderInfo.PaymentAmount != amount {
		return errors.New("payment amount mismatch")
	}
	if !strings.EqualFold(orderInfo.PaymentCurrency, currency) {
		return errors.New("payment currency mismatch")
	}
	return nil
}

// finishedOrderDuplicate reports whether the order is already in the finished
// state and the incoming callback is a safe duplicate.
//
// Historical orders created before trade_no persistence was introduced may
// have an empty TradeNo field.  Blocking those retried callbacks would
// permanently prevent them from being acknowledged.  Instead, a warning is
// emitted so the gap can be audited, and the callback is treated as a known
// duplicate so the gateway stops retrying.
func finishedOrderDuplicate(ctx context.Context, orderInfo *order.Order, tradeNo string) (bool, error) {
	if orderInfo.Status != settle.StatusFinished {
		return false, nil
	}
	if err := settle.ValidateTradeNo(tradeNo); err != nil {
		return false, err
	}
	if orderInfo.TradeNo == "" {
		// Legacy order: trade_no was not persisted at payment time.
		// Warn for audit purposes and accept the duplicate gracefully.
		logger.WithContext(ctx).Infow("[finishedOrderDuplicate] finished order has no trade_no recorded; treating callback as duplicate",
			logger.Field("orderNo", orderInfo.OrderNo),
			logger.Field("incomingTradeNo", tradeNo),
		)
		return true, nil
	}
	if orderInfo.TradeNo != tradeNo {
		return false, errors.New("order trade number mismatch")
	}
	return true, nil
}

func validateOrderCanSettle(orderInfo *order.Order) error {
	if orderInfo.Status != settle.StatusPending && orderInfo.Status != settle.StatusPaid {
		return fmt.Errorf("invalid order status transition: %d", orderInfo.Status)
	}
	return nil
}
