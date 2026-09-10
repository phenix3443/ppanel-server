package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/devicesession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// User device
func newUpdateUserDeviceLogic(ctx context.Context, deps Deps) *UpdateUserDeviceLogic {
	return &UpdateUserDeviceLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserDeviceLogic) UpdateUserDevice(req *dto.UserDevice) error {
	device, err := l.deps.Devices.FindDeviceForAuth(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get Device  error: %v", err.Error())
	}
	if err := devicesession.Revoke(l.ctx, l.deps.Redis, device.Id); err != nil {
		return err
	}
	err = l.deps.Devices.SetDeviceEnabled(l.ctx, device.Id, req.Enabled)
	if err != nil {
		l.Logger.Error("[UpdateUserDeviceLogic] Update Device Error:", logger.Field("err", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update Device error: %v", err.Error())
	}
	if !req.Enabled {
		l.deps.kickDevice(device.UserId, device.Identifier)
	}
	return nil
}
