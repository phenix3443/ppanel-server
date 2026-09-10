package traffic

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

const trafficFlushLockKey = "traffic:flush:lock"

type FlushTrafficLogic struct {
	deps Dependencies
}

func NewFlushTrafficLogic(deps Dependencies) *FlushTrafficLogic {
	return &FlushTrafficLogic{deps: deps}
}

func (l *FlushTrafficLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	ok, err := l.deps.Redis.SetNX(ctx, trafficFlushLockKey, "locked", 55*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		logger.WithContext(ctx).Info("[FlushTraffic] another task is already running, skipping")
		return nil
	}
	defer func() {
		if err := l.deps.Redis.Del(ctx, trafficFlushLockKey).Err(); err != nil {
			logger.WithContext(ctx).Error("[FlushTraffic] release lock failed", logger.Field("error", err.Error()))
		}
	}()

	if err := network.NewTrafficAggregator(l.deps.Aggregator).FlushDueBuckets(ctx, timeutil.Now()); err != nil {
		logger.WithContext(ctx).Error("[FlushTraffic] flush traffic failed", logger.Field("error", err.Error()))
		return err
	}
	return nil
}
