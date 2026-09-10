package common

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.GetSubscribeClientResponse

// GetClientHandler documents Get Client.
//
// @Summary Get Client
// @Tags common
// @Produce json
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.GetSubscribeClientResponse}
// @Router /v1/common/client [get]
func GetClientHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		resp, err := service.GetClient(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
