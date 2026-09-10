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

type GetAuthMethodListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetAuthMethodListLogic Get auth method list
func newGetAuthMethodListLogic(ctx context.Context, deps Deps) *GetAuthMethodListLogic {
	return &GetAuthMethodListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetAuthMethodListLogic) GetAuthMethodList() (resp *dto.GetAuthMethodListResponse, err error) {
	methods, err := l.deps.Auths.FindAll(l.ctx)
	if err != nil {
		l.Errorw("find all failed", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find all failed: %v", err.Error())
	}
	var list []dto.AuthMethodConfig
	for _, method := range methods {
		var item dto.AuthMethodConfig
		mapping.DeepCopy(&item, method)
		if method.Config != "" {
			if err := json.Unmarshal([]byte(method.Config), &item.Config); err != nil {
				l.Errorw("unmarshal config failed", logger.Field("config", method.Config), logger.Field("error", err.Error()))
				return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal config failed: %v", err.Error())
			}
		}
		list = append(list, item)
	}
	return &dto.GetAuthMethodListResponse{List: list}, nil
}
