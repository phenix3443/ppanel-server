// Service assembly for the authentication subdomain. The per-flow logic
// keeps its explicit Dependencies structs from the earlier DI refactor; this
// file builds them per request from the module's injected collaborators and
// a runtime configuration snapshot.
package auth

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/oauth"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Snapshot is the per-request view of every runtime-mutable setting the
// authentication flows consume.
type Snapshot struct {
	JWTAccessSecret string
	JWTAccessExpire int64

	EmailEnabled            bool
	EmailVerifyEnabled      bool
	EmailDomainSuffixList   string
	EmailEnableDomainSuffix bool
	MobileEnabled           bool
	DeviceEnabled           bool
	DeviceOnlyReal          bool

	InviteForced      bool
	OnlyFirstPurchase bool
	TrialEnabled      bool
	TrialSubscribeID  int64
	TrialTime         int64
	TrialTimeUnit     string

	StopRegister            bool
	RegisterVerify          bool
	TurnstileSecret         string
	EnableIpRegisterLimit   bool
	IpRegisterLimit         int64
	IpRegisterLimitDuration int64

	// SiteHost anchors the OAuth redirect allowlist and is the fallback
	// redirect target for the Apple form-post callback.
	SiteHost string
}

// Deps declares the subdomain's dependencies; the identity facade forwards
// them from the composition root.
type Deps struct {
	Store Store
	Redis *redis.Client
	// Config snapshots the runtime-mutable settings per request.
	Config func() Snapshot
}

// Service is the authentication subdomain entry point used by the identity
// facade.
type Service struct {
	deps   Deps
	policy registerpolicy.ServicePolicy
}

func NewService(deps Deps) *Service {
	return &Service{
		deps: deps,
		policy: registerpolicy.New(registerpolicy.Deps{
			Auths: deps.Store.Auth(),
			Redis: deps.Redis,
			Config: func() registerpolicy.Snapshot {
				cfg := deps.Config()
				return registerpolicy.Snapshot{
					EmailEnabled:            cfg.EmailEnabled,
					MobileEnabled:           cfg.MobileEnabled,
					DeviceEnabled:           cfg.DeviceEnabled,
					StopRegister:            cfg.StopRegister,
					RegisterVerify:          cfg.RegisterVerify,
					TurnstileSecret:         cfg.TurnstileSecret,
					EnableIpRegisterLimit:   cfg.EnableIpRegisterLimit,
					IpRegisterLimit:         cfg.IpRegisterLimit,
					IpRegisterLimitDuration: cfg.IpRegisterLimitDuration,
				}
			},
		}),
	}
}

// Policy exposes the register policy to sibling subdomains (the profile
// flows gate method rebinding on the same switches).
func (s *Service) Policy() registerpolicy.ServicePolicy { return s.policy }

func (s *Service) binder(ctx context.Context) *BindDeviceLogic {
	return NewBindDeviceLogic(ctx, BindDeviceDependencies{Store: s.deps.Store})
}

func (s *Service) CheckUser(ctx context.Context, req *dto.CheckUserRequest) (*dto.CheckUserResponse, error) {
	return NewCheckUserLogic(ctx, CheckUserDependencies{Store: s.deps.Store}).CheckUser(req)
}

func (s *Service) CheckUserTelephone(ctx context.Context, req *dto.TelephoneCheckUserRequest) (*dto.TelephoneCheckUserResponse, error) {
	return NewCheckUserTelephoneLogic(ctx, CheckUserDependencies{Store: s.deps.Store}).CheckUserTelephone(req)
}

func (s *Service) UserLogin(ctx context.Context, req *dto.UserLoginRequest) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewUserLoginLogic(ctx, UserLoginDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: UserLoginConfig{
			JWTAccessSecret: cfg.JWTAccessSecret,
			JWTAccessExpire: cfg.JWTAccessExpire,
		},
		Policy:       s.policy,
		DeviceBinder: s.binder(ctx),
	}).UserLogin(req)
}

func (s *Service) UserRegister(ctx context.Context, req *dto.UserRegisterRequest) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewUserRegisterLogic(ctx, UserRegisterDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: UserRegisterConfig{
			EmailDomainSuffixList:   cfg.EmailDomainSuffixList,
			EmailEnableDomainSuffix: cfg.EmailEnableDomainSuffix,
			EmailVerifyEnabled:      cfg.EmailVerifyEnabled,
			InviteForced:            cfg.InviteForced,
			OnlyFirstPurchase:       cfg.OnlyFirstPurchase,
			TrialEnabled:            cfg.TrialEnabled,
			TrialSubscribeID:        cfg.TrialSubscribeID,
			TrialTime:               cfg.TrialTime,
			TrialTimeUnit:           cfg.TrialTimeUnit,
			JWTAccessSecret:         cfg.JWTAccessSecret,
			JWTAccessExpire:         cfg.JWTAccessExpire,
		},
		Policy:       s.policy,
		DeviceBinder: s.binder(ctx),
	}).UserRegister(req)
}

