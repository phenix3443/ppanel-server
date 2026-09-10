package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"

	"github.com/pkg/errors"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
)

type UpdateRegisterConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateRegisterConfigLogic(ctx context.Context, deps Deps) *UpdateRegisterConfigLogic {
	return &UpdateRegisterConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateRegisterConfigLogic) UpdateRegisterConfig(req *dto.RegisterConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "register", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateRegisterConfig] update register config error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update register config error: %v", err.Error())
	}
	// init system config
	l.deps.reinit("register")
	return nil
}
