package usersub

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetUserSubscribeByIdLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get user subcribe by id
func newGetUserSubscribeByIdLogic(ctx context.Context, deps Deps) *GetUserSubscribeByIdLogic {
	return &GetUserSubscribeByIdLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetUserSubscribeByIdLogic) GetUserSubscribeById(req *dto.GetUserSubscribeByIdRequest) (resp *dto.UserSubscribeDetail, err error) {
	sub, err := l.deps.UserSubs.FindOneSubscribeDetailsById(l.ctx, req.Id)
	if err != nil {
		l.Errorw("[GetUserSubscribeByIdLogic] FindOneSubscribeDetailsById error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneSubscribeDetailsById error: %v", err.Error())
	}
	var subscribeDetails dto.UserSubscribeDetail
	mapping.DeepCopy(&subscribeDetails, sub)
	// The identity row is composed at the module layer instead of a
	// cross-domain preload (ADR-001 step 5).
	owner, err := l.deps.Users.FindOne(l.ctx, sub.UserId)
	if err != nil {
		l.Errorw("[GetUserSubscribeByIdLogic] load subscription owner failed",
			logger.Field("error", err.Error()), logger.Field("user_id", sub.UserId))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "load subscription owner error: %v", err.Error())
	}
	mapping.DeepCopy(&subscribeDetails.User, owner)
	return &subscribeDetails, nil
}
