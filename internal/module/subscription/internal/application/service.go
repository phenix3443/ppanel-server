// Service assembly for the client-application subdomain of the subscription
// module: managing the subscribe clients and previewing their delivery
// templates. Only the module facade may reach it.
package application

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Clients repository.ClientRepo
	// Nodes is a read-only view onto the network domain used by the
	// template preview.
	Nodes repository.NodeRepo
}

// Service is the client-application entry point used by the subscription
// facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) CreateSubscribeApplication(ctx context.Context, req *dto.CreateSubscribeApplicationRequest) (*dto.SubscribeApplication, error) {
	return newCreateSubscribeApplicationLogic(ctx, s.deps).CreateSubscribeApplication(req)
}

func (s *Service) UpdateSubscribeApplication(ctx context.Context, req *dto.UpdateSubscribeApplicationRequest) (*dto.SubscribeApplication, error) {
	return newUpdateSubscribeApplicationLogic(ctx, s.deps).UpdateSubscribeApplication(req)
}

func (s *Service) DeleteSubscribeApplication(ctx context.Context, req *dto.DeleteSubscribeApplicationRequest) error {
	return newDeleteSubscribeApplicationLogic(ctx, s.deps).DeleteSubscribeApplication(req)
}

func (s *Service) GetSubscribeApplicationList(ctx context.Context, req *dto.GetSubscribeApplicationListRequest) (*dto.GetSubscribeApplicationListResponse, error) {
	return newGetSubscribeApplicationListLogic(ctx, s.deps).GetSubscribeApplicationList(req)
}

func (s *Service) PreviewSubscribeTemplate(ctx context.Context, req *dto.PreviewSubscribeTemplateRequest) (*dto.PreviewSubscribeTemplateResponse, error) {
	return newPreviewSubscribeTemplateLogic(ctx, s.deps).PreviewSubscribeTemplate(req)
}
