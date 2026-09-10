package order

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
)

// CloseOrderHandler documents Close order.
//
// @Summary Close order
// @Tags user
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CloseOrderRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/public/order/close [post]
func CloseOrderHandler(service billing.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.CloseOrderRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		err := service.CloseOrder(c, &req)
		if errors.Is(err, billing.ErrGatewayUnconfirmed) {
			// Keep the existing response envelope while exposing a retryable
			// business conflict instead of an opaque internal-server error.
			err = xerr.NewErrCodeMsg(409, "PAYMENT_STATUS_UNCONFIRMED")
		}
		httpx.HttpResult(ctx, nil, err)
	}
}
