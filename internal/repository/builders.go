package repository

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// The store is assembled from per-module repository bundles (ADR-001 step-6
// preparation: each module owns its persistence implementation and exports a
// builder; this package keeps only the contracts and the assembly).
//
// A builder runs once for the root connection and once per transaction with
// the tx-scoped connection. Anything that must survive across transactions
// (cache-retry singletons and similar) belongs in the closure that produced
// the builder, not in the bundle.

// ModuleConn is the per-connection context handed to a repo builder.
type ModuleConn struct {
	DB    *gorm.DB
	Redis *redis.Client
	// Invalidations batches cache invalidation keys during a transaction;
	// nil outside transactions.
	Invalidations *cache.InvalidationQueue
}

// Conn builds the cached connection every repository implementation wraps.
func (c ModuleConn) Conn() cache.CachedConn {
	return newCachedConn(c.DB, c.Redis, c.Invalidations)
}

// SubscriptionCacheBridge is the identity bundle's window onto the
// subscription domain's cache concerns: the user-deletion cascade collects
// the user's subscription rows, and the UserCache facade delegates the
// subscription cache operations. The subscription bundle provides it.
type SubscriptionCacheBridge interface {
	QueryUserSubscribe(ctx context.Context, userId int64, status ...int64) ([]*usersub.SubscribeDetails, error)
	ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error
	UpdateUserSubscribeCache(ctx context.Context, data *usersub.Subscribe) error
}

// SubscriptionUserFilter narrows SubscriptionUserIDs. Zero-value fields do
// not constrain; a nil Statuses matches any subscription row.
type SubscriptionUserFilter struct {
	UserSubscribeID *int64
	SubscribeID     *int64
	// Token matches the subscription token or UUID.
	Token    string
	Statuses []int64
}

// SubscriptionScopeBridge is the identity bundle's window onto subscription
// membership: the admin user filter and the email-recipient scopes resolve
// "which users hold matching subscriptions" to an ID list here instead of
// querying the subscription table from identity SQL. The subscription
// bundle provides it.
type SubscriptionScopeBridge interface {
	SubscriptionUserIDs(ctx context.Context, filter SubscriptionUserFilter) ([]int64, error)
}

// OrderStatsBridge is the identity bundle's window onto billing's order
// statistics: user-statistics dashboards merge these per-bucket counts with
// registration counts in Go. The billing bundle provides it.
type OrderStatsBridge interface {
	// OrderUserCountsByBucket counts distinct ordering users per date bucket
	// ("day" or "month"); isNew selects first-purchase orders.
	OrderUserCountsByBucket(ctx context.Context, isNew bool, since time.Time, until *time.Time, bucket string) (map[string]int64, error)
}

// NodeCacheKeyBridge is the subscription bundle's window onto network's
// node-derived cache keys: plan cache invalidation includes the server
// user-list keys of the plan's nodes and node tags. The network bundle
// provides it.
type NodeCacheKeyBridge interface {
	NodeUserListCacheKeys(ctx context.Context, nodeIDs []int64, tags []string) ([]string, error)
}

// IdentityBridges collects the identity bundle's cross-domain windows.
type IdentityBridges struct {
	SubscriptionCache SubscriptionCacheBridge
	SubscriptionScope SubscriptionScopeBridge
	OrderStats        OrderStatsBridge
}

// PlatformRepos is the shared-kernel bundle.
type PlatformRepos struct {
	System SystemRepo
	Logs   LogRepo
	Tasks  TaskRepo
	Client ClientRepo
	Inbox  InboxRepo
	Outbox OutboxRepo
}

type PlatformBuilder func(conn ModuleConn) PlatformRepos

// BillingRepos is the billing domain bundle.
type BillingRepos struct {
	Orders      OrderRepo
	OrderEvents OrderEventRepo
	Payments    PaymentRepo
	Coupons     CouponRepo
	Withdrawals UserWithdrawalRepo
	Wallets     WalletRepo
	// OrderStats feeds the identity bundle's user-statistics merge.
	OrderStats OrderStatsBridge
}

type BillingBuilder func(conn ModuleConn) BillingRepos

// SubscriptionRepos is the subscription domain bundle.
type SubscriptionRepos struct {
	Plans    SubscribeRepo
	UserSubs UserSubscriptionRepo
	Traffic  SubscriptionTrafficRepo
	// CacheBridge feeds the identity bundle's cross-domain cache cascade.
	CacheBridge SubscriptionCacheBridge
	// ScopeBridge feeds the identity bundle's subscription-membership
	// filters.
	ScopeBridge SubscriptionScopeBridge
}

type SubscriptionBuilder func(conn ModuleConn, nodes NodeCacheKeyBridge) SubscriptionRepos

// IdentityRepos is the identity domain bundle. UserCache is the cross-domain
// cache facade (its subscription keys come through the injected reader).
type IdentityRepos struct {
	Users     UserRepo
	UserAuths UserAuthRepo
	Devices   UserDeviceRepo
	UserCache UserCacheRepo
	Auths     AuthRepo
}

type IdentityBuilder func(conn ModuleConn, bridges IdentityBridges) IdentityRepos

// NetworkRepos is the network domain bundle.
type NetworkRepos struct {
	Nodes   NodeRepo
	Traffic TrafficRepo
	// NodeKeys feeds the subscription bundle's plan cache invalidation.
	NodeKeys NodeCacheKeyBridge
}

type NetworkBuilder func(conn ModuleConn) NetworkRepos

// SupportRepos is the support domain bundle.
type SupportRepos struct {
	Tickets       TicketRepo
	Announcements AnnouncementRepo
	Ads           AdsRepo
	Documents     DocumentRepo
}

type SupportBuilder func(conn ModuleConn) SupportRepos

// NotificationRepos is the notification domain bundle.
type NotificationRepos struct {
	TelegramTopics TelegramTopicRepo
}

type NotificationBuilder func(conn ModuleConn) NotificationRepos

// Builders carries every module's repo builder for store assembly.
type Builders struct {
	Platform     PlatformBuilder
	Billing      BillingBuilder
	Subscription SubscriptionBuilder
	Identity     IdentityBuilder
	Network      NetworkBuilder
	Support      SupportBuilder
	Notification NotificationBuilder
}
