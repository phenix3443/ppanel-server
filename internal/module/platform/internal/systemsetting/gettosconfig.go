package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetTosConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetTosConfigLogic(ctx context.Context, deps Deps) *GetTosConfigLogic {
	return &GetTosConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetTosConfigLogic) GetTosConfig() (resp *dto.TosConfig, err error) {
	resp = &dto.TosConfig{}
	// get tos config from db
	configs, err := l.deps.System.GetTosConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetTosConfig] GetTosConfig error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetTosConfig error: %v", err.Error())
	}
	// reflect to response
	config.SystemConfigSliceReflectToStruct(configs, resp)
	return
}
