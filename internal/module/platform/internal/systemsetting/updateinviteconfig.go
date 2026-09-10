package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
)

type UpdateInviteConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateInviteConfigLogic(ctx context.Context, deps Deps) *UpdateInviteConfigLogic {
	return &UpdateInviteConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateInviteConfigLogic) UpdateInviteConfig(req *dto.InviteConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "invite", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateInviteConfig] update invite config error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update invite config error: %v", err)
	}
	l.deps.reinit("invite")
	return nil
}
