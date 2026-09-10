package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// TelephonePasswordResetPolicy contains the authentication-method policy
// required by telephone password reset.
type TelephonePasswordResetPolicy interface {
	EnsureMethodEnabled(ctx context.Context, method string) error
}

// TelephoneResetPasswordStore is the persistence surface used by telephone
// password reset. It excludes unrelated application repositories.
type TelephoneResetPasswordStore interface {
	User() repository.UserRepo
	UserAuth() repository.UserAuthRepo
	Log() repository.LogRepo
}

// TelephoneResetPasswordConfig is the configuration snapshot consumed by
// telephone password reset.
type TelephoneResetPasswordConfig struct {
	JWTAccessSecret string
	JWTAccessExpire int64
}

// TelephoneResetPasswordDependencies explicitly declares the collaborators of
// telephone password reset instead of passing Application to business logic.
type TelephoneResetPasswordDependencies struct {
	Store        TelephoneResetPasswordStore
	Redis        *redis.Client
	Config       TelephoneResetPasswordConfig
	Policy       TelephonePasswordResetPolicy
	DeviceBinder DeviceBinder
}
