package usersub

import (
	"context"

	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserSubscribeResetTrafficLogsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user subcribe reset traffic logs
func newGetUserSubscribeResetTrafficLogsLogic(ctx context.Context, deps Deps) *GetUserSubscribeResetTrafficLogsLogic {
	return &GetUserSubscribeResetTrafficLogsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserSubscribeResetTrafficLogsLogic) GetUserSubscribeResetTrafficLogs(req *dto.GetUserSubscribeResetTrafficLogsRequest) (resp *dto.GetUserSubscribeResetTrafficLogsResponse, err error) {
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeResetSubscribe.Uint8(),
		ObjectID: req.UserSubscribeId,
	})
	if err != nil {
		l.Errorf("[ResetSubscribeTrafficLog] failed to filter system log: %v", err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FilterSystemLog failed, err: %v", err)
	}

	var list []dto.ResetSubscribeTrafficLog

	for _, item := range data {
		var content log.ResetSubscribe
		if err = content.Unmarshal([]byte(item.Content)); err != nil {
			l.Errorf("[ResetSubscribeTrafficLog] failed to unmarshal log: %v", err)
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt reset subscription log %d: %v", item.Id, err)
		}
		list = append(list, dto.ResetSubscribeTrafficLog{
			Id:               item.Id,
			Type:             content.Type,
			OrderNo:          content.OrderNo,
			Timestamp:        content.Timestamp,
			UserSubscribeId:  item.ObjectID,
			ClientIP:         content.ClientIP,
			UserAgent:        content.UserAgent,
			ActorID:          content.ActorID,
			IPCountryCode:    content.IPCountryCode,
			IPCountry:        content.IPCountry,
			IPRegion:         content.IPRegion,
			IPCity:           content.IPCity,
			IPASN:            content.IPASN,
			IPASOrganization: content.IPASOrganization,
		})
	}

	return &dto.GetUserSubscribeResetTrafficLogsResponse{
		Total: total,
		List:  list,
	}, nil
}
