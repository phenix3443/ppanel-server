package ads

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	entity "github.com/perfect-panel/server/internal/module/support/entity/ads"
)

// GetPublicAds lists the active ads for the public site.
func (s *Service) GetPublicAds(ctx context.Context, req *dto.GetAdsRequest) (resp *dto.GetAdsResponse, err error) {
	// todo: add ads position and device
	status := 1
	_, data, err := s.repo.GetAdsListByPage(ctx, 1, 200, entity.Filter{
		Status: &status,
	})
	if err != nil {
		return nil, err
	}
	resp = &dto.GetAdsResponse{
		List: make([]dto.Ads, len(data)),
	}
	mapping.DeepCopy(&resp.List, data)
	return
}
