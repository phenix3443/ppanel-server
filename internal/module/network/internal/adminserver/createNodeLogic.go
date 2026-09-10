package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type CreateNodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewCreateNodeLogic Create Node
func newCreateNodeLogic(ctx context.Context, deps Deps) *CreateNodeLogic {
	return &CreateNodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CreateNodeLogic) CreateNode(req *dto.CreateNodeRequest) error {
	data := node.Node{
		Name:     req.Name,
		Tags:     slicesx.StringSliceToString(req.Tags),
		Enabled:  req.Enabled,
		Port:     req.Port,
		Address:  req.Address,
		ServerId: req.ServerId,
		Protocol: req.Protocol,
	}
	err := l.deps.Store.Node().InsertNode(l.ctx, &data)
	if err != nil {
		l.Errorw("[CreateNode] Insert Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "[CreateNode] Insert Database Error")
	}

	return nil
}
