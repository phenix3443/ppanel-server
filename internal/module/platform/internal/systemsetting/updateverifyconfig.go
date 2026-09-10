package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateVerifyConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateVerifyConfigLogic(ctx context.Context, deps Deps) *UpdateVerifyConfigLogic {
	return &UpdateVerifyConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateVerifyConfigLogic) UpdateVerifyConfig(req *dto.VerifyConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "verify", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateVerifyConfigLogic] update verify config error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update verify config error: %v", err)
	}
	// Update the config
	l.deps.applyVerifyConfig(req)
	l.deps.reinit("verify")
	return nil
}
