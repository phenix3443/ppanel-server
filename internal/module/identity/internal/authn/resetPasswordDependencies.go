package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// EmailPasswordResetPolicy contains the authentication-method policy required
// by email password reset.
type EmailPasswordResetPolicy interface {
	EnsureMethodEnabled(ctx context.Context, method string) error
}

// ResetPasswordStore is the persistence surface used by email password reset.
// It excludes unrelated application repositories.
type ResetPasswordStore interface {
	User() repository.UserRepo
	UserAuth() repository.UserAuthRepo
	Log() repository.LogRepo
}

// ResetPasswordConfig is the configuration snapshot consumed by email password
// reset.
type ResetPasswordConfig struct {
	JWTAccessSecret string
	JWTAccessExpire int64
}

// ResetPasswordDependencies explicitly declares the collaborators of email
// password reset instead of passing Application to business logic.
type ResetPasswordDependencies struct {
	Store        ResetPasswordStore
	Redis        *redis.Client
	Config       ResetPasswordConfig
	Policy       EmailPasswordResetPolicy
	DeviceBinder DeviceBinder
}
