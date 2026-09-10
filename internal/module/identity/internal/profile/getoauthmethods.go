package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetOAuthMethodsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get OAuth Methods
func newGetOAuthMethodsLogic(ctx context.Context, deps Deps) *GetOAuthMethodsLogic {
	return &GetOAuthMethodsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetOAuthMethodsLogic) GetOAuthMethods() (resp *dto.GetOAuthMethodsResponse, err error) {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	methods, err := l.deps.UserAuth.FindUserAuthMethods(l.ctx, u.Id)
	if err != nil {
		l.Errorw("find user auth methods failed:", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find user auth methods failed: %v", err.Error())
	}
	list := make([]dto.UserAuthMethod, 0)
	mapping.DeepCopy(&list, methods)
	return &dto.GetOAuthMethodsResponse{
		Methods: list,
	}, nil
}
