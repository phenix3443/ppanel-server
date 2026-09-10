package app

import (
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// NewStore assembles the shared store from the module-owned repo builders
// (falling back to the repository package's legacy builders for
// implementations that have not migrated yet). One connection pool, module-
// owned persistence (ADR-001 step-6 preparation).
func NewStore(db *gorm.DB, rds *redis.Client) *repository.GormStore {
	return repository.NewGormStoreWithBuilders(db, rds, repository.Builders{
		Platform:     platform.NewRepoBuilder(),
		Billing:      billing.NewRepoBuilder(),
		Subscription: subscription.NewRepoBuilder(),
		Identity:     identity.NewRepoBuilder(),
		Network:      network.NewRepoBuilder(rds),
		Support:      support.NewRepoBuilder(),
		Notification: notification.NewRepoBuilder(),
	})
}
