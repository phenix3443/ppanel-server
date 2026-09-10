package authmethodadmin

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetAuthMethodConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetAuthMethodConfigLogic Get auth method config
func newGetAuthMethodConfigLogic(ctx context.Context, deps Deps) *GetAuthMethodConfigLogic {
	return &GetAuthMethodConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetAuthMethodConfigLogic) GetAuthMethodConfig(req *dto.GetAuthMethodConfigRequest) (resp *dto.AuthMethodConfig, err error) {
	method, err := l.deps.Auths.FindOneByMethod(l.ctx, req.Method)
	if err != nil {
		l.Errorw("find one by method failed", logger.Field("method", req.Method), logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find one by method failed: %v", err.Error())
	}

	resp = new(dto.AuthMethodConfig)
	mapping.DeepCopy(resp, method)
	if method.Config != "" {
		if err := json.Unmarshal([]byte(method.Config), &resp.Config); err != nil {
			l.Errorw("unmarshal config failed", logger.Field("config", method.Config), logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal apple config failed: %v", err.Error())
		}
	}
	return
}
