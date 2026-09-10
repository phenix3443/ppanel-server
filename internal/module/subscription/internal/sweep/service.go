// Package sweep implements the subscription lifecycle sweep: marking
// traffic-exceeded and expired subscriptions finished, then firing the
// retryable side effects (owner notification, cache invalidation). Only the
// module facade may reach it.
package sweep

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

// Notifier delivers the lifecycle notices to the subscription owner. The
// composition root adapts it to the notification queue until a notification
// facade owns email delivery.
type Notifier interface {
	NotifySubscriptionExpired(ctx context.Context, email string, expiredAt time.Time)
	NotifyTrafficExceeded(ctx context.Context, email string)
	// NotifySubscriptionExpiring warns the owner before the subscription
	// stops. Renewal amount is in minor units; an empty plan name or a zero
	// amount means the plan could not be read.
	NotifySubscriptionExpiring(ctx context.Context, userID int64, planName string, expireAt time.Time, renewalAmount int64)
}

// OwnerEmailReader is the read-only identity port resolving a user's email
// binding; the legacy user-auth repository satisfies it structurally.
type OwnerEmailReader interface {
	FindUserAuthMethodsByUserIds(ctx context.Context, method string, userIds []int64) ([]*user.AuthMethods, error)
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	UserSubs repository.UserSubscriptionRepo
	Plans    repository.SubscribeRepo
	Cache    repository.UserCacheRepo
	Store    Store
	Emails   OwnerEmailReader
	Notify   Notifier
}

// Service is the lifecycle-sweep entry point used by the subscription
// facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// CheckSubscriptions runs both lifecycle sweeps. Each sweep commits its
// status flip in a subscription-domain transaction; notifications and cache
// invalidation are retryable side effects that run after the commit
// (ADR-001 step 2).
func (s *Service) CheckSubscriptions(ctx context.Context) error {
	logger.Infof("[CheckSubscription] Start check subscription: %s", timeutil.Now().Format("2006-01-02 15:04:05"))
	if err := s.markSubscribes(ctx, 2, "[Check Subscription Traffic]", s.sendTrafficNotify,
		func(store repository.SubscriptionStore) ([]*usersub.Subscribe, error) {
			return store.UserSubscription().FindTrafficExceededSubscribes(ctx)
		}); err != nil {
		logger.Error("[CheckSubscription] Transaction failed", logger.Field("error", err.Error()))
	}
	if err := s.markSubscribes(ctx, 3, "[Check Subscription Expire]", s.sendExpiredNotify,
		func(store repository.SubscriptionStore) ([]*usersub.Subscribe, error) {
			return store.UserSubscription().FindExpiredSubscribes(ctx, timeutil.Now())
		}); err != nil {
		logger.Info("[CheckSubscription] Transaction failed", logger.Field("error", err.Error()))
	}
	return nil
}

func (s *Service) markSubscribes(ctx context.Context, status uint8, tag string, notify func(context.Context, []*usersub.Subscribe), find func(repository.SubscriptionStore) ([]*usersub.Subscribe, error)) error {
	var list []*usersub.Subscribe
	err := s.deps.Store.InSubscriptionTx(ctx, func(store repository.SubscriptionStore) error {
		var err error
		list, err = find(store)
		if err != nil {
			logger.Errorw(tag+" Query subscribe failed", logger.Field("error", err.Error()))
			return err
		}
		if len(list) == 0 {
			return nil
		}
		ids := make([]int64, 0, len(list))
		for _, item := range list {
			ids = append(ids, item.Id)
		}
		return store.UserSubscription().MarkSubscribesFinished(ctx, ids, status, timeutil.Now())
	})
	if err != nil {
		return err
	}
	if len(list) == 0 {
		logger.Info(tag + " No subscribe need to update")
		return nil
	}
	ids := make([]int64, 0, len(list))
	for _, item := range list {
		ids = append(ids, item.Id)
	}
	notify(ctx, list)
	if err := s.deps.Cache.ClearSubscribeCache(ctx, list...); err != nil {
		logger.Errorw(tag+" Clear subscribe cache failed", logger.Field("error", err.Error()))
	}
	s.clearServerCache(ctx, list...)
	logger.Infow(tag+" Update subscribe status", logger.Field("user_ids", ids), logger.Field("count", int64(len(ids))))
	return nil
}

func (s *Service) ownerEmails(ctx context.Context, subs []*usersub.Subscribe) map[int64]string {
	if len(subs) == 0 || s.deps.Emails == nil {
		return nil
	}
	userIDs := make([]int64, 0, len(subs))
	seen := make(map[int64]struct{}, len(subs))
	for _, sub := range subs {
		if sub == nil || sub.UserId <= 0 {
			continue
		}
		if _, ok := seen[sub.UserId]; ok {
			continue
		}
		seen[sub.UserId] = struct{}{}
		userIDs = append(userIDs, sub.UserId)
	}
	methods, err := s.deps.Emails.FindUserAuthMethodsByUserIds(ctx, "email", userIDs)
	if err != nil {
		logger.Errorw("[CheckSubscription] FindUserAuthMethodsByUserIds failed", logger.Field("error", err.Error()), logger.Field("user_count", len(userIDs)))
		return nil
	}
	emails := make(map[int64]string, len(methods))
	for _, method := range methods {
		if method != nil && method.UserId > 0 && method.AuthIdentifier != "" {
			emails[method.UserId] = method.AuthIdentifier
		}
	}
	return emails
}

func (s *Service) sendExpiredNotify(ctx context.Context, subs []*usersub.Subscribe) {
	emails := s.ownerEmails(ctx, subs)
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		if email := emails[sub.UserId]; email != "" {
			s.deps.Notify.NotifySubscriptionExpired(ctx, email, sub.ExpireTime)
		}
	}
}

func (s *Service) sendTrafficNotify(ctx context.Context, subs []*usersub.Subscribe) {
	emails := s.ownerEmails(ctx, subs)
	for _, sub := range subs {
		if sub == nil {
			continue
		}
		if email := emails[sub.UserId]; email != "" {
			s.deps.Notify.NotifyTrafficExceeded(ctx, email)
		}
	}
}

func (s *Service) clearServerCache(ctx context.Context, userSubs ...*usersub.Subscribe) {
	subs := make(map[int64]bool)
	for _, sub := range userSubs {
		subs[sub.SubscribeId] = true
	}
	for sub := range subs {
		if err := s.deps.Plans.ClearCache(ctx, sub); err != nil {
			logger.Errorw("[CheckSubscription] ClearCache failed", logger.Field("error", err.Error()), logger.Field("subscribe_id", sub))
		}
	}
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.SubscriptionTransactor
	Inbox() repository.InboxRepo
}
