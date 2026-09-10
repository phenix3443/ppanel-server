package tool

import (
	"context"

	"github.com/perfect-panel/server/pkg/logger"
)

type RestartSystemLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Restart System
func newRestartSystemLogic(ctx context.Context, deps Deps) *RestartSystemLogic {
	return &RestartSystemLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *RestartSystemLogic) RestartSystem() error {
	l.Logger.Info("[RestartSystem]", logger.Field("info", "Restarting system"))
	go func() {
		err := l.deps.Restart()
		if err != nil {
			l.Errorw("[RestartSystem]", logger.Field("error", err.Error()))
		}
		l.Logger.Info("[RestartSystem]", logger.Field("info", "System restarted"))
	}()
	return nil
}
