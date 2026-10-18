// Package adminserver is the network module's admin-side management of the
// servers and nodes: their creation, updates and removal, their sort order,
// the protocols a server offers and its node configuration overrides, and the
// node-facing caches these changes invalidate. Only the module facade may
// reach it.
package adminserver

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// Snapshot is the per-request view of the runtime-mutable node settings.
type Snapshot struct {
	Node config.NodeConfig
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Store Store
	// Subscriptions is the subscription read port (the subscription
	// facade): the subscriptions behind the online users.
	Subscriptions OnlineSubscriptionReader
	// Config snapshots the runtime-mutable node settings per request.
	Config func() Snapshot
}

// Service is the admin server-management entry point used by the network
// facade.
type Service struct {
	deps Deps
}

// NewService returns the subdomain's service over deps.
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// 以下两个是本 fork 的节点目标版本功能(SetServerTargetVersion/ListNodeVersions)，
// 上游没有对应实现。其余 Service 方法上游已内联到各自的 per-op 文件，这里不再重复。
func (s *Service) SetServerTargetVersion(ctx context.Context, req *dto.SetServerTargetVersionRequest) error {
	return newSetServerTargetVersionLogic(ctx, s.deps).SetServerTargetVersion(req)
}

func (s *Service) ListNodeVersions(ctx context.Context) (*dto.ListNodeVersionsResponse, error) {
	return newListNodeVersionsLogic(ctx, s.deps).ListNodeVersions()
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.NetworkTransactor
	Node() repository.NodeRepo
}
