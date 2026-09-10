package subscription

import (
	"context"

	"github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/subscription/internal/trafficusage"
)

type TrafficUsageStore = trafficusage.Store

type TrafficUsage interface {
	ApplyBucketOnce(ctx context.Context, bucket string, deltas []traffic.SubscribeTrafficDelta) error
}

func NewTrafficUsage(store TrafficUsageStore) TrafficUsage {
	return trafficusage.New(store)
}
