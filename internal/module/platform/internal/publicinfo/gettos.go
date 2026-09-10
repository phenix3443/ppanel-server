package publicinfo

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetTosLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Tos
func newGetTosLogic(ctx context.Context, deps Deps) *GetTosLogic {
	return &GetTosLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetTosLogic) GetTos() (resp *dto.GetTosResponse, err error) {
	resp = &dto.GetTosResponse{}
	// get Tos config from db
	configs, err := l.deps.Store.System().GetTosConfig(l.ctx)
	if err != nil {
		l.Errorw("[GetTosLogic] GetTos error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetTos error: %v", err.Error())
	}
	// reflect to response
	config.SystemConfigSliceReflectToStruct(configs, resp)
	return
}
