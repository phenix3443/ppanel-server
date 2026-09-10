package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/nodeconfig"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateServerNodeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateServerNodeConfigLogic(ctx context.Context, deps Deps) *UpdateServerNodeConfigLogic {
	return &UpdateServerNodeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateServerNodeConfigLogic) UpdateServerNodeConfig(req *dto.UpdateServerNodeConfigRequest) error {
	nodeStore := l.deps.Store.Node()
	if _, err := nodeStore.FindOneServer(l.ctx, req.ServerID); err != nil {
		l.Errorf("[UpdateServerNodeConfig] FindOneServer Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server error: %v", err)
	}

	data, allInherited, err := nodeconfig.OverrideModel(req.ServerID, req.ServerNodeConfigOverride)
	if err != nil {
		l.Errorf("[UpdateServerNodeConfig] OverrideModel Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCodeMsg(xerr.InvalidParams, "server node config is invalid"), "server node config is invalid: %v", err)
	}

	if allInherited {
		err = nodeStore.DeleteServerConfigOverride(l.ctx, req.ServerID)
	} else {
		err = nodeStore.SaveServerConfigOverride(l.ctx, data)
	}
	if err != nil {
		l.Errorf("[UpdateServerNodeConfig] SaveServerConfigOverride Error: %v", err.Error())
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server node config error: %v", err)
	}

	return nodeStore.ClearServerCache(l.ctx, req.ServerID)
}
