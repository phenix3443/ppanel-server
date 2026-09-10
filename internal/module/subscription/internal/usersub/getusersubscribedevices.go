package usersub

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserSubscribeDevicesLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user subcribe devices
func newGetUserSubscribeDevicesLogic(ctx context.Context, deps Deps) *GetUserSubscribeDevicesLogic {
	return &GetUserSubscribeDevicesLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserSubscribeDevicesLogic) GetUserSubscribeDevices(req *dto.GetUserSubscribeDevicesRequest) (resp *dto.GetUserSubscribeDevicesResponse, err error) {
	list, total, err := l.deps.Devices.QueryDevicePageList(l.ctx, req.UserId, req.SubscribeId, req.Page, req.Size)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetUserSubscribeDevices failed: %v", err.Error())
	}
	userRespList := make([]dto.SubscriptionUserDeviceSnapshot, 0)
	mapping.DeepCopy(&userRespList, list)
	return &dto.GetUserSubscribeDevicesResponse{
		Total: total,
		List:  userRespList,
	}, nil
}
