// Package fulfillment applies a paid order's business effect: creating,
// renewing or traffic-resetting the user subscription in the fulfillment
// transaction, idempotent via the inbox marker. Only the module facade may
// reach it.
package fulfillment

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

// Order lifecycle constants mirrored from the billing domain's order rows.
const (
	OrderTypeSubscribe    = 1
	OrderTypeRenewal      = 2
	OrderTypeResetTraffic = 3

	inboxFulfillment = "subscription.fulfillment"
)

// ErrInvalidOrderType rejects order types this subdomain does not fulfil.
var ErrInvalidOrderType = fmt.Errorf("invalid order type")

// NotifyKind labels the fulfillment outcome for the caller's notification
// dispatch, without coupling this module to the notification templates.
const (
	NotifyPurchase     = "purchase"
	NotifyRenewal      = "renewal"
	NotifyResetTraffic = "reset_traffic"
)

// Outcome carries what the caller needs after the fulfillment committed:
// notification context only — every domain mutation is already committed.
type Outcome struct {
	UserID     int64
	PlanName   string
	NotifyKind string
	HasSub     bool
	ExpireAt   time.Time
}

// outcomeParts is the internal working set assembled inside the fulfillment
// transaction (entities stay inside the module).
type outcomeParts struct {
	order      *order.Order
	subscribe  *subscribe.Subscribe
	userSub    *usersub.Subscribe
	notifyType string
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	// Orders is the billing-domain read port resolving the paid order.
	Orders repository.OrderRepo
	// Store carries the subscription-scoped fulfillment transaction; the
	// per-user quota serialization uses the domain's own serial lock.
	Store    Store
	UserSubs repository.UserSubscriptionRepo
	Plans    repository.SubscribeRepo
	Cache    repository.UserCacheRepo
	// SingleModel forbids holding more than one blocking subscription;
	// runtime-mutable, read per request.
	SingleModel func() bool
}

// Service is the fulfillment entry point used by the subscription facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// FulfillPaidOrder applies the order's effect exactly once and returns the
// notification context. A replayed delivery whose fulfillment already
// committed rebuilds the context without re-applying anything; post-commit
// cache invalidation runs on every path because it is retryable.
func (s *Service) FulfillPaidOrder(ctx context.Context, orderNo string) (*Outcome, error) {
	orderInfo, err := s.deps.Orders.FindOneByOrderNo(ctx, orderNo)
	if err != nil {
		return nil, err
	}
	mark, err := s.deps.Store.Inbox().Find(ctx, inboxFulfillment, orderNo)
	if err != nil {
		return nil, err
	}
	var parts *outcomeParts
	if mark != nil {
		parts, err = s.loadOutcome(ctx, orderInfo)
	} else {
		err = s.deps.Store.InSubscriptionTx(ctx, func(store repository.SubscriptionStore) error {
			var txErr error
			parts, txErr = s.processOrderByTypeInTx(ctx, store, orderInfo)
			if txErr != nil {
				return txErr
			}
			// A duplicate key here means a concurrent delivery fulfilled
			// first; this transaction rolls back and the retry takes the
			// replay path.
			return store.Inbox().Insert(ctx, inboxFulfillment, orderNo, "")
		})
	}
	if err != nil {
		return nil, err
	}
	s.afterCommit(ctx, parts)
	return &Outcome{
		UserID:     orderInfo.UserId,
		PlanName:   parts.subscribe.Name,
		NotifyKind: parts.notifyType,
		HasSub:     parts.userSub != nil,
		ExpireAt:   expireOf(parts.userSub),
	}, nil
}

func expireOf(sub *usersub.Subscribe) time.Time {
	if sub == nil {
		return time.Time{}
	}
	return sub.ExpireTime
}

