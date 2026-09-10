package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetSiteConfigLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newGetSiteConfigLogic(ctx context.Context, deps Deps) *GetSiteConfigLogic {
	return &GetSiteConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetSiteConfigLogic) GetSiteConfig() (resp *dto.SiteConfig, err error) {
	resp = &dto.SiteConfig{}
	// get site config from db
	siteConfigs, err := l.deps.System.GetSiteConfig(l.ctx)
	if err != nil {
		l.Logger.Error("[GetSiteConfig] Database query error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get site config failed: %v", err.Error())
	}
	// reflect to response
	config.SystemConfigSliceReflectToStruct(siteConfigs, resp)
	return resp, nil
}
