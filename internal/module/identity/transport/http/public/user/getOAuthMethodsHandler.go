package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.GetOAuthMethodsResponse

// GetOAuthMethodsHandler documents Get OAuth Methods.
//
// @Summary Get OAuth Methods
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.GetOAuthMethodsResponse}
// @Router /v1/public/user/oauth_methods [get]
func GetOAuthMethodsHandler(service identity.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := service.GetOAuthMethods(c)
		httpx.HttpResult(ctx, resp, err)
	}
}
