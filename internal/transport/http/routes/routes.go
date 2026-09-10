package routes

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/http/middleware"
	"github.com/redis/go-redis/v9"
)

// Dependencies is the HTTP routing boundary. Each handler receives one module
// facade or a smaller endpoint-specific dependency set from this structure.
type Dependencies struct {
	Config         config.Config
	ConfigProvider func() config.Config
	Redis          *redis.Client
	Store          repository.Store
	Support        support.Service
	Billing        billing.Service
	Platform       platform.Service
	Subscription   subscription.Service
	Identity       identity.Service
	Network        network.Service
}

func (deps Dependencies) runtimeConfig() config.Config {
	if deps.ConfigProvider != nil {
		return deps.ConfigProvider()
	}
	return deps.Config
}

func (deps Dependencies) verifyConfig() config.Verify {
	return deps.runtimeConfig().Verify
}

func (deps Dependencies) subscribeConfig() config.SubscribeConfig {
	return deps.runtimeConfig().Subscribe
}

func (deps Dependencies) edgeSubscribeConfig() config.EdgeSubscribeConfig {
	return deps.runtimeConfig().EdgeSubscribe
}

func (deps Dependencies) nodeSecret() string {
	return deps.runtimeConfig().Node.NodeSecret
}

func (deps Dependencies) authMiddleware() app.HandlerFunc {
	return middleware.AuthMiddleware(middleware.AuthDeps{
		JWT: deps.runtimeConfig().JwtAuth, Redis: deps.Redis, Store: deps.Store,
	})
}

func (deps Dependencies) optionalAuthMiddleware() app.HandlerFunc {
	return middleware.OptionalAuthMiddleware(middleware.AuthDeps{
		JWT: deps.runtimeConfig().JwtAuth, Redis: deps.Redis, Store: deps.Store,
	})
}

func (deps Dependencies) deviceMiddleware() app.HandlerFunc {
	if deps.Redis == nil {
		return middleware.DeviceMiddleware(func() config.DeviceConfig { return deps.runtimeConfig().Device }, nil)
	}
	return middleware.DeviceMiddleware(func() config.DeviceConfig { return deps.runtimeConfig().Device }, deps.Redis)
}

func RegisterHandlers(router *server.Hertz, deps Dependencies) {
	registerEdgeRoutes(router, deps)
	registerSubscribeConfigRoutes(router, deps)
	registerServerRoutes(router, deps)

	registerAdminAdsRoutes(router, deps)
	registerAdminAnnouncementRoutes(router, deps)
	registerAdminApplicationRoutes(router, deps)
	registerAdminAuthMethodRoutes(router, deps)
	registerAdminConsoleRoutes(router, deps)
	registerAdminCouponRoutes(router, deps)
	registerAdminDocumentRoutes(router, deps)
	registerAdminLogRoutes(router, deps)
	registerAdminMarketingRoutes(router, deps)
	registerAdminOrderRoutes(router, deps)
	registerAdminPaymentRoutes(router, deps)
	registerAdminWithdrawalRoutes(router, deps)
	registerAdminServerRoutes(router, deps)
	registerAdminSubscribeRoutes(router, deps)
	registerAdminSystemRoutes(router, deps)
	registerAdminTicketRoutes(router, deps)
	registerAdminToolRoutes(router, deps)
	registerAdminUserRoutes(router, deps)

	registerAuthRoutes(router, deps)
	registerCommonRoutes(router, deps)

	registerPublicAnnouncementRoutes(router, deps)
	registerPublicDocumentRoutes(router, deps)
	registerPublicOrderRoutes(router, deps)
	registerPublicOrderV2Routes(router, deps)
	registerPublicPaymentRoutes(router, deps)
	registerPublicPortalRoutes(router, deps)
	registerPublicSubscribeRoutes(router, deps)
	registerPublicTicketRoutes(router, deps)
	registerPublicUserRoutes(router, deps)
}
