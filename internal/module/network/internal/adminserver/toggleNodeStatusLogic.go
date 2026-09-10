package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type ToggleNodeStatusLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewToggleNodeStatusLogic Toggle Node Status
func newToggleNodeStatusLogic(ctx context.Context, deps Deps) *ToggleNodeStatusLogic {
	return &ToggleNodeStatusLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ToggleNodeStatusLogic) ToggleNodeStatus(req *dto.ToggleNodeStatusRequest) error {
	nodeStore := l.deps.Store.Node()
	data, err := nodeStore.FindOneNode(l.ctx, req.Id)
	if err != nil {
		l.Errorw("[ToggleNodeStatus] Query Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[ToggleNodeStatus] Query Database Error")
	}
	data.Enabled = req.Enable

	err = nodeStore.UpdateNode(l.ctx, data)
	if err != nil {
		l.Errorw("[ToggleNodeStatus] Update Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "[ToggleNodeStatus] Update Database Error")
	}

	return nodeStore.ClearServerCache(l.ctx, data.ServerId)
}
