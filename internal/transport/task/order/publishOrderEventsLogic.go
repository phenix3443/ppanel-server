package order

import (
	"context"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/pkg/logger"
)

// PublishOrderEventsLogic drains the durable order event outbox. Publishing
// is intentionally separate from writing the event: a Redis outage may delay
// a notification but can never roll back a committed payment state.
type PublishOrderEventsLogic struct {
	deps Dependencies
}

func NewPublishOrderEventsLogic(deps Dependencies) *PublishOrderEventsLogic {
	return &PublishOrderEventsLogic{deps: deps}
}

func (l *PublishOrderEventsLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	events, err := l.deps.Store.OrderEvent().ListUnpublished(ctx, 500)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := l.deps.Redis.Publish(ctx, order.EventChannel(event.OrderNo), strconv.FormatInt(event.ID, 10)).Err(); err != nil {
			return err
		}
		if _, err := l.deps.Store.OrderEvent().MarkPublished(ctx, event.ID, time.Now()); err != nil {
			return err
		}
	}
	if len(events) > 0 {
		logger.WithContext(ctx).Debugf("published %d order events", len(events))
	}
	return nil
}
