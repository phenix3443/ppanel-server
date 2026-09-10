package usersub

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserSubscribeTrafficLogsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user subcribe traffic logs
func newGetUserSubscribeTrafficLogsLogic(ctx context.Context, deps Deps) *GetUserSubscribeTrafficLogsLogic {
	return &GetUserSubscribeTrafficLogsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserSubscribeTrafficLogsLogic) GetUserSubscribeTrafficLogs(req *dto.GetUserSubscribeTrafficLogsRequest) (resp *dto.GetUserSubscribeTrafficLogsResponse, err error) {
	list, total, err := l.deps.Traffic.QueryTrafficLogPageList(l.ctx, req.UserId, req.SubscribeId, req.Page, req.Size)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetUserSubscribeTrafficLogs failed: %v", err.Error())
	}
	userRespList := make([]dto.TrafficLog, 0)
	mapping.DeepCopy(&userRespList, list)
	return &dto.GetUserSubscribeTrafficLogsResponse{
		Total: total,
		List:  userRespList,
	}, nil
}
