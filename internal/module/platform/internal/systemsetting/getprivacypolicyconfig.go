package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetPrivacyPolicyConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetPrivacyPolicyConfigLogic get Privacy Policy Config
func newGetPrivacyPolicyConfigLogic(ctx context.Context, deps Deps) *GetPrivacyPolicyConfigLogic {
	return &GetPrivacyPolicyConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetPrivacyPolicyConfigLogic) GetPrivacyPolicyConfig() (resp *dto.PrivacyPolicyConfig, err error) {
	resp = &dto.PrivacyPolicyConfig{}
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
