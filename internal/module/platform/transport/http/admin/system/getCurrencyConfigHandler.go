package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.CurrencyConfig

// GetCurrencyConfigHandler documents Get Currency Config.
//
// @Summary Get Currency Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.CurrencyConfig}
// @Router /v1/admin/system/currency_config [get]
func GetCurrencyConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetCurrencyConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
