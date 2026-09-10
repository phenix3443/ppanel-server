package console

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.RevenueStatisticsResponse

// QueryRevenueStatisticsHandler documents Query revenue statistics.
//
// @Summary Query revenue statistics
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.RevenueStatisticsResponse}
// @Router /v1/admin/console/revenue [get]
func QueryRevenueStatisticsHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.QueryRevenueStatistics(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
