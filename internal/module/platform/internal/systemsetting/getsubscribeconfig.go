package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetSubscribeConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetSubscribeConfigLogic(ctx context.Context, deps Deps) *GetSubscribeConfigLogic {
	return &GetSubscribeConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetSubscribeConfigLogic) GetSubscribeConfig() (resp *dto.SubscribeConfig, err error) {
	resp = &dto.SubscribeConfig{}
	// get subscribe config from db
	subscribeConfigs, err := l.deps.System.GetSubscribeConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetSubscribeConfig] Database query error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get subscribe config failed: %v", err.Error())
	}

	// reflect to response
	config.SystemConfigSliceReflectToStruct(subscribeConfigs, resp)
	return resp, nil
}
