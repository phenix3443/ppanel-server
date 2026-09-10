package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
)

type GetDeviceListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Device List
func newGetDeviceListLogic(ctx context.Context, deps Deps) *GetDeviceListLogic {
	return &GetDeviceListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetDeviceListLogic) GetDeviceList() (resp *dto.GetDeviceListResponse, err error) {
	userInfo := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	list, count, err := l.deps.Devices.QueryDeviceList(l.ctx, userInfo.Id)
	userRespList := make([]dto.UserDevice, 0)
	mapping.DeepCopy(&userRespList, list)
	resp = &dto.GetDeviceListResponse{
		Total: count,
		List:  userRespList,
	}
	return
}
