package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/devicestate"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UnbindDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Unbind Device
func newUnbindDeviceLogic(ctx context.Context, deps Deps) *UnbindDeviceLogic {
	return &UnbindDeviceLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UnbindDeviceLogic) UnbindDevice(req *dto.UnbindDeviceRequest) error {
	userInfo := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	device, err := l.deps.Devices.FindDeviceForAuth(l.ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DeviceNotExist), "find device")
	}

	if device.UserId != userInfo.Id {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "device not belong to user")
	}

	removed, err := devicestate.Delete(l.ctx, l.deps.Store, l.deps.Redis, req.Id, userInfo.Id)
	if err != nil {
		return err
	}
	if removed != nil && l.deps.KickDevice != nil {
		l.deps.KickDevice(removed.UserId, removed.Identifier)
	}
	return nil
}
