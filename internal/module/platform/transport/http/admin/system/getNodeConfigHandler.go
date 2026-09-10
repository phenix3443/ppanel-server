package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.NodeConfig

// GetNodeConfigHandler documents Get node config.
//
// @Summary Get node config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.NodeConfig}
// @Router /v1/admin/system/node_config [get]
func GetNodeConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetNodeConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
