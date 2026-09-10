package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetRegisterConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetRegisterConfigLogic(ctx context.Context, deps Deps) *GetRegisterConfigLogic {
	return &GetRegisterConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetRegisterConfigLogic) GetRegisterConfig() (*dto.RegisterConfig, error) {
	resp := &dto.RegisterConfig{}

	// get register config from database
	configs, err := l.deps.System.GetRegisterConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetRegisterConfig] Database query error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get register config error: %v", err.Error())
	}

	// reflect to response
	config.SystemConfigSliceReflectToStruct(configs, resp)
	return resp, nil
}
