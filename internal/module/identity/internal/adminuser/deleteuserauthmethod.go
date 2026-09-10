package adminuser

import (
	"context"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type DeleteUserAuthMethodLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Delete user auth method
func newDeleteUserAuthMethodLogic(ctx context.Context, deps Deps) *DeleteUserAuthMethodLogic {
	return &DeleteUserAuthMethodLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *DeleteUserAuthMethodLogic) DeleteUserAuthMethod(req *dto.DeleteUserAuthMethodRequest) error {
	if strings.EqualFold(strings.TrimSpace(req.AuthType), "device") {
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "use device management for device identities")
	}
	err := l.deps.UserAuths.DeleteUserAuthMethods(l.ctx, req.UserId, req.AuthType)
	if err != nil {
		l.Errorw("[DeleteUserAuthMethodLogic] Delete User Auth Method Error:", logger.Field("err", err.Error()), logger.Field("userId", req.UserId), logger.Field("authType", req.AuthType))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "Delete User Auth Method Error")
	}
	return nil
}
