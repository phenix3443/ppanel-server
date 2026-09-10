package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateNodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateNodeLogic Update Node
func newUpdateNodeLogic(ctx context.Context, deps Deps) *UpdateNodeLogic {
	return &UpdateNodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateNodeLogic) UpdateNode(req *dto.UpdateNodeRequest) error {
	nodeStore := l.deps.Store.Node()
	data, err := nodeStore.FindOneNode(l.ctx, req.Id)
	if err != nil {
		l.Errorw("[UpdateNode] Query Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "[UpdateNode] Query Database Error")
	}
	oldServerID := data.ServerId
	data.Name = req.Name
	data.Tags = slicesx.StringSliceToString(req.Tags)
	data.ServerId = req.ServerId
	data.Port = req.Port
	data.Address = req.Address
	data.Protocol = req.Protocol
	data.Enabled = req.Enabled
	err = nodeStore.UpdateNode(l.ctx, data)
	if err != nil {
		l.Errorw("[UpdateNode] Update Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "[UpdateNode] Update Database Error")
	}
	if err := nodeStore.ClearServerCache(l.ctx, oldServerID); err != nil {
		return err
	}
	if oldServerID != data.ServerId {
		return nodeStore.ClearServerCache(l.ctx, data.ServerId)
	}
	return nil
}
