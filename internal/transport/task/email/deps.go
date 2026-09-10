package email

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
)

type TaskScheduler interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

type Dependencies struct {
	Store    repository.Store
	Queue    TaskScheduler
	Email    func() config.EmailConfig
	SiteName func() string
}
