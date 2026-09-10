package publicinfo

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetPrivacyPolicyLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Privacy Policy
func newGetPrivacyPolicyLogic(ctx context.Context, deps Deps) *GetPrivacyPolicyLogic {
	return &GetPrivacyPolicyLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetPrivacyPolicyLogic) GetPrivacyPolicy() (resp *dto.PrivacyPolicyConfig, err error) {
	resp = &dto.PrivacyPolicyConfig{}
	// get tos config from db
	configs, err := l.deps.Store.System().GetTosConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetTosConfig] GetTosConfig error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetTosConfig error: %v", err.Error())
	}
	// reflect to response
	config.SystemConfigSliceReflectToStruct(configs, resp)
	return
}
