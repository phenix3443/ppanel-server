package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type UpdateCurrencyConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update Currency Config
func newUpdateCurrencyConfigLogic(ctx context.Context, deps Deps) *UpdateCurrencyConfigLogic {
	return &UpdateCurrencyConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateCurrencyConfigLogic) UpdateCurrencyConfig(req *dto.CurrencyConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "currency", convertedConfigFields(*req))
	l.deps.reinit("currency")
	if err != nil {
		l.Errorw("[UpdateCurrencyConfig] update currency config error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update invite config error: %v", err)
	}
	return nil
}
