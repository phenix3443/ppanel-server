package serverapi

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type statusNodeRepo struct {
	repository.NodeRepo
	server          *node.Server
	findCalls       int
	statusCalls     int
	protocolUpdates int
	cacheClears     int
}

func (r *statusNodeRepo) FindOneServer(context.Context, int64) (*node.Server, error) {
	r.findCalls++
	copy := *r.server
	return &copy, nil
}

func (r *statusNodeRepo) UpdateStatusCache(context.Context, int64, *node.Status) error {
	r.statusCalls++
	return nil
}

func (r *statusNodeRepo) UpdateServerProtocolsIfCurrent(_ context.Context, _ int64, current, updated string, _ ...*gorm.DB) (bool, error) {
	r.protocolUpdates++
	if r.server.Protocols != current {
		return false, nil
	}
	r.server.Protocols = updated
	return true, nil
}

func (r *statusNodeRepo) ClearServerCache(context.Context, int64) error {
	r.cacheClears++
	return nil
}

type statusStore struct {
	repository.Store
	nodes repository.NodeRepo
}

func (s *statusStore) Node() repository.NodeRepo { return s.nodes }

func TestServerPushStatusDefersLastReportedDatabaseWrite(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	nodes := &statusNodeRepo{server: &node.Server{Id: 7}}
	logic := newServerPushStatusLogic(context.Background(), Deps{Store: &statusStore{nodes: nodes}, Redis: client})
	if err := logic.ServerPushStatus(&dto.ServerPushStatusRequest{ServerCommon: dto.ServerCommon{ServerId: 7}}); err != nil {
		t.Fatalf("ServerPushStatus() error = %v", err)
	}
	if nodes.findCalls != 1 || nodes.statusCalls != 1 {
		t.Fatalf("find/status calls = %d/%d, want 1/1", nodes.findCalls, nodes.statusCalls)
	}
	if nodes.protocolUpdates != 0 || nodes.cacheClears != 0 {
		t.Fatalf("ordinary heartbeat wrote protocols or cleared cache: updates=%d clears=%d", nodes.protocolUpdates, nodes.cacheClears)
	}
	if got := client.HGet(context.Background(), "traffic:server:last_reported", "7").Val(); got == "" {
		t.Fatal("heartbeat did not stage last_reported_at in Redis")
	}
}
