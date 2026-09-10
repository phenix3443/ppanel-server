package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetVerifyConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetVerifyConfigLogic(ctx context.Context, deps Deps) *GetVerifyConfigLogic {
	return &GetVerifyConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetVerifyConfigLogic) GetVerifyConfig() (*dto.VerifyConfig, error) {
	resp := &dto.VerifyConfig{}
	// get verify config from db
	verifyConfigs, err := l.deps.System.GetVerifyConfig(l.ctx)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get verify config failed: %v", err.Error())
	}
	// reflect to response
	config.SystemConfigSliceReflectToStruct(verifyConfigs, resp)
	// update verify config to system
	l.deps.reinit("verify")
	return resp, nil
}
