package traffic

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/pkg/logger"
)

//goland:noinspection GoNameStartsWithPackageName
type TrafficStatisticsLogic struct {
	deps Dependencies
}

func NewTrafficStatisticsLogic(deps Dependencies) *TrafficStatisticsLogic {
	return &TrafficStatisticsLogic{
		deps: deps,
	}
}

func (l *TrafficStatisticsLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload taskqueue.TrafficStatistics
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.WithContext(ctx).Error("[TrafficStatistics] Unmarshal payload failed",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(task.Payload())),
		)
		return nil
	}
	if len(payload.Logs) == 0 {
		logger.WithContext(ctx).Error("[TrafficStatistics] Payload is empty")
		return nil
	}
	// query server info
	serverInfo, err := l.deps.Store.Node().FindOneServer(ctx, payload.ServerId)
	if err != nil {
		logger.WithContext(ctx).Error("[TrafficStatistics] Find server info failed",
			logger.Field("serverId", payload.ServerId),
			logger.Field("error", err.Error()),
		)
		return nil
	}
	if err = network.NewTrafficAggregator(l.deps.Aggregator).AddReport(ctx, serverInfo, payload.Protocol, queueTrafficToAggregator(payload.Logs)); err != nil {
		logger.WithContext(ctx).Error("[TrafficStatistics] Aggregate traffic failed",
			logger.Field("serverId", payload.ServerId),
			logger.Field("error", err.Error()),
		)
	}
	return nil
}

func queueTrafficToAggregator(items []taskqueue.UserTraffic) []network.UserTraffic {
	if len(items) == 0 {
		return nil
	}
	result := make([]network.UserTraffic, 0, len(items))
	for _, item := range items {
		result = append(result, network.UserTraffic{
			SID:      item.SID,
			Upload:   item.Upload,
			Download: item.Download,
		})
	}
	return result
}
