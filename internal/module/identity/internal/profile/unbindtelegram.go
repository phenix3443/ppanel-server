package profile

import (
	"context"
	"strconv"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UnbindTelegramLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Unbind Telegram
func newUnbindTelegramLogic(ctx context.Context, deps Deps) *UnbindTelegramLogic {
	return &UnbindTelegramLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UnbindTelegramLogic) UnbindTelegram() error {
	// Get User Info
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)

	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	method, err := l.deps.UserAuth.FindUserAuthMethodByPlatform(l.ctx, u.Id, "telegram")
	if err != nil {
		l.Errorw("UnbindTelegramLogic FindUserAuthMethodByPlatform Error", logger.Field("id", u.Id), logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find User Auth Method By Platform Failed")
	}

	userTelegramChatId, err := strconv.ParseInt(method.AuthIdentifier, 10, 64)
	if err != nil {
		l.Errorw("UnbindTelegramLogic ParseInt Error", logger.Field("id", u.Id), logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "ParseInt Error")
	}

	if userTelegramChatId == 0 {
		return errors.Wrapf(xerr.NewErrCode(xerr.TelegramNotBound), "Unbind Telegram")
	}

	// Unbind Telegram
	err = l.deps.UserAuth.DeleteUserAuthMethods(l.ctx, u.Id, "telegram")
	if err != nil {
		l.Errorw("UnbindTelegramLogic DeleteUserAuthMethods Error", logger.Field("id", u.Id), logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "Delete User Auth Methods Failed")
	}
	// The unbind notice is best-effort: the composition root renders and
	// sends it through the runtime-configured bot.
	if err := l.deps.NotifyUnbind(u.Id, userTelegramChatId); err != nil {
		l.Errorw("UnbindTelegramLogic Send Error", logger.Field("id", u.Id), logger.Field("error", err.Error()))
	}
	return nil
}
