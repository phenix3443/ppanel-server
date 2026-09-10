package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserAuthMethodLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user auth method
func newGetUserAuthMethodLogic(ctx context.Context, deps Deps) *GetUserAuthMethodLogic {
	return &GetUserAuthMethodLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserAuthMethodLogic) GetUserAuthMethod(req *dto.GetUserAuthMethodRequest) (resp *dto.GetUserAuthMethodResponse, err error) {
	methods, err := l.deps.UserAuths.FindUserAuthMethods(l.ctx, req.UserId)
	if err != nil {
		l.Errorw("[GetUserAuthMethodLogic] Get User Auth Method Error:", logger.Field("err", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Get User Auth Method Error")
	}
	list := make([]dto.UserAuthMethod, 0)
	mapping.DeepCopy(&list, methods)

	return &dto.GetUserAuthMethodResponse{
		AuthMethods: list,
	}, nil
}
