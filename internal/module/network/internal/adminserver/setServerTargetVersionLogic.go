package adminserver

import (
	"context"
	"fmt"
	"regexp"

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

// 和我们 release 的 tag 命名一致：v + 三段数字，允许 -rc.1 这类预发布后缀。
// 不接受 "latest"：那会让升级时机取决于「节点哪一刻去拉的」，不可预测也不可复现。
var targetVersionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.\-]+)?$`)

// validTargetVersion 校验控制台填进来的版本号。
//
// 这个值会被节点拿去拼 GitHub release 的 URL、下载并替换自己的二进制，
// 所以必须在写库前拦住畸形输入——放过去的话，故障现场在远端节点上，离这里很远。
// 空串是合法的，表示「不干预这个节点」。
func validTargetVersion(v string) error {
	if v == "" {
		return nil
	}
	if !targetVersionPattern.MatchString(v) {
		return fmt.Errorf("版本号格式不对：应形如 v1.1.14（不支持 latest，升级时机要可复现）")
	}
	return nil
}

// SetServerTargetVersion 设置某个节点的期望版本。
//
// 【支持降级】这里不校验「必须比当前版本新」——新版本出问题时要能回退，
// 而回退和升级走的是同一条路：把期望版本改成旧的那个，节点下次拉配置就切回去。
func (l *SetServerTargetVersionLogic) SetServerTargetVersion(req *dto.SetServerTargetVersionRequest) error {
	if err := validTargetVersion(req.TargetVersion); err != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, err.Error()), "invalid target version")
	}
	nodeStore := l.deps.Store.Node()
	server, err := nodeStore.FindOneServer(l.ctx, req.Id)
	if err != nil || server.Id <= 0 {
		l.Errorw("[SetServerTargetVersion] FindOneServer Error", logger.Field("error", err))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "server not found")
	}
	server.TargetVersion = req.TargetVersion
	if err := nodeStore.UpdateServer(l.ctx, server); err != nil {
		l.Errorw("[SetServerTargetVersion] UpdateServer Error", logger.Field("error", err))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update target version failed")
	}
	// 节点读的是缓存过的配置，不清掉的话下发会延迟一个缓存周期。
	return nodeStore.ClearServerCache(l.ctx, req.Id)
}
