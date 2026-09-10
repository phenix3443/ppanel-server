package oauth

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// OAuthLoginURLPolicy contains the authentication-method policy required when
// creating an OAuth authorization URL.
type OAuthLoginURLPolicy interface {
	EnsureMethodEnabled(ctx context.Context, method string) error
}

// OAuthLoginURLStore is the persistence surface used to load OAuth provider
// configuration. It excludes unrelated application repositories.
type OAuthLoginURLStore interface {
	Auth() repository.AuthRepo
}

// OAuthLoginURLDependencies explicitly declares the collaborators of OAuth
// authorization URL creation instead of passing Application to business
// logic.
type OAuthLoginURLDependencies struct {
	Store  OAuthLoginURLStore
	Redis  *redis.Client
	Policy OAuthLoginURLPolicy
	// SiteHost anchors the redirect allowlist for flows whose stored
	// redirect later becomes a browser redirect; empty disables the pin.
	SiteHost string
}
