// Package checkout implements the user-facing money flows of the billing
// module: purchase, renewal, traffic reset, recharge, order preview and
// close. Only the module facade may reach it.
package checkout

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
	subscribeEntity "github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// Order lifecycle constants shared with the V2 orchestration layer via the
// module facade.
const (
	CloseOrderTimeMinutes = 15

	// MaxOrderAmount Order amount limits
	MaxOrderAmount    = 2147483647 // int32 max value (2.1 billion)
	MaxRechargeAmount = 2000000000 // 2 billion, slightly lower for safety
	MinRechargeAmount = 100        // minimum recharge amount in minor currency units
	MaxQuantity       = 1000       // Maximum quantity per order
)

// PlanReader is the module's port onto the subscription domain's plan
// catalogue; the legacy subscribe repository satisfies it structurally.
type PlanReader interface {
	FindOne(ctx context.Context, id int64) (*subscribeEntity.Subscribe, error)
}

// UserSubscriptionReader is the module's port onto the subscription domain's
// user subscriptions; the legacy user-subscription repository satisfies it
// structurally.
type UserSubscriptionReader interface {
	HasBlockingSubscription(ctx context.Context, userID int64) (bool, error)
	CountQuotaConsumingSubscriptions(ctx context.Context, userID, subscribeID int64) (int64, error)
	FindOneUserSubscribe(ctx context.Context, id int64) (*usersub.SubscribeDetails, error)
	FindOneSubscribe(ctx context.Context, id int64) (*usersub.Subscribe, error)
}

// OrderQueue mirrors the facade's order queue port.
type OrderQueue interface {
	EnqueueActivation(ctx context.Context, orderNo string) error
	EnqueueDeferredClose(ctx context.Context, orderNo string) error
}

// Store exposes billing transactions, wallet access and the inbox. Inventory
// operations use their own injected subscription capability.
type Store interface {
	InBillingTx(ctx context.Context, fn func(repository.BillingStore) error) error
	Inbox() repository.InboxRepo
	Wallet() repository.WalletRepo
}

type Inventory interface {
	Reserve(context.Context, string, int64) error
	Restore(context.Context, string, int64) error
}

type Deps struct {
	Orders    repository.OrderRepo
	Coupons   repository.CouponRepo
	Payments  repository.PaymentRepo
	Plans     PlanReader
	UserSubs  UserSubscriptionReader
	Store     Store
	Inventory Inventory
	Queue     OrderQueue
	// SingleModel forbids holding more than one blocking subscription;
	// read per request because the admin can change it at runtime.
	SingleModel func() bool
	// CurrencyUnit is the site currency used for gateway verification;
	// read per request because the admin can change it at runtime.
	CurrencyUnit func() string
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func getDiscount(discounts []dto.BillingSubscribeDiscount, inputMonths int64) float64 {
	var finalDiscount float64 = 100

	for _, discount := range discounts {
		if discount.Quantity > 0 && discount.Discount >= 0 && discount.Discount <= 100 && inputMonths >= discount.Quantity && discount.Discount < finalDiscount {
			finalDiscount = discount.Discount
		}
	}

	return finalDiscount / float64(100)
}

func ensureCouponEnabled(couponInfo *coupon.Coupon) error {
	if !couponInfo.IsEnabled() {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponDisabled), "coupon disabled")
	}
	// Coupon start/expire times are stored as Unix milliseconds; comparing
	// them against seconds made every coupon with a start time permanently
	// "not active".
	now := timeutil.Now().UnixMilli()
	if couponInfo.StartTime > 0 && now < couponInfo.StartTime {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponNotApplicable), "coupon is not active")
	}
	if couponInfo.ExpireTime <= 0 || now > couponInfo.ExpireTime {
		return errors.Wrapf(xerr.NewErrCode(xerr.CouponExpired), "coupon expired")
	}
	return nil
}

func ensurePaymentAvailable(paymentInfo *paymentEntity.Payment) error {
	if paymentInfo == nil || paymentInfo.Enable == nil || !*paymentInfo.Enable || payment.ParsePlatform(paymentInfo.Platform) == payment.UNSUPPORTED {
		return errors.Wrapf(xerr.NewErrCode(xerr.PaymentMethodNotFound), "payment method is unavailable")
	}
	return nil
}

func calculateCoupon(amount int64, couponInfo *coupon.Coupon) int64 {
	if amount <= 0 || couponInfo == nil || couponInfo.Discount < 0 {
		return 0
	}
	if couponInfo.Type == 1 {
		if couponInfo.Discount > 100 {
			return amount
		}
		return int64(float64(amount) * (float64(couponInfo.Discount) / float64(100)))
	}
	return min(couponInfo.Discount, amount)
}

func calculateFee(amount int64, config *paymentEntity.Payment) int64 {
	if amount <= 0 || config == nil || config.FeePercent < 0 || config.FeeAmount < 0 {
		return 0
	}
	var fee float64
	switch config.FeeMode {
	case 0:
		return 0
	case 1:
		fee = float64(amount) * (float64(config.FeePercent) / float64(100))
	case 2:
		if amount > 0 {
			fee = float64(config.FeeAmount)
		}
	case 3:
		fee = float64(amount)*(float64(config.FeePercent)/float64(100)) + float64(config.FeeAmount)
	}
	if fee < 0 {
		return 0
	}
	return int64(fee)
}

// settleVerifiedPayment marks a gateway-verified payment as paid and enqueues
// activation. Callers must authenticate the gateway response and verify the
// order amount before invoking it. The committed Paid state is the durable
// outbox: an enqueue failure is repaired by paid-order reconciliation.
func (s *Service) settleVerifiedPayment(ctx context.Context, orderInfo *orderEntity.Order, tradeNo string) error {
	return settle.VerifiedPayment(ctx, s.deps.Orders, s.deps.Queue, orderInfo, tradeNo)
}
