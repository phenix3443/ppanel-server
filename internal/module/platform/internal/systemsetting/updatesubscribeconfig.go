package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateSubscribeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateSubscribeConfigLogic(ctx context.Context, deps Deps) *UpdateSubscribeConfigLogic {
	return &UpdateSubscribeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateSubscribeConfigLogic) UpdateSubscribeConfig(req *dto.SubscribeConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "subscribe", convertedConfigFields(*req))

	if err != nil {
		l.Errorw("[UpdateSubscribeConfigLogic] update subscribe config error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update subscribe config error: %v", err)
	}

	if l.deps.subscribePath() != req.SubscribePath {
		go func() {
			if err := l.deps.restart(); err != nil {
				l.Errorw("[UpdateSubscribeConfigLogic] restart error: ", logger.Field("error", err.Error()))
			}
		}()
		return nil
	}

	l.deps.reinit("subscribe")
	return nil
}
