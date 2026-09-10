package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.User

// QueryUserInfoHandler documents returns the current user profile..
//
// @Summary returns the current user profile.
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.User}
// @Router /v1/public/user/info [get]
func QueryUserInfoHandler(service identity.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := service.QueryUserInfo(c)
		httpx.HttpResult(ctx, resp, err)
	}
}
