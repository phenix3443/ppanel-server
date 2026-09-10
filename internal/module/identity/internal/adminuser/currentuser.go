package adminuser

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

type CurrentUserLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newCurrentUserLogic(ctx context.Context, deps Deps) *CurrentUserLogic {
	return &CurrentUserLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *CurrentUserLogic) CurrentUser() (*dto.User, error) {
	resp := &dto.User{}
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}

	l.Logger.Infow("current user", logger.Field("user_id", u.Id))
	mapping.DeepCopy(resp, u)
	// The context user is the middleware's cached identity row; wallet
	// values come from the billing-owned table, and a read failure fails
	// the request rather than rendering zero balances.
	w, err := l.deps.Wallet.FindWallet(l.ctx, u.Id)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "load user wallet error: %v", err.Error())
	}
	if w != nil {
		resp.Balance = w.Balance
		resp.GiftAmount = w.GiftAmount
		resp.Commission = w.Commission
	}
	return resp, nil
}
