package systemsetting

import (
	"context"
	"github.com/perfect-panel/server/internal/infra/nodeversion"

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
	// 这个值会被节点拿去拼 GitHub release 的 URL 下载二进制，畸形输入要在写库前拦住，
	// 否则故障现场在远端节点上。和单节点设置走同一个校验。
	if err := nodeversion.ValidateTarget(req.DefaultTargetVersion); err != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, err.Error()), "invalid default target version")
	}
	err := updateConfigFields(l.ctx, l.deps, "server", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateNodeConfig] update node config error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update server config error: %v", err)
	}
	l.deps.reinit("node")
	return nil
}
