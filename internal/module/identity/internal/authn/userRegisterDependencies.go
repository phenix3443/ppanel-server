package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// EmailRegistrationPolicy contains the registration checks required by email
// registration.
type EmailRegistrationPolicy interface {
	EnsureRegistrationOpen(ctx context.Context, method string) error
	VerifyHuman(ctx context.Context, token, ip string) error
	TakeIPPermit(ctx context.Context, ip string) error
}

// UserRegisterStore is the persistence surface used by email registration.
// It excludes unrelated application repositories.
type UserRegisterStore interface {
	User() repository.UserRepo
	UserAuth() repository.UserAuthRepo
	Log() repository.LogRepo
	InIdentityTx(ctx context.Context, fn func(repository.IdentityStore) error) error
}

// DeviceBinder attaches a device identifier to a newly registered user.
type DeviceBinder interface {
	BindDeviceToUser(identifier, ip, userAgent string, currentUserID int64) (*user.Device, error)
}

// UserRegisterConfig is the configuration snapshot consumed by email
// registration.
type UserRegisterConfig struct {
	EmailDomainSuffixList   string
	EmailEnableDomainSuffix bool
	EmailVerifyEnabled      bool
	InviteForced            bool
	OnlyFirstPurchase       bool
	TrialEnabled            bool
	TrialSubscribeID        int64
	TrialTime               int64
	TrialTimeUnit           string
	JWTAccessSecret         string
	JWTAccessExpire         int64
}

// UserRegisterDependencies explicitly declares the collaborators of email
// registration instead of passing Application to business logic.
type UserRegisterDependencies struct {
	Store        UserRegisterStore
	Redis        *redis.Client
	Config       UserRegisterConfig
	Policy       EmailRegistrationPolicy
	DeviceBinder DeviceBinder
}
