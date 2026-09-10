package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type DeleteServerLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewDeleteServerLogic Delete Server
func newDeleteServerLogic(ctx context.Context, deps Deps) *DeleteServerLogic {
	return &DeleteServerLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeleteServerLogic) DeleteServer(req *dto.DeleteServerRequest) error {
	if err := l.deps.Store.InNetworkTx(l.ctx, func(store repository.NetworkStore) error {
		nodeStore := store.Node()
		if err := nodeStore.DeleteServer(l.ctx, req.Id); err != nil {
			return err
		}
		return nodeStore.DeleteServerConfigOverride(l.ctx, req.Id)
	}); err != nil {
		l.Errorw("[DeleteServer] Delete Server Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "[DeleteServer] Delete Server Error")
	}
	return l.deps.Store.Node().ClearServerCache(l.ctx, req.Id)
}
