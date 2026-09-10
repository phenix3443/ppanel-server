package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type DeleteNodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewDeleteNodeLogic Delete Node
func newDeleteNodeLogic(ctx context.Context, deps Deps) *DeleteNodeLogic {
	return &DeleteNodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeleteNodeLogic) DeleteNode(req *dto.DeleteNodeRequest) error {
	nodeStore := l.deps.Store.Node()
	data, err := nodeStore.FindOneNode(l.ctx, req.Id)
	if err != nil {
		l.Errorw("[DeleteNode] Query Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[DeleteNode] Query Database Error")
	}

	err = nodeStore.DeleteNode(l.ctx, req.Id)
	if err != nil {
		l.Errorw("[DeleteNode] Delete Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "[DeleteNode] Delete Database Error")
	}

	return nodeStore.ClearServerCache(l.ctx, data.ServerId)
}
