package profile

import (
	"context"
	"strings"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UnbindOAuthLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Unbind OAuth
func newUnbindOAuthLogic(ctx context.Context, deps Deps) *UnbindOAuthLogic {
	return &UnbindOAuthLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UnbindOAuthLogic) UnbindOAuth(req *dto.UnbindOAuthRequest) error {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if !l.validator(req) {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid parameter")
	}
	err := l.deps.UserAuth.DeleteUserAuthMethods(l.ctx, u.Id, req.Method)
	if err != nil {
		l.Errorw("delete user auth methods failed:", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete user auth methods failed: %v", err.Error())
	}

	return nil
}
func (l *UnbindOAuthLogic) validator(req *dto.UnbindOAuthRequest) bool {
	switch strings.ToLower(strings.TrimSpace(req.Method)) {
	case "google", "apple", "github", "facebook", "telegram":
		return true
	default:
		return false
	}
}
