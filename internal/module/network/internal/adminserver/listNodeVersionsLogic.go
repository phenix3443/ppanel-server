package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/nodeversion"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type ListNodeVersionsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newListNodeVersionsLogic(ctx context.Context, deps Deps) *ListNodeVersionsLogic {
	return &ListNodeVersionsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// ListNodeVersions 给控制台的版本下拉供数。
//
// 【不返回 error】拉不到上游版本时返回空列表，前端退化成手输。把它做成会失败的
// 接口，GitHub 一抖动整个升级入口就点不动了，而降级恰恰是 GitHub 出问题时最
// 需要的操作。
func (l *ListNodeVersionsLogic) ListNodeVersions() (*dto.ListNodeVersionsResponse, error) {
	releases := nodeversion.Default.List()
	list := make([]dto.ServerNodeVersion, 0, len(releases))
	for _, r := range releases {
		list = append(list, dto.ServerNodeVersion{
			Version:        r.Version,
			Prerelease:     r.Prerelease,
			SelfManageable: nodeversion.SelfManageable(r.Version),
			PublishedAt:    r.PublishedAt,
		})
	}
	return &dto.ListNodeVersionsResponse{
		Repo:              nodeversion.Repo(),
		Latest:            nodeversion.Default.Latest(),
		Default:           l.deps.Config().Node.DefaultTargetVersion,
		MinSelfManageable: nodeversion.MinSelfManageable,
		List:              list,
	}, nil
}
