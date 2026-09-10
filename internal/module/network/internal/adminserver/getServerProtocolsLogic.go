package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetServerProtocolsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Server Protocols
func newGetServerProtocolsLogic(ctx context.Context, deps Deps) *GetServerProtocolsLogic {
	return &GetServerProtocolsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetServerProtocolsLogic) GetServerProtocols(req *dto.GetServerProtocolsRequest) (resp *dto.GetServerProtocolsResponse, err error) {
	// find server
	data, err := l.deps.Store.Node().FindOneServer(l.ctx, req.Id)
	if err != nil {
		l.Errorf("[GetServerProtocols] FindOneServer Error: %s", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[GetServerProtocols] FindOneServer Error: %s", err.Error())
	}

	// handler protocols
	var protocols []dto.Protocol
	dst, err := data.UnmarshalProtocols()
	if err != nil {
		l.Errorf("[FilterServerList] UnmarshalProtocols Error: %s", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[FilterServerList] UnmarshalProtocols Error: %s", err.Error())
	}
	mapping.DeepCopy(&protocols, dst)

	return &dto.GetServerProtocolsResponse{
		Protocols: protocols,
	}, nil
}
