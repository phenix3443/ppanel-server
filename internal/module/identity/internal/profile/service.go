package profile

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

// Service is the profile subdomain entry point used by the identity facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) QueryUserInfo(ctx context.Context) (*dto.User, error) {
	return newQueryUserInfoLogic(ctx, s.deps).QueryUserInfo()
}

func (s *Service) UpdateUserPassword(ctx context.Context, req *dto.UpdateUserPasswordRequest) error {
	return newUpdateUserPasswordLogic(ctx, s.deps).UpdateUserPassword(req)
}

func (s *Service) UpdateUserNotify(ctx context.Context, req *dto.UpdateUserNotifyRequest) error {
	return newUpdateUserNotifyLogic(ctx, s.deps).UpdateUserNotify(req)
}

func (s *Service) UpdateUserRules(ctx context.Context, req *dto.UpdateUserRulesRequest) error {
	return newUpdateUserRulesLogic(ctx, s.deps).UpdateUserRules(req)
}

func (s *Service) GetLoginLog(ctx context.Context, req *dto.GetLoginLogRequest) (*dto.GetLoginLogResponse, error) {
	return newGetLoginLogLogic(ctx, s.deps).GetLoginLog(req)
}

func (s *Service) GetDeviceList(ctx context.Context) (*dto.GetDeviceListResponse, error) {
	return newGetDeviceListLogic(ctx, s.deps).GetDeviceList()
}

func (s *Service) UnbindDevice(ctx context.Context, req *dto.UnbindDeviceRequest) error {
	return newUnbindDeviceLogic(ctx, s.deps).UnbindDevice(req)
}

func (s *Service) GetOAuthMethods(ctx context.Context) (*dto.GetOAuthMethodsResponse, error) {
	return newGetOAuthMethodsLogic(ctx, s.deps).GetOAuthMethods()
}

func (s *Service) BindOAuth(ctx context.Context, req *dto.BindOAuthRequest) (*dto.BindOAuthResponse, error) {
	return newBindOAuthLogic(ctx, s.deps).BindOAuth(req)
}

func (s *Service) BindOAuthCallback(ctx context.Context, req *dto.BindOAuthCallbackRequest) error {
	return newBindOAuthCallbackLogic(ctx, s.deps).BindOAuthCallback(req)
}

func (s *Service) UnbindOAuth(ctx context.Context, req *dto.UnbindOAuthRequest) error {
	return newUnbindOAuthLogic(ctx, s.deps).UnbindOAuth(req)
}

func (s *Service) BindTelegram(ctx context.Context) (*dto.BindTelegramResponse, error) {
	return newBindTelegramLogic(ctx, s.deps).BindTelegram()
}

func (s *Service) UnbindTelegram(ctx context.Context) error {
	return newUnbindTelegramLogic(ctx, s.deps).UnbindTelegram()
}

func (s *Service) UpdateBindEmail(ctx context.Context, req *dto.UpdateBindEmailRequest) error {
	return newUpdateBindEmailLogic(ctx, s.deps).UpdateBindEmail(req)
}

func (s *Service) VerifyEmail(ctx context.Context, req *dto.VerifyEmailRequest) error {
	return newVerifyEmailLogic(ctx, s.deps).VerifyEmail(req)
}

func (s *Service) UpdateBindMobile(ctx context.Context, req *dto.UpdateBindMobileRequest) error {
	return newUpdateBindMobileLogic(ctx, s.deps).UpdateBindMobile(req)
}
