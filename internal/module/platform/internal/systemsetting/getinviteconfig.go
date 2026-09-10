package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetInviteConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetInviteConfigLogic(ctx context.Context, deps Deps) *GetInviteConfigLogic {
	return &GetInviteConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetInviteConfigLogic) GetInviteConfig() (*dto.InviteConfig, error) {
	resp := &dto.InviteConfig{}
	// get invite config from db
	configs, err := l.deps.System.GetInviteConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetInviteConfigLogic] get invite config error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get invite config error: %v", err.Error())
	}
	// reflect to response
	config.SystemConfigSliceReflectToStruct(configs, resp)

	return resp, nil
}
