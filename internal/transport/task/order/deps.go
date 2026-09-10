package order

import (
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

type Dependencies struct {
	Store        repository.Store
	Redis        *redis.Client
	Queue        *taskqueue.Client
	Inspector    *asynq.Inspector
	Billing      billing.Service
	Subscription subscription.Service
	Notification notification.Service
	Telegram     func() config.Telegram
}

func (deps Dependencies) telegramConfig() config.Telegram {
	if deps.Telegram == nil {
		return config.Telegram{}
	}
	return deps.Telegram()
}
