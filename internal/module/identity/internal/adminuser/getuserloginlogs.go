package adminuser

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserLoginLogsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user login logs
func newGetUserLoginLogsLogic(ctx context.Context, deps Deps) *GetUserLoginLogsLogic {
	return &GetUserLoginLogsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserLoginLogsLogic) GetUserLoginLogs(req *dto.GetUserLoginLogsRequest) (resp *dto.GetUserLoginLogsResponse, err error) {
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeLogin.Uint8(),
		ObjectID: req.UserId,
	})
	if err != nil {
		l.Errorw("[GetUserLoginLogs] get user login logs failed", logger.Field("error", err.Error()), logger.Field("request", req))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get user login logs failed: %v", err.Error())
	}
	var list []dto.UserLoginLog

	for _, datum := range data {
		var content log.Login
		if err = content.Unmarshal([]byte(datum.Content)); err != nil {
			l.Errorf("[GetUserLoginLogs] unmarshal login log content failed: %v", err.Error())
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt login log %d: %v", datum.Id, err)
		}
		list = append(list, dto.UserLoginLog{
			Id:               datum.Id,
			UserId:           datum.ObjectID,
			LoginIP:          content.LoginIP,
			UserAgent:        content.UserAgent,
			Success:          content.Success,
			Timestamp:        datum.CreatedAt.UnixMilli(),
			ActorID:          content.ActorID,
			IPCountryCode:    content.IPCountryCode,
			IPCountry:        content.IPCountry,
			IPRegion:         content.IPRegion,
			IPCity:           content.IPCity,
			IPASN:            content.IPASN,
			IPASOrganization: content.IPASOrganization,
		})
	}

	return &dto.GetUserLoginLogsResponse{
		Total: total,
		List:  list,
	}, nil
}
