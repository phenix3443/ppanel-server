package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
)

type UpdateNodeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateNodeConfigLogic(ctx context.Context, deps Deps) *UpdateNodeConfigLogic {
	return &UpdateNodeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateNodeConfigLogic) UpdateNodeConfig(req *dto.NodeConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "server", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateNodeConfig] update node config error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server config error: %v", err)
	}
	l.deps.reinit("node")
	return nil
}