// loadOutcome rebuilds the notification context for a replayed delivery.
func (s *Service) loadOutcome(ctx context.Context, orderInfo *order.Order) (*outcomeParts, error) {
	token := orderInfo.SubscribeToken
	if orderInfo.Type == OrderTypeSubscribe {
		// New-purchase tokens are derived from the order number.
		token = usersub.TokenFromOrder(orderInfo.OrderNo)
	}
	userSub, err := s.deps.UserSubs.FindOneSubscribeByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	subID := orderInfo.SubscribeId
	if orderInfo.Type == OrderTypeResetTraffic {
		subID = userSub.SubscribeId
	}
	sub, err := s.deps.Plans.FindOne(ctx, subID)
	if err != nil {
		return nil, err
	}
	parts := &outcomeParts{order: orderInfo, subscribe: sub, userSub: userSub}
	switch orderInfo.Type {
	case OrderTypeSubscribe:
		parts.notifyType = NotifyPurchase
	case OrderTypeRenewal:
		parts.notifyType = NotifyRenewal
	case OrderTypeResetTraffic:
		parts.notifyType = NotifyResetTraffic
	}
	return parts, nil
}

// afterCommit runs the retryable cache invalidation for the committed
// fulfillment.
func (s *Service) afterCommit(ctx context.Context, parts *outcomeParts) {
	if parts.userSub != nil {
		if err := s.deps.Cache.ClearSubscribeCache(ctx, parts.userSub); err != nil {
			logger.WithContext(ctx).Error("[Fulfillment] Clear user subscribe cache failed", logger.Field("error", err.Error()))
		}
	}
	if parts.subscribe != nil {
		if err := s.deps.Plans.ClearCache(ctx, parts.subscribe.Id); err != nil {
			logger.WithContext(ctx).Error("[Fulfillment] Clear plan cache failed", logger.Field("error", err.Error()))
		}
	}
}

func (s *Service) processOrderByTypeInTx(ctx context.Context, store repository.SubscriptionStore, orderInfo *order.Order) (*outcomeParts, error) {
	switch orderInfo.Type {
	case OrderTypeSubscribe:
		return s.activateNewPurchaseTx(ctx, store, orderInfo)
	case OrderTypeRenewal:
		return s.activateRenewalTx(ctx, store, orderInfo)
	case OrderTypeResetTraffic:
		return s.activateResetTrafficTx(ctx, store, orderInfo)
	default:
		return nil, ErrInvalidOrderType
	}
}

func (s *Service) activateNewPurchaseTx(ctx context.Context, store repository.SubscriptionStore, orderInfo *order.Order) (*outcomeParts, error) {
	// Guest accounts are created by ensureGuestAccount before this stage, so
	// UserId is always set here. The domain's serial lock serialises
	// per-user quota checks and subscription creation (it replaced the
	// cross-domain user-row lock).
	if err := store.UserSubscription().LockUserSerial(ctx, orderInfo.UserId); err != nil {
		return nil, err
	}

	sub, err := store.Subscribe().FindOne(ctx, orderInfo.SubscribeId)
	if err != nil {
		return nil, err
	}
	userSub, err := s.createUserSubscriptionTx(ctx, store, orderInfo, sub)
	if err != nil {
		return nil, err
	}
	return &outcomeParts{order: orderInfo, subscribe: sub, userSub: userSub, notifyType: NotifyPurchase}, nil
}

func (s *Service) createUserSubscriptionTx(ctx context.Context, store repository.SubscriptionStore, orderInfo *order.Order, sub *subscribe.Subscribe) (*usersub.Subscribe, error) {
	if s.deps.SingleModel() {
		hasBlockingSubscription, err := store.UserSubscription().HasBlockingSubscription(ctx, orderInfo.UserId)
		if err != nil {
			return nil, err
		}
		if hasBlockingSubscription {
			return nil, fmt.Errorf("single subscription mode exceeds limit")
		}
	}
	if sub.Quota > 0 {
		count, err := store.UserSubscription().CountQuotaConsumingSubscriptions(ctx, orderInfo.UserId, orderInfo.SubscribeId)
		if err != nil {
			return nil, err
		}
		if count >= sub.Quota {
			return nil, fmt.Errorf("subscribe quota limit exceeded")
		}
	}
	now := timeutil.Now()
	userSub := &usersub.Subscribe{
		UserId:      orderInfo.UserId,
		OrderId:     orderInfo.Id,
		SubscribeId: orderInfo.SubscribeId,
		StartTime:   now,
		ExpireTime:  timeutil.AddTime(sub.UnitTime, orderInfo.Quantity, now),
		Traffic:     sub.Traffic,
		Token:       usersub.TokenFromOrder(orderInfo.OrderNo),
		UUID:        uuid.NewV4().String(),
		Status:      1,
	}
	if err := store.UserSubscription().InsertSubscribe(ctx, userSub); err != nil {
		return nil, err
	}
	return userSub, nil
}

