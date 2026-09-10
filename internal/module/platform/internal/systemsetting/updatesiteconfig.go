package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateSiteConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newUpdateSiteConfigLogic(ctx context.Context, deps Deps) *UpdateSiteConfigLogic {
	return &UpdateSiteConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateSiteConfigLogic) UpdateSiteConfig(req *dto.SiteConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "site", stringConfigFields(*req))
	if err != nil {
		l.Logger.Error("[UpdateSiteConfig] update site config error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update site config error: %v", err.Error())
	}
	l.deps.reinit("site")
	return nil
}
