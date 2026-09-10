// Package dashboard implements the admin console subdomain of the platform
// module: cross-domain reporting aggregates. Every foreign-domain access is a
// read through a port satisfied structurally by the legacy repositories.
package dashboard

import (
	"context"
	"time"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Read ports onto the billing, identity, support and network domains.
type (
	OrderStatsReader   = repository.OrderRepo
	UserStatsReader    = repository.UserRepo
	TicketStatsReader  = repository.TicketRepo
	NodeStatsReader    = repository.NodeRepo
	TrafficStatsReader = repository.TrafficRepo
)

// Cache is the dashboard's snapshot cache; the redis client satisfies it
// structurally.
type Cache interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

type Deps struct {
	Orders  OrderStatsReader
	Users   UserStatsReader
	Tickets TicketStatsReader
	Nodes   NodeStatsReader
	Traffic TrafficStatsReader
	Logs    repository.LogRepo
	Cache   Cache
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) QueryRevenueStatistics(ctx context.Context) (*dto.RevenueStatisticsResponse, error) {
	return newQueryRevenueStatisticsLogic(ctx, s.deps).QueryRevenueStatistics()
}

func (s *Service) QueryServerTotalData(ctx context.Context) (*dto.ServerTotalDataResponse, error) {
	return newQueryServerTotalDataLogic(ctx, s.deps).QueryServerTotalData()
}

func (s *Service) QueryTicketWaitReply(ctx context.Context) (*dto.TicketWaitRelpyResponse, error) {
	return newQueryTicketWaitReplyLogic(ctx, s.deps).QueryTicketWaitReply()
}

func (s *Service) QueryUserStatistics(ctx context.Context) (*dto.UserStatisticsResponse, error) {
	return newQueryUserStatisticsLogic(ctx, s.deps).QueryUserStatistics()
}
