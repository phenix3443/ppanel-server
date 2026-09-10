package usersub

import (
	"context"
	"time"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserSubscribeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateUserSubscribeLogic Update user subscribe
func newUpdateUserSubscribeLogic(ctx context.Context, deps Deps) *UpdateUserSubscribeLogic {
	return &UpdateUserSubscribeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserSubscribeLogic) UpdateUserSubscribe(req *dto.UpdateUserSubscribeRequest) error {
	userSub, err := l.deps.UserSubs.FindOneSubscribe(l.ctx, req.UserSubscribeId)
	if err != nil {
		l.Errorw("FindOneUserSubscribe failed:", logger.Field("error", err.Error()), logger.Field("userSubscribeId", req.UserSubscribeId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneUserSubscribe failed: %v", err.Error())
	}
	// ExpiredAt == 0 is the NoLimit sentinel (see tool.AddTime), not an expired epoch time
	expiredAt := time.UnixMilli(req.ExpiredAt)
	if req.ExpiredAt != 0 && time.Since(expiredAt).Minutes() > 0 {
		userSub.Status = 3
	} else {
		userSub.Status = 1
	}

	err = l.deps.UserSubs.UpdateSubscribe(l.ctx, &usersub.Subscribe{
		Id:          userSub.Id,
		UserId:      userSub.UserId,
		OrderId:     userSub.OrderId,
		SubscribeId: req.SubscribeId,
		StartTime:   userSub.StartTime,
		ExpireTime:  time.UnixMilli(req.ExpiredAt),
		Traffic:     req.Traffic,
		Download:    req.Download,
		Upload:      req.Upload,
		Token:       userSub.Token,
		UUID:        userSub.UUID,
		Status:      userSub.Status,
	})

	if err != nil {
		l.Errorw("UpdateSubscribe failed:", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "UpdateSubscribe failed: %v", err.Error())
	}
	// Clear user subscribe cache
	if err = l.deps.Cache.ClearSubscribeCache(l.ctx, userSub); err != nil {
		l.Errorw("ClearSubscribeCache failed:", logger.Field("error", err.Error()), logger.Field("userSubscribeId", userSub.Id))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "ClearSubscribeCache failed: %v", err.Error())
	}
	// Clear subscribe cache
	if err = l.deps.Plans.ClearCache(l.ctx, userSub.SubscribeId); err != nil {
		l.Errorw("failed to clear subscribe cache", logger.Field("error", err.Error()), logger.Field("subscribeId", userSub.SubscribeId))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "failed to clear subscribe cache: %v", err.Error())
	}
	return nil
}
