package selfsub

import (
	"context"
	"uuid"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type ResetUserSubscribeTokenLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewResetUserSubscribeTokenLogic Reset User Subscribe Token
func newResetUserSubscribeTokenLogic(ctx context.Context, deps Deps) *ResetUserSubscribeTokenLogic {
	return &ResetUserSubscribeTokenLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ResetUserSubscribeTokenLogic) ResetUserSubscribeToken(req *dto.ResetUserSubscribeTokenRequest) error {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	userSub, err := l.deps.UserSubs.FindOneUserSubscribe(l.ctx, req.UserSubscribeId)
	if err != nil {
		l.Errorw("FindOneUserSubscribe failed:", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneUserSubscribe failed: %v", err.Error())
	}
	if userSub.UserId != u.Id {
		l.Errorw("UserSubscribeId does not belong to the current user")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "UserSubscribeId does not belong to the current user")
	}

	var orderDetails *order.Details
	// find order
	if userSub.OrderId != 0 {
		orderDetails, err = l.deps.Orders.FindOneDetails(l.ctx, userSub.OrderId)
		if err != nil {
			l.Errorw("FindOneDetails failed:", logger.Field("error", err.Error()))
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneDetails failed: %v", err.Error())
		}
	} else {
		// if order id is 0, this a admin create user subscribe
		orderDetails = &order.Details{}
	}

	userSub.Token = usersub.TokenFromOrder(orderDetails.OrderNo + timeutil.Now().Format("20060102150405.000"))
	userSub.UUID = uuid.NewV4().String()
	var newSub usersub.Subscribe
	mapping.DeepCopy(&newSub, userSub)

	err = l.deps.UserSubs.UpdateSubscribe(l.ctx, &newSub)
	if err != nil {
		l.Errorw("UpdateSubscribe failed:", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "UpdateSubscribe failed: %v", err.Error())
	}
	//clear user subscription cache
	if err = l.deps.Cache.ClearSubscribeCache(l.ctx, &newSub); err != nil {
		l.Errorw("ClearSubscribeCache failed", logger.Field("error", err.Error()), logger.Field("userSubscribeId", userSub.Id))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "ClearSubscribeCache failed: %v", err.Error())
	}
	// Clear subscription cache
	if err = l.deps.Plans.ClearCache(l.ctx, userSub.SubscribeId); err != nil {
		l.Errorw("ClearSubscribeCache failed", logger.Field("error", err.Error()), logger.Field("subscribeId", userSub.SubscribeId))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "ClearSubscribeCache failed: %v", err.Error())
	}

	return nil
}
