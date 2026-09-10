package auditlog

import (
	"context"
	"strconv"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateLogSettingLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateLogSettingLogic Update log setting
func newUpdateLogSettingLogic(ctx context.Context, deps Deps) *UpdateLogSettingLogic {
	return &UpdateLogSettingLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateLogSettingLogic) UpdateLogSetting(req *dto.LogSetting) error {
	if err := validateLogSetting(req); err != nil {
		return err
	}
	err := l.deps.Store.InPlatformTx(l.ctx, func(store repository.PlatformStore) error {
		systemStore := store.System()
		if err := systemStore.UpdateValueByCategoryKey(l.ctx, "log", "AutoClear", strconv.FormatBool(*req.AutoClear), "bool"); err != nil {
			return err
		}
		if err := systemStore.UpdateValueByCategoryKey(l.ctx, "log", "ClearDays", strconv.FormatInt(req.ClearDays, 10), "int64"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		l.Errorw("[UpdateLogSetting] update log setting error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), " update log setting error: %v", err)
	}

	if l.deps.OnLogSettingChanged != nil {
		l.deps.OnLogSettingChanged(*req.AutoClear, req.ClearDays)
	}

	return nil
}

func validateLogSetting(req *dto.LogSetting) error {
	if req == nil || req.AutoClear == nil || req.ClearDays < 1 || req.ClearDays > 3650 {
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "log retention requires auto_clear and clear_days between 1 and 3650")
	}
	return nil
}
