package console

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.ServerTotalDataResponse

// QueryServerTotalDataHandler documents Query server total data.
//
// @Summary Query server total data
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.ServerTotalDataResponse}
// @Router /v1/admin/console/server [get]
func QueryServerTotalDataHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.QueryServerTotalData(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
