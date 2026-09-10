package portal

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// QueryPurchaseOrderHandler documents Query Purchase Order.
//
// @Summary Query Purchase Order
// @Tags user
// @Accept json
// @Produce json
// @Param request query dto.QueryPurchaseOrderRequest false "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.QueryPurchaseOrderResponse}
// @Router /v1/public/portal/order/status [get]
func QueryPurchaseOrderHandler(service billing.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.QueryPurchaseOrderRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		resp, err := service.QueryPurchaseOrder(c, &req)
		httpx.HttpResult(ctx, resp, err)
	}
}
