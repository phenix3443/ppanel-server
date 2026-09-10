package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.GetDeviceListResponse

// GetDeviceListHandler documents Get Device List.
//
// @Summary Get Device List
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.GetDeviceListResponse}
// @Router /v1/public/user/devices [get]
func GetDeviceListHandler(service identity.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := service.GetDeviceList(c)
		httpx.HttpResult(ctx, resp, err)
	}
}
