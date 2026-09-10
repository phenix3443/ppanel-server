// Package usersub implements the admin-side user subscription management of
// the subscription module. Only the module facade may reach it.
package usersub

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/repository"
)

type Deps struct {
	Plans    repository.SubscribeRepo
	UserSubs repository.UserSubscriptionRepo
	// Users/Devices/Traffic/Logs are read ports onto the identity, network
	// and platform domains for the admin detail views.
	Users   repository.UserRepo
	Devices repository.UserDeviceRepo
	Cache   repository.UserCacheRepo
	Traffic repository.TrafficRepo
	Logs    repository.LogRepo
	Store   Store
	// SingleModel forbids holding more than one blocking subscription;
	// runtime-mutable, read per request.
	SingleModel func() bool
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) CreateUserSubscribe(ctx context.Context, req *dto.CreateUserSubscribeRequest) error {
	return newCreateUserSubscribeLogic(ctx, s.deps).CreateUserSubscribe(req)
}

func (s *Service) DeleteUserSubscribe(ctx context.Context, req *dto.DeleteUserSubscribeRequest) error {
	return newDeleteUserSubscribeLogic(ctx, s.deps).DeleteUserSubscribe(req)
}

func (s *Service) UpdateUserSubscribe(ctx context.Context, req *dto.UpdateUserSubscribeRequest) error {
	return newUpdateUserSubscribeLogic(ctx, s.deps).UpdateUserSubscribe(req)
}

func (s *Service) GetUserSubscribe(ctx context.Context, req *dto.GetUserSubscribeListRequest) (*dto.GetUserSubscribeListResponse, error) {
	return newGetUserSubscribeLogic(ctx, s.deps).GetUserSubscribe(req)
}

func (s *Service) GetUserSubscribeById(ctx context.Context, req *dto.GetUserSubscribeByIdRequest) (*dto.UserSubscribeDetail, error) {
	return newGetUserSubscribeByIdLogic(ctx, s.deps).GetUserSubscribeById(req)
}

func (s *Service) GetUserSubscribeDevices(ctx context.Context, req *dto.GetUserSubscribeDevicesRequest) (*dto.GetUserSubscribeDevicesResponse, error) {
	return newGetUserSubscribeDevicesLogic(ctx, s.deps).GetUserSubscribeDevices(req)
}

func (s *Service) GetUserSubscribeLogs(ctx context.Context, req *dto.GetUserSubscribeLogsRequest) (*dto.GetUserSubscribeLogsResponse, error) {
	return newGetUserSubscribeLogsLogic(ctx, s.deps).GetUserSubscribeLogs(req)
}

func (s *Service) GetUserSubscribeResetTrafficLogs(ctx context.Context, req *dto.GetUserSubscribeResetTrafficLogsRequest) (*dto.GetUserSubscribeResetTrafficLogsResponse, error) {
	return newGetUserSubscribeResetTrafficLogsLogic(ctx, s.deps).GetUserSubscribeResetTrafficLogs(req)
}

func (s *Service) GetUserSubscribeTrafficLogs(ctx context.Context, req *dto.GetUserSubscribeTrafficLogsRequest) (*dto.GetUserSubscribeTrafficLogsResponse, error) {
	return newGetUserSubscribeTrafficLogsLogic(ctx, s.deps).GetUserSubscribeTrafficLogs(req)
}

func (s *Service) ResetUserSubscribeToken(ctx context.Context, req *dto.ResetUserSubscribeTokenRequest) error {
	return newResetUserSubscribeTokenLogic(ctx, s.deps).ResetUserSubscribeToken(req)
}

func (s *Service) ResetUserSubscribeTraffic(ctx context.Context, req *dto.ResetUserSubscribeTrafficRequest) error {
	return newResetUserSubscribeTrafficLogic(ctx, s.deps).ResetUserSubscribeTraffic(req)
}

func (s *Service) ToggleUserSubscribeStatus(ctx context.Context, req *dto.ToggleUserSubscribeStatusRequest) error {
	return newToggleUserSubscribeStatusLogic(ctx, s.deps).ToggleUserSubscribeStatus(req)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
}
