// Package registerpolicy enforces the administrator-configured account
// policies shared by every authentication and registration path.
package registerpolicy

import (
	"context"
	"net"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/auth/challenge"
	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/ratelimit"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

const (
	MethodEmail  = identifier.Email
	MethodMobile = identifier.Mobile
	MethodDevice = identifier.Device
)

// Snapshot is the per-request view of the runtime-mutable policy settings.
type Snapshot struct {
	EmailEnabled  bool
	MobileEnabled bool
	DeviceEnabled bool

	StopRegister            bool
	RegisterVerify          bool
	TurnstileSecret         string
	EnableIpRegisterLimit   bool
	IpRegisterLimit         int64
	IpRegisterLimitDuration int64
}

// Deps declares the policy's collaborators; the identity facade provides
// them.
type Deps struct {
	Auths repository.AuthRepo
	Redis *redis.Client
	// Config snapshots the runtime-mutable policy settings per call.
	Config func() Snapshot
}

// ServicePolicy is the use-case policy port implementation.
type ServicePolicy struct {
	deps Deps
}

func New(deps Deps) ServicePolicy {
	return ServicePolicy{deps: deps}
}

// EnsureMethodEnabled rejects direct calls to authentication methods disabled
// by the administrator. OAuth methods are loaded from the auth_method table.
func (p ServicePolicy) EnsureMethodEnabled(ctx context.Context, method string) error {
	cfg := p.deps.Config()
	switch method {
	case MethodEmail:
		if cfg.EmailEnabled {
			return nil
		}
	case MethodMobile:
		if cfg.MobileEnabled {
			return nil
		}
	case MethodDevice:
		if cfg.DeviceEnabled {
			return nil
		}
	default:
		configured, err := p.deps.Auths.FindOneByMethod(ctx, method)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.GetAuthenticatorError), "load auth method %q: %v", method, err)
		}
		if configured.Enabled != nil && *configured.Enabled {
			return nil
		}
	}
	return errors.Wrapf(xerr.NewErrCode(xerr.GetAuthenticatorError), "auth method %q is disabled", method)
}

// EnsureRegistrationOpen applies policies shared by every new-account path.
func (p ServicePolicy) EnsureRegistrationOpen(ctx context.Context, method string) error {
	if p.deps.Config().StopRegister {
		return errors.Wrap(xerr.NewErrCode(xerr.StopRegister), "registration is disabled")
	}
	return p.EnsureMethodEnabled(ctx, method)
}

// VerifyHuman enforces the configured registration Turnstile challenge.
func (p ServicePolicy) VerifyHuman(ctx context.Context, token, ip string) error {
	cfg := p.deps.Config()
	if !cfg.RegisterVerify {
		return nil
	}
	if strings.TrimSpace(token) == "" || strings.TrimSpace(cfg.TurnstileSecret) == "" {
		return errors.Wrap(xerr.NewErrCode(xerr.TooManyRequests), "registration verification failed")
	}
	verifier := challenge.New(challenge.Config{
		Secret:  cfg.TurnstileSecret,
		Timeout: 3 * time.Second,
	})
	ok, err := verifier.Verify(ctx, token, ip)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "registration verification failed: %v", err)
	}
	if !ok {
		return errors.Wrap(xerr.NewErrCode(xerr.TooManyRequests), "registration verification failed")
	}
	return nil
}

// TakeIPPermit atomically reserves one registration from the configured IP
// quota. The duration is configured in minutes.
func (p ServicePolicy) TakeIPPermit(ctx context.Context, ip string) error {
	cfg := p.deps.Config()
	if !cfg.EnableIpRegisterLimit {
		return nil
	}
	if p.deps.Redis == nil || cfg.IpRegisterLimit <= 0 || cfg.IpRegisterLimitDuration <= 0 {
		return errors.Wrap(xerr.NewErrCode(xerr.ERROR), "invalid IP registration limit configuration")
	}
	parsedIP := net.ParseIP(strings.TrimSpace(ip))
	if parsedIP == nil {
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "invalid client IP")
	}

	maxInt := int64(^uint(0) >> 1)
	if cfg.IpRegisterLimit > maxInt || cfg.IpRegisterLimitDuration > maxInt/60 {
		return errors.Wrap(xerr.NewErrCode(xerr.ERROR), "IP registration limit configuration is too large")
	}
	limiter := ratelimit.NewPeriodLimit(
		int(cfg.IpRegisterLimitDuration*60),
		int(cfg.IpRegisterLimit),
		p.deps.Redis,
		config.RegisterIPLimitKeyPrefix,
	)
	permit, err := limiter.TakeCtx(ctx, parsedIP.String())
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "check IP registration limit: %v", err)
	}
	if !limiter.ParsePermitState(permit) {
		return errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "registration limit exceeded for IP %s", parsedIP.String())
	}
	return nil
}
