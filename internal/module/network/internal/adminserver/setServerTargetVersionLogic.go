package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/nodeversion"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type SetServerTargetVersionLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newSetServerTargetVersionLogic(ctx context.Context, deps Deps) *SetServerTargetVersionLogic {
	return &SetServerTargetVersionLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// SetServerTargetVersion 设置一批节点的期望版本。
//
// 【支持降级】这里不校验「必须比当前版本新」——新版本出问题时要能回退，
// 而回退和升级走的是同一条路：把期望版本改成旧的那个，节点下次拉配置就切回去。
func (l *SetServerTargetVersionLogic) SetServerTargetVersion(req *dto.SetServerTargetVersionRequest) error {
	if err := nodeversion.ValidateTarget(req.TargetVersion); err != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, err.Error()), "invalid target version")
	}

	ids := req.Ids
	if len(ids) == 0 && req.Id > 0 {
		ids = []int64{req.Id}
	}
	if len(ids) == 0 {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "没有选中任何节点"), "no server selected")
	}

	nodeStore := l.deps.Store.Node()
	if err := nodeStore.UpdateServerTargetVersion(l.ctx, ids, req.TargetVersion); err != nil {
		l.Errorw("[SetServerTargetVersion] UpdateServerTargetVersion Error", logger.Field("error", err))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update target version failed")
	}

	// 节点读的是缓存过的配置，不清掉的话下发会延迟一个缓存周期。
	// 【清缓存失败不回滚】库已经写成功了，这里再报错会让管理员以为没生效而重试；
	// ClearServerCache 内部有重试队列兜底。
	for _, id := range ids {
		if err := nodeStore.ClearServerCache(l.ctx, id); err != nil {
			l.Errorw("[SetServerTargetVersion] ClearServerCache Error",
				logger.Field("error", err), logger.Field("server_id", id))
		}
	}
	return nil
}
