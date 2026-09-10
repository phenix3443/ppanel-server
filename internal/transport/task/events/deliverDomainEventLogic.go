package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/eventbus"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
)

// DeliverDomainEventLogic is the delivery worker: each task carries one
// outbox event, and the bus runs every subscriber of its topic. A failing
// subscriber fails the task so asynq retries with backoff and eventually
// archives it (the dead-letter queue); subscribers are idempotent, so
// retries and duplicate deliveries are safe.
type DeliverDomainEventLogic struct {
	bus *eventbus.Bus
}

func NewDeliverDomainEventLogic(bus *eventbus.Bus) *DeliverDomainEventLogic {
	return &DeliverDomainEventLogic{bus: bus}
}

func (l *DeliverDomainEventLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload taskqueue.EventDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		// A malformed payload can never deliver; retrying cannot fix it.
		return fmt.Errorf("unmarshal event payload: %v: %w", err, asynq.SkipRetry)
	}
	return l.bus.Deliver(ctx, eventbus.Event{
		ID:      payload.ID,
		Topic:   payload.Topic,
		Key:     payload.Key,
		Payload: payload.Payload,
	})
}
