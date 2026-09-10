package publicinfo

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type HeartbeatLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewHeartbeatLogic Heartbeat
func newHeartbeatLogic(ctx context.Context, deps Deps) *HeartbeatLogic {
	return &HeartbeatLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *HeartbeatLogic) Heartbeat() (resp *dto.HeartbeatResponse, err error) {
	return &dto.HeartbeatResponse{
		Status:    true,
		Message:   "service is alive",
		Timestamp: timeutil.Now().Unix(),
	}, nil
}
