package traffic

import (
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

type Dependencies struct {
	Store      repository.Store
	Redis      *redis.Client
	Queue      *taskqueue.Client
	Log        func() config.Log
	Aggregator network.TrafficAggregatorDeps
}
