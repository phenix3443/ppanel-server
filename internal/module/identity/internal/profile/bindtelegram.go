package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// telegramBindTokenTTL bounds how long a deep link stays usable. The value is
// the expiry advertised to the client, so the two can no longer drift.
const telegramBindTokenTTL = 300 * time.Second

type BindTelegramLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Bind Telegram
func newBindTelegramLogic(ctx context.Context, deps Deps) *BindTelegramLogic {
	return &BindTelegramLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *BindTelegramLogic) BindTelegram() (resp *dto.BindTelegramResponse, err error) {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok || u == nil {
		l.Errorw("bind telegram failed: user missing from context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if l.deps.TelegramBotName() == "" {
		l.Errorw("bind telegram failed: telegram bot is not initialized")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "telegram bot is not configured")
	}

	// The deep link carries a dedicated single-use token rather than the
	// caller's session id: the link travels through Telegram chats and
	// screenshots, and a leaked session id would let its holder bind their
	// own Telegram account — and therefore log in — as this user.
	token := random.KeyNew(32, 1)
	expiredAt := timeutil.Now().Add(telegramBindTokenTTL)
	key := fmt.Sprintf("%s:%s", config.TelegramBindKey, token)
	if err := l.deps.Redis.Set(l.ctx, key, u.Id, telegramBindTokenTTL).Err(); err != nil {
		l.Errorw("bind telegram failed: cannot store bind token",
			logger.Field("user_id", u.Id),
			logger.Field("error", err.Error()),
		)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "store telegram bind token failed: %v", err)
	}

	return &dto.BindTelegramResponse{
		Url:       fmt.Sprintf("https://t.me/%s?start=%s", l.deps.TelegramBotName(), token),
		ExpiredAt: expiredAt.UnixMilli(),
	}, nil
}
