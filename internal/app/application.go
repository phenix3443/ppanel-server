package app

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/app/state"
	"github.com/perfect-panel/server/internal/auth/ratelimit"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/eventbus"
	"github.com/perfect-panel/server/internal/infra/geoip"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/devicesocket"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/redis/go-redis/v9"
)

type Application struct {
	Redis        *redis.Client
	Runtime      *state.State
	Queue        *taskqueue.Client
	Inspector    *asynq.Inspector
	ExchangeRate *billing.CurrencyRateCache
	GeoIP        *geoip.IPLocation
	Store        repository.Store

	// Domain modules (see docs/design/adr-001-modular-monolith.md). Application is
	// their composition root; handlers call the module facades.
	Support      support.Service
	Billing      billing.Service
	Platform     platform.Service
	Subscription subscription.Service
	Identity     identity.Service
	Network      network.Service
	Notification notification.Service
	EventBus     *eventbus.Bus

	//NodeCache   *cache.NodeCacheClient
	AuthLimiter   *ratelimit.PeriodLimit
	DeviceManager *devicesocket.DeviceManager
}

func NewApplication(c config.Config) *Application {
	// gorm initialize
	db, err := orm.ConnectMysql(orm.Mysql{
		Config: c.DatabaseConfig(),
	})

	if err != nil {
		panic(err.Error())
	}

	// IP location initialize
	geoIP, err := geoip.NewIPLocation("./cache/GeoLite2-City.mmdb")
	if err != nil {
		panic(err.Error())
	}

	rds := redis.NewClient(&redis.Options{
		Addr:     c.Redis.Host,
		Password: c.Redis.Pass,
		DB:       c.Redis.DB,
	})
	err = rds.Ping(context.Background()).Err()
	if err != nil {
		panic(err.Error())
	}
	authLimiter := ratelimit.NewPeriodLimit(86400, 15, rds, config.SendCountLimitKeyPrefix, ratelimit.Align())
	store := NewStore(db, rds)
	queue := NewAsynqClient(c)
	rate := billing.NewCurrencyRateCache(0)
	srv := &Application{
		Redis:        rds,
		Runtime:      state.New(c),
		Queue:        queue,
		Inspector:    NewAsynqInspector(c),
		ExchangeRate: rate,
		GeoIP:        geoIP,
		Store:        store,
		//NodeCache:   cache.NewNodeCacheClient(rds),
		AuthLimiter: authLimiter,
	}
	// Support takes srv for the ticket→Telegram mirror; the adapter reads
	// srv.Notification lazily, so constructing it before Notification is safe.
	srv.Support = newSupportModule(store, queue, srv)
	srv.Billing = newBillingModule(c, store, queue, rds, rate, srv)
	srv.Platform = newPlatformModule(store, srv)
	srv.DeviceManager = NewDeviceManager(srv)
	srv.Subscription = newSubscriptionModule(store, srv)
	srv.Identity = newIdentityModule(store, srv)
	srv.Network = newNetworkModule(store, srv)
	srv.Notification = newNotificationModule(store, srv)
	srv.EventBus = newEventBus(store, srv)
	return srv

}
