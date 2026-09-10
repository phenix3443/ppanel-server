// Package events hosts the queue shells of the domain-event bus: the publish
// pump moving outbox rows onto the asynq broker, and the delivery worker
// running subscribers.
package events

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/eventbus"
)

// DispatchDomainEventsLogic is the publish pump: it drains the generic
// domain-event outbox onto the asynq queue. Enqueues deduplicate by outbox
// event id, so the tick can retry freely.
type DispatchDomainEventsLogic struct {
	bus *eventbus.Bus
}

func NewDispatchDomainEventsLogic(bus *eventbus.Bus) *DispatchDomainEventsLogic {
	return &DispatchDomainEventsLogic{bus: bus}
}

func (l *DispatchDomainEventsLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.bus.Publish(ctx, 500)
}
