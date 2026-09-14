package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/subscription"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// GetSubscribeTrafficOverviewHandler documents Get Subscribe Traffic Overview.
//
// @Summary Get Subscribe Traffic Overview
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request query dto.GetSubscribeTrafficOverviewRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.GetSubscribeTrafficOverviewResponse}
// @Router /v1/public/user/traffic/overview [get]
func GetSubscribeTrafficOverviewHandler(service subscription.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.GetSubscribeTrafficOverviewRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		if validateErr := validation.Validate(&req); validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := service.GetSubscribeTrafficOverview(c, &req)
		httpx.HttpResult(ctx, resp, err)
	}
}
