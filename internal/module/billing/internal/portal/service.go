// Package portal implements the guest storefront subdomain of the billing
// module: guest pre-orders, gateway/balance checkout, order status polling
// with session exchange, and the public plan/payment listings. Only the
// module facade may reach it.
package portal

import (
	"context"
	"fmt"
	"time"
	"uuid"

	token2 "github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

// PlanReader is the subdomain's port onto the subscription domain's plan
// catalogue; the legacy subscribe repository satisfies it structurally.
type PlanReader interface {
	FindOne(ctx context.Context, id int64) (*subscribe.Subscribe, error)
	FilterList(ctx context.Context, params *subscribe.FilterParams) (int64, []*subscribe.Subscribe, error)
}

// GuestAccountReader is the subdomain's port onto the identity domain: guest
// purchase must refuse identifiers that already have an account.
type GuestAccountReader interface {
	FindUserAuthMethodByOpenID(ctx context.Context, method, openID string) (*user.AuthMethods, error)
}

// SessionStore issues the Redis-backed session created after a guest
// purchase completes; the redis client satisfies it structurally.
type SessionStore interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// OrderQueue mirrors the facade's order queue port (deferred close only; the
// checkout logic keeps its own activation port).
type OrderQueue interface {
	EnqueueDeferredClose(ctx context.Context, orderNo string) error
}

// Config is the static configuration snapshot for the portal flows. ClientIP
// is deliberately absent: it is resolved per request from the context.
type Config struct {
	Host string
	// SiteName/CurrencyUnit/CurrencyAccessKey are runtime-mutable (the
	// admin edits them and ReinitSubsystem reloads); read per request.
	SiteName          func() string
	CurrencyUnit      func() string
	CurrencyAccessKey func() string
	JwtSecret         string
	JwtExpire         int64
}

type Deps struct {
	Orders    repository.OrderRepo
	Coupons   repository.CouponRepo
	Payments  repository.PaymentRepo
	UserAuths GuestAccountReader
	Plans     PlanReader
	// Store serves billing persistence and read-only identity lookups.
	// Inventory changes belong to the separately injected capability.
	Store              Store
	Inventory          Inventory
	Sessions           SessionStore
	Queue              OrderQueue
	GuestCheckoutCache GuestCheckoutCache
	ActivationQueue    ActivationQueue
	ExchangeRate       ExchangeRateCache
	Config             Config
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

// Checkout drives the gateway or balance payment for a pending order.
func (s *Service) Checkout(ctx context.Context, req *dto.CheckoutOrderRequest) (*dto.CheckoutOrderResponse, error) {
	l := NewPurchaseCheckoutLogic(ctx, CheckoutDependencies{
		Store:              NewCheckoutStore(s.deps.Store),
		GuestCheckoutCache: s.deps.GuestCheckoutCache,
		ActivationQueue:    s.deps.ActivationQueue,
		Config: CheckoutConfig{
			Host:              s.deps.Config.Host,
			SiteName:          s.deps.Config.SiteName(),
			CurrencyUnit:      s.deps.Config.CurrencyUnit(),
			CurrencyAccessKey: s.deps.Config.CurrencyAccessKey(),
		},
		ExchangeRateCache: s.deps.ExchangeRate,
	})
	return l.PurchaseCheckout(req)
}

// IssueSession creates the normal authenticated session issued after a guest
// purchase completes.  Both V1's status endpoint and V2's explicit
// capability-exchange endpoint use this helper so their token and Redis
// session semantics cannot drift.
func (s *Service) IssueSession(ctx context.Context, userID int64) (string, error) {
	sessionId := uuid.NewV7().String()
	token, err := token2.NewJwtToken(
		s.deps.Config.JwtSecret,
		timeutil.Now().Unix(),
		s.deps.Config.JwtExpire,
		token2.WithOption("UserId", userID),
		token2.WithOption("SessionId", sessionId),
	)
	if err != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Token generation error")
	}

	cacheKey := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionId)
	if err := s.deps.Sessions.Set(ctx, cacheKey, userID, time.Duration(s.deps.Config.JwtExpire)*time.Second).Err(); err != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Session storage error")
	}

	return token, nil
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.BillingTransactor
	Log() repository.LogRepo
	Order() repository.OrderRepo
	Payment() repository.PaymentRepo
	User() repository.UserRepo
	UserCache() repository.UserCacheRepo
	Wallet() repository.WalletRepo
}

type Inventory interface {
	Reserve(context.Context, string, int64) error
}
