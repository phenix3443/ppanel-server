// Service assembly for the verification-code subdomain: issuing and
// pre-checking the email/SMS codes that gate registration and account
// mutations. Only the module facade may reach it.
package verifycode

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/redis/go-redis/v9"
)

// Snapshot is the per-request view of the runtime-mutable settings the
// verification-code flows consume.
type Snapshot struct {
	DomainSuffixList   string
	EnableDomainSuffix bool
	VerifyCodeInterval int64
	VerifyCodeLimit    int64
	VerifyCodeExpire   int64
	SiteLogo           string
	SiteName           string
}

// Deps declares the subdomain's dependencies; the identity facade forwards
// them from the composition root and supplies the register policy from the
// authentication subdomain.
type Deps struct {
	Store  VerificationIdentityStore
	Redis  *redis.Client
	Queue  VerificationTaskQueue
	Policy VerificationCodePolicy
	// Config snapshots the runtime-mutable settings per request.
	Config func() Snapshot
}

// Service is the verification-code subdomain entry point used by the
// identity facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) SendEmailCode(ctx context.Context, req *dto.SendCodeRequest) (*dto.SendCodeResponse, error) {
	cfg := s.deps.Config()
	return NewSendEmailCodeLogic(ctx, SendEmailCodeDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Queue: s.deps.Queue,
		Config: EmailCodeConfig{
			DomainSuffixList:   cfg.DomainSuffixList,
			EnableDomainSuffix: cfg.EnableDomainSuffix,
			VerifyCodeInterval: cfg.VerifyCodeInterval,
			VerifyCodeLimit:    cfg.VerifyCodeLimit,
			VerifyCodeExpire:   cfg.VerifyCodeExpire,
			SiteLogo:           cfg.SiteLogo,
			SiteName:           cfg.SiteName,
		},
		Policy: s.deps.Policy,
	}).SendEmailCode(req)
}

func (s *Service) SendSmsCode(ctx context.Context, req *dto.SendSmsCodeRequest) (*dto.SendCodeResponse, error) {
	cfg := s.deps.Config()
	return NewSendSmsCodeLogic(ctx, SendSmsCodeDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Queue: s.deps.Queue,
		Config: SmsCodeConfig{
			VerifyCodeInterval: cfg.VerifyCodeInterval,
			VerifyCodeLimit:    cfg.VerifyCodeLimit,
			VerifyCodeExpire:   cfg.VerifyCodeExpire,
		},
		Policy: s.deps.Policy,
	}).SendSmsCode(req)
}

func (s *Service) CheckVerificationCode(ctx context.Context, req *dto.CheckVerificationCodeRequest) (*dto.CheckVerificationCodeRespone, error) {
	return newCheckVerificationCodeLogic(ctx, s.deps).CheckVerificationCode(req)
}
