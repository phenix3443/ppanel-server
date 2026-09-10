package serverapi

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/trafficagg"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
)

//goland:noinspection GoNameStartsWithPackageName
type ServerPushUserTrafficLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewServerPushUserTrafficLogic Push user Traffic
func newServerPushUserTrafficLogic(ctx context.Context, deps Deps) *ServerPushUserTrafficLogic {
	return &ServerPushUserTrafficLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ServerPushUserTrafficLogic) ServerPushUserTraffic(req *dto.ServerPushUserTrafficRequest) error {
	// Find server info
	serverInfo, err := l.deps.Store.Node().FindOneServer(l.ctx, req.ServerId)
	if err != nil {
		l.Errorw("[PushOnlineUsers] FindOne error", logger.Field("error", err))
		return errors.New("server not found")
	}

	if err = trafficagg.New(trafficagg.Deps{
		Store: l.deps.Store,
		Usage: l.deps.TrafficUsage,
		Redis: l.deps.Redis,
		TrafficReportThreshold: func() int64 {
			return l.deps.Config().Node.TrafficReportThreshold
		},
		Multiplier: l.deps.Multiplier,
	}).AddReport(l.ctx, serverInfo, req.Protocol, dtoTrafficToAggregator(req.Traffic)); err != nil {
		l.Errorw("[ServerPushUserTraffic] Aggregate traffic error", logger.Field("error", err.Error()))
		return errors.Wrap(err, "aggregate traffic")
	}
	return nil
}

func dtoTrafficToAggregator(items []dto.UserTraffic) []trafficagg.UserTraffic {
	if len(items) == 0 {
		return nil
	}
	result := make([]trafficagg.UserTraffic, 0, len(items))
	for _, item := range items {
		result = append(result, trafficagg.UserTraffic{
			SID:      item.SID,
			Upload:   item.Upload,
			Download: item.Download,
		})
	}
	return result
}
