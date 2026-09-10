package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/network/internal/geolocation"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type CreateServerLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewCreateServerLogic Create Server
func newCreateServerLogic(ctx context.Context, deps Deps) *CreateServerLogic {
	return &CreateServerLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CreateServerLogic) CreateServer(req *dto.CreateServerRequest) error {
	data := node.Server{
		Name:      req.Name,
		Country:   req.Country,
		City:      req.City,
		Address:   req.Address,
		Sort:      req.Sort,
		Protocols: "",
	}
	protocols := make([]node.Protocol, 0)
	for _, item := range req.Protocols {
		if item.Type == "" {
			return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols type is empty"), "protocols type is empty")
		}
		var protocol node.Protocol
		mapping.DeepCopy(&protocol, item)
		ensureGeneratedProtocolKey(&protocol, nil)
		ensureShadowsocks2022ServerKey(&protocol, nil)
		if err := ensureRealityProtocolKey(&protocol, nil); err != nil {
			l.Errorf("[CreateServer] Generate Reality Key Error: %v", err.Error())
			return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "generate reality key error: %v", err)
		}
		ensureRealityProtocolDefaults(&protocol)
		protocol, err := node.NormalizeProtocolForStorage(protocol)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, err.Error()), "protocols normalize error: %v", err)
		}
		protocols = append(protocols, protocol)
	}

	err := data.MarshalProtocols(protocols)
	if err != nil {
		l.Errorf("[CreateServer] Marshal Protocols Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "protocols marshal error"), "protocols marshal error: %v", err)
	}
	if data.City == "" && data.Country == "" {
		// query server ip location
		result, err := geolocation.GetRegionByIp(req.Address)
		if err != nil {
			l.Errorf("[CreateServer] GetRegionByIp Error: %v", err.Error())
		} else {
			data.City = result.City
			data.Country = result.Country
		}
	}
	err = l.deps.Store.Node().InsertServer(l.ctx, &data)
	if err != nil {
		l.Errorf("[CreateServer] Insert Server error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert server error: %v", err)
	}
	return nil
}
