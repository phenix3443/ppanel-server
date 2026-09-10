package system

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/platform"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.PrivacyPolicyConfig

// GetPrivacyPolicyConfigHandler documents get Privacy Policy Config.
//
// @Summary get Privacy Policy Config
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.PrivacyPolicyConfig}
// @Router /v1/admin/system/privacy [get]
func GetPrivacyPolicyConfigHandler(service platform.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {

		resp, err := service.GetPrivacyPolicyConfig(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
