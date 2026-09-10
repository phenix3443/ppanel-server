package subscription

import (
	"context"

	"github.com/hibiken/asynq"
	module "github.com/perfect-panel/server/internal/module/subscription"
)

// RemindExpiringLogic is the queue shell for the pre-expiry reminder; the
// business logic lives in the subscription module.
type RemindExpiringLogic struct {
	service module.Service
}

func NewRemindExpiringLogic(service module.Service) *RemindExpiringLogic {
	return &RemindExpiringLogic{service: service}
}

func (l *RemindExpiringLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.service.RemindExpiringSubscriptions(ctx)
}
