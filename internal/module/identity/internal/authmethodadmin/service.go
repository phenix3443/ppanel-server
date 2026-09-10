// Service assembly for the admin-side authentication-method subdomain of the
// identity module: method configuration, sender platforms and test sends.
// Only the module facade may reach it.
package authmethodadmin

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// Snapshot is the per-request view of the runtime-mutable sender platform
// settings.
type Snapshot struct {
	EmailPlatform        string
	EmailPlatformConfig  string
	MobilePlatform       string
	MobilePlatformConfig string
	SiteName             string
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Auths repository.AuthRepo
	// Config snapshots the runtime-mutable sender settings per request.
	Config func() Snapshot
	// Reinitialize re-runs a sender subsystem's initialization after its
	// configuration changed ("email", "mobile" or "device").
	Reinitialize func(subsystem string)
}

// Service is the auth-method administration entry point used by the identity
// facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) GetAuthMethodList(ctx context.Context) (*dto.GetAuthMethodListResponse, error) {
	return newGetAuthMethodListLogic(ctx, s.deps).GetAuthMethodList()
}

func (s *Service) GetAuthMethodConfig(ctx context.Context, req *dto.GetAuthMethodConfigRequest) (*dto.AuthMethodConfig, error) {
	return newGetAuthMethodConfigLogic(ctx, s.deps).GetAuthMethodConfig(req)
}

func (s *Service) UpdateAuthMethodConfig(ctx context.Context, req *dto.UpdateAuthMethodConfigRequest) (*dto.AuthMethodConfig, error) {
	return newUpdateAuthMethodConfigLogic(ctx, s.deps).UpdateAuthMethodConfig(req)
}

func (s *Service) GetEmailPlatform(ctx context.Context) (*dto.AuthPlatformResponse, error) {
	return newGetEmailPlatformLogic(ctx, s.deps).GetEmailPlatform()
}

func (s *Service) GetSmsPlatform(ctx context.Context) (*dto.AuthPlatformResponse, error) {
	return newGetSmsPlatformLogic(ctx, s.deps).GetSmsPlatform()
}

func (s *Service) TestEmailSend(ctx context.Context, req *dto.TestEmailSendRequest) error {
	return newTestEmailSendLogic(ctx, s.deps).TestEmailSend(req)
}

func (s *Service) TestSmsSend(ctx context.Context, req *dto.TestSmsSendRequest) error {
	return newTestSmsSendLogic(ctx, s.deps).TestSmsSend(req)
}
