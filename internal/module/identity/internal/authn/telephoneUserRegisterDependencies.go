package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// TelephoneRegistrationPolicy contains the registration checks required by
// telephone registration.
type TelephoneRegistrationPolicy interface {
	EnsureRegistrationOpen(ctx context.Context, method string) error
	VerifyHuman(ctx context.Context, token, ip string) error
	TakeIPPermit(ctx context.Context, ip string) error
}

// TelephoneUserRegisterStore is the persistence surface used by telephone
// registration. It excludes unrelated application repositories.
type TelephoneUserRegisterStore interface {
	User() repository.UserRepo
	UserAuth() repository.UserAuthRepo
	Log() repository.LogRepo
	InIdentityTx(ctx context.Context, fn func(repository.IdentityStore) error) error
}

// TelephoneUserRegisterConfig is the configuration snapshot consumed by
// telephone registration.
type TelephoneUserRegisterConfig struct {
	InviteForced      bool
	OnlyFirstPurchase bool
	TrialEnabled      bool
	TrialSubscribeID  int64
	TrialTime         int64
	TrialTimeUnit     string
	JWTAccessSecret   string
	JWTAccessExpire   int64
}

// TelephoneUserRegisterDependencies explicitly declares the collaborators of
// telephone registration instead of passing Application to business logic.
type TelephoneUserRegisterDependencies struct {
	Store        TelephoneUserRegisterStore
	Redis        *redis.Client
	Config       TelephoneUserRegisterConfig
	Policy       TelephoneRegistrationPolicy
	DeviceBinder DeviceBinder
}
