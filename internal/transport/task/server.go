package task

import (
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/pkg/logger"
)

type Service struct {
	deps   Dependencies
	server *asynq.Server
}

func NewService(redisConfig config.RedisConfig, deps Dependencies) *Service {
	return &Service{
		deps:   deps,
		server: initService(redisConfig),
	}
}

func (m *Service) Start() {
	logger.Infof("start consumer service")
	mux := asynq.NewServeMux()
	// Resume the producer's trace from the payload envelope and span every
	// task execution before any handler runs.
	mux.Use(taskqueue.Middleware())
	// register tasks
	RegisterHandlers(mux, m.deps)
	if err := m.server.Run(mux); err != nil {
		logger.Error("consumer service error", logger.LogField{
			Key:   "error",
			Value: err.Error(),
		})
	}
}

func (m *Service) Stop() {
	logger.Info("stop consumer service")
	m.server.Stop()
}

func initService(redisConfig config.RedisConfig) *asynq.Server {
	return asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisConfig.Host, Password: redisConfig.Pass, DB: 5},
		asynq.Config{
			IsFailure: func(err error) bool {
				logger.Error("consumer service error", logger.Field("error", err.Error()))
				return true
			},
			Concurrency: 20,
		},
	)
}
