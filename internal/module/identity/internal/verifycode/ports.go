package verifycode

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// VerificationCodePolicy contains the authentication policy required before
// issuing a verification code.
type VerificationCodePolicy interface {
	EnsureRegistrationOpen(ctx context.Context, method string) error
	EnsureMethodEnabled(ctx context.Context, method string) error
}

// VerificationIdentityStore is the persistence surface used by verification
// code delivery. It
// excludes unrelated application repositories.
type VerificationIdentityStore interface {
	UserAuth() repository.UserAuthRepo
}

// VerificationTaskQueue publishes verification-code delivery tasks.
type VerificationTaskQueue interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// EmailCodeConfig is the configuration snapshot consumed by email code
// delivery.
type EmailCodeConfig struct {
	DomainSuffixList   string
	EnableDomainSuffix bool
	VerifyCodeInterval int64
	VerifyCodeLimit    int64
	VerifyCodeExpire   int64
	SiteLogo           string
	SiteName           string
}

// SendEmailCodeDependencies explicitly declares the collaborators of email
// code delivery instead of passing Application to business logic.
type SendEmailCodeDependencies struct {
	Store  VerificationIdentityStore
	Redis  *redis.Client
	Queue  VerificationTaskQueue
	Config EmailCodeConfig
	Policy VerificationCodePolicy
}
