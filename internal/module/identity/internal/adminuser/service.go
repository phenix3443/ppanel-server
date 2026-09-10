// Package adminuser implements the admin-side account management subdomain
// of the identity module: user CRUD, auth methods, devices and login logs.
// Only the module facade may reach it.
package adminuser

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

type Deps struct {
	Users     repository.UserRepo
	UserAuths repository.UserAuthRepo
	Devices   repository.UserDeviceRepo
	Cache     repository.UserCacheRepo
	// UserSubs/Plans/Traffic/Logs are read ports onto the subscription,
	// network and platform domains for the admin detail views.
	UserSubs repository.UserSubscriptionRepo
	Plans    repository.SubscribeRepo
	Traffic  repository.TrafficRepo
	Logs     repository.LogRepo
	// Wallet is the read port onto the billing domain: the admin views show
	// wallet values from the authoritative table, not the legacy user
	// columns (ADR-001 step 5).
	Wallet repository.WalletRepo
	Store  Store
	Redis  *redis.Client
	// KickDevice force-disconnects a bound device.
	KickDevice func(userID int64, identifier string)
}

func (d Deps) kickDevice(userID int64, identifier string) {
	if d.KickDevice != nil {
		d.KickDevice(userID, identifier)
	}
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) CreateUser(ctx context.Context, req *dto.CreateUserRequest) error {
	return newCreateUserLogic(ctx, s.deps).CreateUser(req)
}

func (s *Service) DeleteUser(ctx context.Context, req *dto.GetDetailRequest) error {
	return newDeleteUserLogic(ctx, s.deps).DeleteUser(req)
}

func (s *Service) BatchDeleteUser(ctx context.Context, req *dto.BatchDeleteUserRequest) error {
	return newBatchDeleteUserLogic(ctx, s.deps).BatchDeleteUser(req)
}

func (s *Service) GetUserDetail(ctx context.Context, req *dto.GetDetailRequest) (*dto.User, error) {
	return newGetUserDetailLogic(ctx, s.deps).GetUserDetail(req)
}

func (s *Service) GetUserList(ctx context.Context, req *dto.GetUserListRequest) (*dto.GetUserListResponse, error) {
	return newGetUserListLogic(ctx, s.deps).GetUserList(req)
}

func (s *Service) CurrentUser(ctx context.Context) (*dto.User, error) {
	return newCurrentUserLogic(ctx, s.deps).CurrentUser()
}

func (s *Service) CreateUserAuthMethod(ctx context.Context, req *dto.CreateUserAuthMethodRequest) error {
	return newCreateUserAuthMethodLogic(ctx, s.deps).CreateUserAuthMethod(req)
}

func (s *Service) DeleteUserAuthMethod(ctx context.Context, req *dto.DeleteUserAuthMethodRequest) error {
	return newDeleteUserAuthMethodLogic(ctx, s.deps).DeleteUserAuthMethod(req)
}

func (s *Service) GetUserAuthMethod(ctx context.Context, req *dto.GetUserAuthMethodRequest) (*dto.GetUserAuthMethodResponse, error) {
	return newGetUserAuthMethodLogic(ctx, s.deps).GetUserAuthMethod(req)
}

func (s *Service) UpdateUserAuthMethod(ctx context.Context, req *dto.UpdateUserAuthMethodRequest) error {
	return newUpdateUserAuthMethodLogic(ctx, s.deps).UpdateUserAuthMethod(req)
}

func (s *Service) DeleteUserDevice(ctx context.Context, req *dto.DeleteUserDeivceRequest) error {
	return newDeleteUserDeviceLogic(ctx, s.deps).DeleteUserDevice(req)
}

func (s *Service) UpdateUserDevice(ctx context.Context, req *dto.UserDevice) error {
	return newUpdateUserDeviceLogic(ctx, s.deps).UpdateUserDevice(req)
}

func (s *Service) KickOfflineByUserDevice(ctx context.Context, req *dto.KickOfflineRequest) error {
	return newKickOfflineByUserDeviceLogic(ctx, s.deps).KickOfflineByUserDevice(req)
}

func (s *Service) GetUserLoginLogs(ctx context.Context, req *dto.GetUserLoginLogsRequest) (*dto.GetUserLoginLogsResponse, error) {
	return newGetUserLoginLogsLogic(ctx, s.deps).GetUserLoginLogs(req)
}

func (s *Service) UpdateUserBasicInfo(ctx context.Context, req *dto.UpdateUserBasiceInfoRequest) error {
	return newUpdateUserBasicInfoLogic(ctx, s.deps).UpdateUserBasicInfo(req)
}

func (s *Service) UpdateUserNotifySetting(ctx context.Context, req *dto.UpdateUserNotifySettingRequest) error {
	return newUpdateUserNotifySettingLogic(ctx, s.deps).UpdateUserNotifySetting(req)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.BillingTransactor
	repository.IdentityTransactor
	Node() repository.NodeRepo
}
