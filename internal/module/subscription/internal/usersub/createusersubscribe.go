package usersub

import (
	"context"
	"fmt"
	"time"

	"uuid"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type CreateUserSubscribeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Create user subcribe
func newCreateUserSubscribeLogic(ctx context.Context, deps Deps) *CreateUserSubscribeLogic {
	return &CreateUserSubscribeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CreateUserSubscribeLogic) CreateUserSubscribe(req *dto.CreateUserSubscribeRequest) error {
	// validate user
	userInfo, err := l.deps.Users.FindOne(l.ctx, req.UserId)
	if err != nil {
		l.Errorw("FindOne error", logger.Field("error", err.Error()), logger.Field("userId", req.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOne error: %v", err.Error())
	}
	if l.deps.SingleModel() {
		hasBlockingSubscription, err := l.deps.UserSubs.HasBlockingSubscription(l.ctx, req.UserId)
		if err != nil {
			l.Errorw("HasBlockingSubscription error", logger.Field("error", err.Error()), logger.Field("userId", req.UserId))
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "check user subscription error: %v", err.Error())
		}
		if hasBlockingSubscription {
			return errors.Wrapf(xerr.NewErrCode(xerr.SingleSubscribeModeExceedsLimit), "Single subscribe mode exceeds limit")
		}
	}
	sub, err := l.deps.Plans.FindOne(l.ctx, req.SubscribeId)
	if err != nil {
		l.Errorw("FindOne error", logger.Field("error", err.Error()), logger.Field("subscribeId", req.SubscribeId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOne error: %v", err.Error())
	}
	if req.Traffic == 0 {
		req.Traffic = sub.Traffic
	}

	userSub := usersub.Subscribe{
		UserId:      req.UserId,
		SubscribeId: req.SubscribeId,
		StartTime:   timeutil.Now(),
		ExpireTime:  time.UnixMilli(req.ExpiredAt),
		Traffic:     req.Traffic,
		Download:    0,
		Upload:      0,
		Token:       usersub.TokenFromOrder(fmt.Sprintf("adminCreate:%d", timeutil.Now().UnixMilli())),
		UUID:        uuid.NewV4().String(),
		Status:      usersub.SubscribeStatusActive,
	}
	if err = l.deps.UserSubs.InsertSubscribe(l.ctx, &userSub); err != nil {
		l.Errorw("InsertSubscribe error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "InsertSubscribe error: %v", err.Error())
	}

	err = l.deps.Cache.UpdateUserCache(l.ctx, userInfo)
	if err != nil {
		l.Errorw("UpdateUserCache error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "UpdateUserCache error: %v", err.Error())
	}

	err = l.deps.Plans.ClearCache(l.ctx, userSub.SubscribeId)
	if err != nil {
		logger.Errorw("ClearSubscribe error", logger.Field("error", err.Error()))
	}
	return nil
}
