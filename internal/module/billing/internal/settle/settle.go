// Package settle holds the one settlement primitive shared by the checkout
// and payment-callback subdomains: marking a gateway-verified payment as paid
// and enqueueing activation. A single implementation keeps the callback and
// expiry-time settlement semantics from drifting apart.
package settle

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/pkg/errors"
)

// Order status values of the billing state machine.
const (
	StatusPending  = uint8(1)
	StatusPaid     = uint8(2)
	StatusFinished = uint8(5)
)

// Queue schedules order activation; the module's order queue port satisfies it.
type Queue interface {
	EnqueueActivation(ctx context.Context, orderNo string) error
}

// ValidateTradeNo rejects malformed or hostile gateway trade numbers.
func ValidateTradeNo(tradeNo string) error {
	if tradeNo == "" || len(tradeNo) > 255 || strings.TrimSpace(tradeNo) != tradeNo || !utf8.ValidString(tradeNo) {
		return errors.New("invalid trade number")
	}
	for _, char := range tradeNo {
		if char < 0x20 || char == 0x7f {
			return errors.New("invalid trade number")
		}
	}
	return nil
}

// VerifiedPayment implements settlement idempotency. A settlement may only
// perform Pending -> Paid. A retry for an already-paid order may recreate a
// previously failed queue insertion, while the deterministic activation task
// ID prevents concurrent settlements from activating the order twice. Callers
// must authenticate the gateway response and verify the order amount first.
func VerifiedPayment(ctx context.Context, orders repository.OrderRepo, queue Queue, orderInfo *order.Order, tradeNo string) error {
	if err := ValidateTradeNo(tradeNo); err != nil {
		return err
	}
	if orderInfo.TradeNo != "" && orderInfo.TradeNo != tradeNo {
		return errors.New("order trade number mismatch")
	}

	switch orderInfo.Status {
	case StatusFinished:
		return nil
	case StatusPaid:
		// A prior settlement may have committed the database update but failed
		// to contact Redis. Re-enqueue below so retries heal that partial
		// failure.
	case StatusPending:
		updated, err := orders.MarkOrderPaid(ctx, orderInfo.OrderNo, tradeNo)
		if err != nil {
			return err
		}
		if !updated {
			latest, err := orders.FindOneByOrderNo(ctx, orderInfo.OrderNo)
			if err != nil {
				return err
			}
			if latest.TradeNo != "" && latest.TradeNo != tradeNo {
				return errors.New("order trade number mismatch")
			}
			if latest.Status == StatusFinished {
				return nil
			}
			if latest.Status != StatusPaid {
				return errors.Errorf("invalid order status transition: %d", latest.Status)
			}
		}
	default:
		return errors.Errorf("invalid order status transition: %d", orderInfo.Status)
	}

	return queue.EnqueueActivation(ctx, orderInfo.OrderNo)
}
