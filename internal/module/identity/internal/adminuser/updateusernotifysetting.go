package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserNotifySettingLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateUserNotifySettingLogic Update user notify setting
func newUpdateUserNotifySettingLogic(ctx context.Context, deps Deps) *UpdateUserNotifySettingLogic {
	return &UpdateUserNotifySettingLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserNotifySettingLogic) UpdateUserNotifySetting(req *dto.UpdateUserNotifySettingRequest) error {
	userInfo, err := l.deps.Users.FindOne(l.ctx, req.UserId)
	if err != nil {
		l.Errorw("[UpdateUserNotifySettingLogic] Find User Error:", logger.Field("err", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find User Error")
	}
	mapping.DeepCopy(userInfo, req)
	err = l.deps.Users.Update(l.ctx, userInfo)
	if err != nil {
		l.Errorw("[UpdateUserNotifySettingLogic] Update User Error:", logger.Field("err", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update User Error")
	}
	return nil
}
