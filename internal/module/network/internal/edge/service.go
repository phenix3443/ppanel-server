// Service assembly for the edge subdomain of the network module: the token-
// authenticated client manifest. Only the module facade may reach it.
package edge

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// Snapshot is the per-request view of the runtime-mutable settings the edge
// manifest consumes.
type Snapshot struct {
	Subscribe config.SubscribeConfig
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Store Store
	// Config snapshots the runtime-mutable settings per request.
	Config func() Snapshot
}

// Service is the edge manifest entry point used by the network facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) Manifest(ctx context.Context, token string) (*dto.EdgeManifestResponse, error) {
	return newManifestLogic(ctx, s.deps).Manifest(token)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	Node() repository.NodeRepo
	Subscribe() repository.SubscribeRepo
	User() repository.UserRepo
	UserSubscription() repository.UserSubscriptionRepo
}
