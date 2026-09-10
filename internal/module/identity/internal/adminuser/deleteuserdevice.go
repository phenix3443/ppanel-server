package adminuser

import (
	"context"

	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/internal/devicestate"
	"github.com/perfect-panel/server/pkg/logger"
)

type DeleteUserDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Delete user device
func newDeleteUserDeviceLogic(ctx context.Context, deps Deps) *DeleteUserDeviceLogic {
	return &DeleteUserDeviceLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeleteUserDeviceLogic) DeleteUserDevice(req *dto.DeleteUserDeivceRequest) error {
	device, err := devicestate.Delete(l.ctx, l.deps.Store, l.deps.Redis, req.Id, 0)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete user error: %v", err.Error())
	}
	if device != nil {
		l.deps.kickDevice(device.UserId, device.Identifier)
	}
	return nil
}
