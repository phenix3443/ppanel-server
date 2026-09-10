package adminuser

import (
	"context"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserAuthMethodLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update user auth method
func newUpdateUserAuthMethodLogic(ctx context.Context, deps Deps) *UpdateUserAuthMethodLogic {
	return &UpdateUserAuthMethodLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserAuthMethodLogic) UpdateUserAuthMethod(req *dto.UpdateUserAuthMethodRequest) error {
	if strings.EqualFold(strings.TrimSpace(req.AuthType), "device") {
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "use device management for device identities")
	}
	method, err := l.deps.UserAuths.FindUserAuthMethodByPlatform(l.ctx, req.UserId, req.AuthType)
	if err != nil {
		l.Errorw("Get user auth method error", logger.Field("error", err.Error()), logger.Field("userId", req.UserId), logger.Field("authType", req.AuthType))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Get user auth method error: %v", err.Error())
	}
	userInfo, err := l.deps.Users.FindOne(l.ctx, req.UserId)
	if err != nil {
		l.Errorw("Get user info error", logger.Field("error", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Get user info error: %v", err.Error())
	}

	method.AuthType = req.AuthType
	method.AuthIdentifier = req.AuthIdentifier
	if err = l.deps.UserAuths.UpdateUserAuthMethods(l.ctx, method); err != nil {
		l.Errorw("Update user auth method error", logger.Field("error", err.Error()), logger.Field("userId", req.UserId), logger.Field("authType", req.AuthType))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update user auth method error: %v", err.Error())
	}
	if err = l.deps.Cache.UpdateUserCache(l.ctx, userInfo); err != nil {
		l.Errorw("Update user cache error", logger.Field("error", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Update user cache error: %v", err.Error())
	}
	return nil
}
