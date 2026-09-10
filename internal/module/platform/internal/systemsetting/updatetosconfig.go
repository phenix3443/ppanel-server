package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateTosConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateTosConfigLogic(ctx context.Context, deps Deps) *UpdateTosConfigLogic {
	return &UpdateTosConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateTosConfigLogic) UpdateTosConfig(req *dto.TosConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "tos", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateTosConfigLogic] update tos config error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update tos config error: %v", err)
	}

	return nil
}