func (s *Service) TelephoneLogin(ctx context.Context, req *dto.TelephoneLoginRequest, ip, userAgent string) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewTelephoneLoginLogic(ctx, TelephoneLoginDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: TelephoneLoginConfig{
			JWTAccessSecret: cfg.JWTAccessSecret,
			JWTAccessExpire: cfg.JWTAccessExpire,
		},
		Policy:       s.policy,
		DeviceBinder: s.binder(ctx),
	}).TelephoneLogin(req, ip, userAgent)
}

func (s *Service) TelephoneUserRegister(ctx context.Context, req *dto.TelephoneRegisterRequest) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewTelephoneUserRegisterLogic(ctx, TelephoneUserRegisterDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: TelephoneUserRegisterConfig{
			InviteForced:      cfg.InviteForced,
			OnlyFirstPurchase: cfg.OnlyFirstPurchase,
			TrialEnabled:      cfg.TrialEnabled,
			TrialSubscribeID:  cfg.TrialSubscribeID,
			TrialTime:         cfg.TrialTime,
			TrialTimeUnit:     cfg.TrialTimeUnit,
			JWTAccessSecret:   cfg.JWTAccessSecret,
			JWTAccessExpire:   cfg.JWTAccessExpire,
		},
		Policy:       s.policy,
		DeviceBinder: s.binder(ctx),
	}).TelephoneUserRegister(req)
}

func (s *Service) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewResetPasswordLogic(ctx, ResetPasswordDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: ResetPasswordConfig{
			JWTAccessSecret: cfg.JWTAccessSecret,
			JWTAccessExpire: cfg.JWTAccessExpire,
		},
		Policy:       s.policy,
		DeviceBinder: s.binder(ctx),
	}).ResetPassword(req)
}

func (s *Service) TelephoneResetPassword(ctx context.Context, req *dto.TelephoneResetPasswordRequest) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewTelephoneResetPasswordLogic(ctx, TelephoneResetPasswordDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: TelephoneResetPasswordConfig{
			JWTAccessSecret: cfg.JWTAccessSecret,
			JWTAccessExpire: cfg.JWTAccessExpire,
		},
		Policy:       s.policy,
		DeviceBinder: s.binder(ctx),
	}).TelephoneResetPassword(req)
}

func (s *Service) DeviceLogin(ctx context.Context, req *dto.DeviceLoginRequest) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return NewDeviceLoginLogic(ctx, DeviceLoginDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: DeviceLoginConfig{
			Enabled:           cfg.DeviceEnabled,
			OnlyRealDevice:    cfg.DeviceOnlyReal,
			InviteForced:      cfg.InviteForced,
			OnlyFirstPurchase: cfg.OnlyFirstPurchase,
			TrialEnabled:      cfg.TrialEnabled,
			TrialSubscribeID:  cfg.TrialSubscribeID,
			TrialTime:         cfg.TrialTime,
			TrialTimeUnit:     cfg.TrialTimeUnit,
			JWTAccessSecret:   cfg.JWTAccessSecret,
			JWTAccessExpire:   cfg.JWTAccessExpire,
		},
		Policy: s.policy,
	}).DeviceLogin(req)
}

func (s *Service) OAuthLogin(ctx context.Context, req *dto.OAthLoginRequest) (*dto.OAuthLoginResponse, error) {
	return oauth.NewOAuthLoginLogic(ctx, oauth.OAuthLoginURLDependencies{
		Store:    s.deps.Store,
		Redis:    s.deps.Redis,
		Policy:   s.policy,
		SiteHost: s.deps.Config().SiteHost,
	}).OAuthLogin(req)
}

func (s *Service) OAuthLoginGetToken(ctx context.Context, req *dto.OAuthLoginGetTokenRequest, ip, userAgent string) (*dto.LoginResponse, error) {
	cfg := s.deps.Config()
	return oauth.NewOAuthLoginGetTokenLogic(ctx, oauth.OAuthLoginDependencies{
		Store: s.deps.Store,
		Redis: s.deps.Redis,
		Config: oauth.OAuthLoginConfig{
			InviteForced:            cfg.InviteForced,
			OnlyFirstPurchase:       cfg.OnlyFirstPurchase,
			EmailDomainSuffixList:   cfg.EmailDomainSuffixList,
			EmailEnableDomainSuffix: cfg.EmailEnableDomainSuffix,
			TrialEnabled:            cfg.TrialEnabled,
			TrialSubscribeID:        cfg.TrialSubscribeID,
			TrialTime:               cfg.TrialTime,
			TrialTimeUnit:           cfg.TrialTimeUnit,
			JWTAccessSecret:         cfg.JWTAccessSecret,
			JWTAccessExpire:         cfg.JWTAccessExpire,
		},
		Policy: s.policy,
	}).OAuthLoginGetToken(req, ip, userAgent)
}

func (s *Service) AppleLoginCallback(ctx context.Context, req *dto.AppleLoginCallbackRequest) (*oauth.AppleLoginRedirect, error) {
	return oauth.NewAppleLoginCallbackLogic(ctx, oauth.AppleLoginCallbackDependencies{
		Redis:            s.deps.Redis,
		FallbackRedirect: s.deps.Config().SiteHost,
	}).AppleLoginCallback(req)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	Auth() repository.AuthRepo
	repository.IdentityTransactor
	Log() repository.LogRepo
	User() repository.UserRepo
	UserAuth() repository.UserAuthRepo
	UserDevice() repository.UserDeviceRepo
}
