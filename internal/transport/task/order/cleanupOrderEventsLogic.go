package order

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/pkg/logger"
)

const orderEventRetention = 30 * 24 * time.Hour

// CleanupOrderEventsLogic removes only events that have already reached Redis
// and are older than the replay contract. Unpublished events are never
// deleted, even if an outage lasts longer than the normal retention period.
type CleanupOrderEventsLogic struct {
	deps Dependencies
}

func NewCleanupOrderEventsLogic(deps Dependencies) *CleanupOrderEventsLogic {
	return &CleanupOrderEventsLogic{deps: deps}
}

func (l *CleanupOrderEventsLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	deleted, err := l.deps.Store.OrderEvent().DeletePublishedBefore(ctx, time.Now().Add(-orderEventRetention))
	if err != nil {
		return err
	}
	if deleted > 0 {
		logger.WithContext(ctx).Infof("removed %d expired order events", deleted)
	}
	// The idempotent inbox shares the retention contract: every consumer's
	// replay window (deferred closes, activation retries, bucket flushes)
	// resolves far inside it.
	outboxDeleted, err := l.deps.Store.Outbox().DeletePublishedBefore(ctx, time.Now().Add(-orderEventRetention))
	if err != nil {
		return err
	}
	if outboxDeleted > 0 {
		logger.WithContext(ctx).Infof("cleaned up %d published domain events", outboxDeleted)
	}
	inboxDeleted, err := l.deps.Store.Inbox().DeleteProcessedBefore(ctx, time.Now().Add(-orderEventRetention))
	if err != nil {
		return err
	}
	if inboxDeleted > 0 {
		logger.WithContext(ctx).Infof("removed %d expired inbox markers", inboxDeleted)
	}
	return nil
}