func (s *Service) activateRenewalTx(ctx context.Context, store repository.SubscriptionStore, orderInfo *order.Order) (*outcomeParts, error) {
	userSub, err := store.UserSubscription().FindOneSubscribeByTokenForUpdate(ctx, orderInfo.SubscribeToken)
	if err != nil {
		return nil, err
	}
	if userSub.UserId != orderInfo.UserId {
		return nil, fmt.Errorf("renewal subscription ownership mismatch")
	}
	sub, err := store.Subscribe().FindOne(ctx, orderInfo.SubscribeId)
	if err != nil {
		return nil, err
	}
	if err := s.updateSubscriptionForRenewalTx(ctx, store, userSub, sub, orderInfo); err != nil {
		return nil, err
	}
	return &outcomeParts{order: orderInfo, subscribe: sub, userSub: userSub, notifyType: NotifyRenewal}, nil
}

func (s *Service) updateSubscriptionForRenewalTx(ctx context.Context, store repository.SubscriptionStore, userSub *usersub.Subscribe, sub *subscribe.Subscribe, orderInfo *order.Order) error {
	now := timeutil.Now()
	if userSub.ExpireTime.Before(now) {
		userSub.ExpireTime = now
	}
	today := now.Day()
	resetDay := userSub.ExpireTime.Day()
	if (sub.RenewalReset != nil && *sub.RenewalReset) || today == resetDay {
		userSub.Download = 0
		userSub.Upload = 0
	}
	if userSub.FinishedAt != nil {
		if userSub.FinishedAt.Before(now) && today > resetDay {
			userSub.Download = 0
			userSub.Upload = 0
		}
		userSub.FinishedAt = nil
	}
	userSub.ExpireTime = timeutil.AddTime(sub.UnitTime, orderInfo.Quantity, userSub.ExpireTime)
	userSub.Status = 1
	return store.UserSubscription().UpdateSubscribe(ctx, userSub)
}

func (s *Service) activateResetTrafficTx(ctx context.Context, store repository.SubscriptionStore, orderInfo *order.Order) (*outcomeParts, error) {
	userSub, err := store.UserSubscription().FindOneSubscribeByTokenForUpdate(ctx, orderInfo.SubscribeToken)
	if err != nil {
		return nil, err
	}
	if userSub.UserId != orderInfo.UserId {
		return nil, fmt.Errorf("reset subscription ownership mismatch")
	}
	userSub.Download = 0
	userSub.Upload = 0
	userSub.Status = 1
	userSub.FinishedAt = nil
	if err := store.UserSubscription().UpdateSubscribe(ctx, userSub); err != nil {
		return nil, err
	}
	sub, err := store.Subscribe().FindOne(ctx, userSub.SubscribeId)
	if err != nil {
		return nil, err
	}
	resetLog := &log.ResetSubscribe{
		Type:      log.ResetSubscribeTypePaid,
		UserId:    orderInfo.UserId,
		OrderNo:   orderInfo.OrderNo,
		Timestamp: timeutil.Now().UnixMilli(),
	}
	content, err := resetLog.Marshal()
	if err != nil {
		return nil, err
	}
	if err := store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeResetSubscribe.Uint8(),
		Date:     timeutil.Now().Format(time.DateOnly),
		ObjectID: userSub.Id,
		Content:  string(content),
	}); err != nil {
		return nil, err
	}
	return &outcomeParts{order: orderInfo, subscribe: sub, userSub: userSub, notifyType: NotifyResetTraffic}, nil
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.SubscriptionTransactor
	Inbox() repository.InboxRepo
}
