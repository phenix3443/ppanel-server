package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserNotifyLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update User Notify
func newUpdateUserNotifyLogic(ctx context.Context, deps Deps) *UpdateUserNotifyLogic {
	return &UpdateUserNotifyLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserNotifyLogic) UpdateUserNotify(req *dto.UpdateUserNotifyRequest) error {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if u.Id == 0 {
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "user not login")
	}
	u.EnableLoginNotify = req.EnableLoginNotify
	u.EnableBalanceNotify = req.EnableBalanceNotify
	u.EnableSubscribeNotify = req.EnableSubscribeNotify
	u.EnableTradeNotify = req.EnableTradeNotify
	if err := l.deps.Users.Update(l.ctx, u); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "update user notify error: %v", err.Error())
	}
	return nil
}
