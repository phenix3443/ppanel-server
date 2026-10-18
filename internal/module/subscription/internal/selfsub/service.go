// Package selfsub implements the user self-service subscription management
// of the subscription module: viewing, token reset, notes, and the two-phase
// cancellation whose refund stage the billing module settles. Only the
// module facade may reach it.
package selfsub

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	nodeEntity "github.com/perfect-panel/server/internal/module/network/entity/node"
	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	UserSubs UserSubscriptions
	Plans    repository.SubscribeRepo
	// Orders/Refunds/Cache/Logs/Inbox 是 billing、platform 域的窄接口端口
	// （上游重构：退款结算、缓存失效、系统日志、取消标记）。
	Orders  OrderReader
	Refunds RefundSettler
	Cache   CacheInvalidator
	Logs    LogReader
	Inbox   MarkerReader
	// Traffic/Nodes 支撑用户侧订阅流量概览与明细（本 fork 功能 #18）。它们是
	// network 域的读，按 ADR-001 rule 4 经 network facade 提供（不直接依赖
	// network 的 repository），这里只声明本功能需要的窄接口。
	Traffic SubscribeTrafficReader
	Nodes   ServerLister
	Store   Store
	// SingleModel forbids holding more than one blocking subscription;
	// runtime-mutable, read per request.
	SingleModel func() bool
}

// UserSubscriptions is the part of the subscription repository the
// self-service reads and the note edit use outside a transaction; the
// module's repository provides it.
type UserSubscriptions interface {
	FindOneSubscribe(ctx context.Context, id int64) (*usersub.Subscribe, error)
	// FindOneUserSubscribe returns the subscription with its plan.
	FindOneUserSubscribe(ctx context.Context, id int64) (*usersub.SubscribeDetails, error)
	// QueryUserSubscribe lists the user's subscriptions with their plans;
	// given statuses, only those in one of them.
	QueryUserSubscribe(ctx context.Context, userID int64, statuses ...int64) ([]*usersub.SubscribeDetails, error)
	// UpdateSubscribeColumns writes only the named columns of data.Id.
	UpdateSubscribeColumns(ctx context.Context, data *usersub.Subscribe, columns ...string) error
}

// OrderReader reads an order with its renewals: what a refund pays back.
type OrderReader interface {
	FindOneDetails(ctx context.Context, id int64) (*order.Details, error)
}

// RefundSettler is the billing port of the cancellation's refund stage; the
// billing facade provides it.
type RefundSettler interface {
	// UnsubscribeRefundSettled reports whether the subscription's refund was
	// settled.
	UnsubscribeRefundSettled(ctx context.Context, subscriptionID int64) (bool, error)
	// SettleUnsubscribeRefund settles it in a billing-domain transaction:
	// amount back to the user's wallet and the referral commission taken
	// back when the subscription has an order, and the refund marker; a
	// second settlement fails.
	SettleUnsubscribeRefund(ctx context.Context, userID, subscriptionID, orderID, amount int64) error
}

// CacheInvalidator drops cached subscription rows.
type CacheInvalidator interface {
	ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error
}

// LogReader pages the platform's system log.
type LogReader interface {
	FilterSystemLog(ctx context.Context, filter *log.FilterParams) ([]*log.SystemLog, int64, error)
}

// MarkerReader reads the inbox markers of the cancellation stage.
type MarkerReader interface {
	Find(ctx context.Context, consumer, eventKey string) (*inbox.Record, error)
}

// SubscribeTrafficReader reads the network-owned traffic_log for one
// subscription's usage overview (#18); the network facade satisfies it.
type SubscribeTrafficReader interface {
	QuerySubscribeTrafficSummary(ctx context.Context, scope trafficEntity.SubscribeTrafficScope) (*trafficEntity.TotalTraffic, error)
	QuerySubscribeHourlyTraffic(ctx context.Context, scope trafficEntity.SubscribeTrafficScope) ([]trafficEntity.HourlyTraffic, error)
	QuerySubscribeServerRanking(ctx context.Context, scope trafficEntity.SubscribeTrafficScope) ([]trafficEntity.ServerTrafficRanking, error)
	QueryTrafficLogDetails(ctx context.Context, filter *trafficEntity.TrafficLogDetailsFilter) ([]*trafficEntity.TrafficLog, int64, error)
}

// ServerLister labels the per-server traffic ranking with server names; the
// network facade satisfies it.
type ServerLister interface {
	QueryServerList(ctx context.Context, ids []int64) ([]*nodeEntity.Server, error)
}

// Service is the self-service entry point used by the subscription facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// 以下两个是本 fork 的订阅流量概览/明细(#18)。其余 Service 方法上游已拆到
// 各自的单文件(queryusersubscribe.go 等)，这里不再重复声明。
func (s *Service) GetSubscribeTrafficOverview(ctx context.Context, req *dto.GetSubscribeTrafficOverviewRequest) (*dto.GetSubscribeTrafficOverviewResponse, error) {
	return newGetSubscribeTrafficLogic(ctx, s.deps).Overview(req)
}

func (s *Service) GetSubscribeTrafficDetails(ctx context.Context, req *dto.GetSubscribeTrafficDetailsRequest) (*dto.GetSubscribeTrafficDetailsResponse, error) {
	return newGetSubscribeTrafficLogic(ctx, s.deps).Details(req)
}

// Store is the persistence capability required by this package: the
// cancellation's subscription-domain transaction.
type Store interface {
	repository.SubscriptionTransactor
}
