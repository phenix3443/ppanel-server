// Package profile implements the self-service identity subdomain of the
// identity module: account info, credentials, third-party bindings, devices
// and notification preferences. Only the module facade may reach it.
package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// OAuthMethodPolicy checks whether a requested authentication method is
// enabled; the sibling authentication subdomain's register policy satisfies
// it.
type OAuthMethodPolicy interface {
	EnsureMethodEnabled(ctx context.Context, method string) error
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Users     repository.UserRepo
	UserAuth  repository.UserAuthRepo
	Auth      repository.AuthRepo
	Devices   repository.UserDeviceRepo
	UserCache repository.UserCacheRepo
	Logs      repository.LogRepo
	// Wallet is the billing-domain read port for the account view.
	Wallet repository.WalletRepo
	Redis  *redis.Client
	// Store carries the identity-scoped transaction for device unbinding.
	Store  Store
	Policy OAuthMethodPolicy

	// EmailDomains snapshots the runtime-mutable email domain-suffix policy.
	EmailDomains func() (domainList string, restrict bool)
	// SiteHost snapshots the runtime-mutable site host that anchors the
	// OAuth redirect allowlist.
	SiteHost func() string
	// TelegramBotName snapshots the runtime-mutable Telegram bot name.
	TelegramBotName func() string
	// NotifyUnbind sends the best-effort Telegram unbind notice through the
	// runtime-configured bot.
	NotifyUnbind func(userID, chatID int64) error
	KickDevice   func(userID int64, identifier string)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.IdentityTransactor
}
