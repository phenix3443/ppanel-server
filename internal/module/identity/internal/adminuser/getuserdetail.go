package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserDetailLogic struct {
	ctx  context.Context
	deps Deps
	logger.Logger
}

func newGetUserDetailLogic(ctx context.Context, deps Deps) *GetUserDetailLogic {
	return &GetUserDetailLogic{
		ctx:    ctx,
		deps:   deps,
		Logger: logger.WithContext(ctx),
	}
}

func (l *GetUserDetailLogic) GetUserDetail(req *dto.GetDetailRequest) (*dto.User, error) {
	resp := dto.User{}
	userInfo, err := l.deps.Users.FindOne(l.ctx, req.Id)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get user detail error: %v", err.Error())
	}
	mapping.DeepCopy(&resp, userInfo)
	// Wallet values come from the billing-owned table. A read failure must
	// fail the request: this response populates the admin edit form, and a
	// silently zeroed balance would round-trip into a real adjustment.
	w, err := l.deps.Wallet.FindWallet(l.ctx, userInfo.Id)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "load user wallet error: %v", err.Error())
	}
	if w != nil {
		resp.Balance = w.Balance
		resp.GiftAmount = w.GiftAmount
		resp.Commission = w.Commission
	}
	return &resp, nil
}
