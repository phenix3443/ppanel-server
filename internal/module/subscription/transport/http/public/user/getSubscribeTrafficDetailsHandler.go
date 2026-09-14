package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/subscription"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// GetSubscribeTrafficDetailsHandler documents Get Subscribe Traffic Details.
//
// @Summary Get Subscribe Traffic Details
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetSubscribeTrafficDetailsRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.GetSubscribeTrafficDetailsResponse}
// @Router /v1/public/user/traffic/details [get]
func GetSubscribeTrafficDetailsHandler(service subscription.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetSubscribeTrafficDetailsRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		if validateErr := validation.Validate(&req); validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := service.GetSubscribeTrafficDetails(c, &req)
		httpx.HttpResult(ctx, resp, err)
	}
}
