package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/devicesession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type KickOfflineByUserDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// kick offline user device
func newKickOfflineByUserDeviceLogic(ctx context.Context, deps Deps) *KickOfflineByUserDeviceLogic {
	return &KickOfflineByUserDeviceLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *KickOfflineByUserDeviceLogic) KickOfflineByUserDevice(req *dto.KickOfflineRequest) error {
	device, err := l.deps.Devices.FindDeviceForAuth(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get Device  error: %v", err.Error())
	}
	if err := devicesession.Revoke(l.ctx, l.deps.Redis, device.Id); err != nil {
		return err
	}
	l.deps.kickDevice(device.UserId, device.Identifier)
	err = l.deps.Devices.SetDeviceOnline(l.ctx, device.Id, false)
	if err != nil {
		l.Logger.Error("[KickOfflineByUserDeviceLogic] Update Device Error:", logger.Field("err", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update Device error: %v", err.Error())
	}

	return nil
}
