package subscription

import (
	"context"

	"github.com/hibiken/asynq"
	module "github.com/perfect-panel/server/internal/module/subscription"
)

// CheckSubscriptionLogic is the queue shell for the subscription lifecycle
// sweep; the business logic lives in the subscription module (ADR-001
// step 6 preparation).
type CheckSubscriptionLogic struct {
	service module.Service
}

func NewCheckSubscriptionLogic(service module.Service) *CheckSubscriptionLogic {
	return &CheckSubscriptionLogic{service: service}
}

func (l *CheckSubscriptionLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.service.CheckSubscriptions(ctx)
}
