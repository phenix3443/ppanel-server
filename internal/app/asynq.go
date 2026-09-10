package app

import (
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
)

func redisOpt(c config.Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{Addr: c.Redis.Host, Password: c.Redis.Pass, DB: 5}
}

// NewAsynqClient returns the tracing asynq client: EnqueueContext stamps the
// caller's trace context onto the task for the worker-side middleware to
// resume. Pass task options to EnqueueContext, not NewTask — wrapping
// rebuilds the task.
func NewAsynqClient(c config.Config) *taskqueue.Client {
	return taskqueue.NewClient(asynq.NewClient(redisOpt(c)))
}

func NewAsynqInspector(c config.Config) *asynq.Inspector {
	return asynq.NewInspector(redisOpt(c))
}
