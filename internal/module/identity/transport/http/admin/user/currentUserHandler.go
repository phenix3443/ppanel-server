package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/httpx"
)

var _ dto.User

// CurrentUserHandler documents Current user.
//
// @Summary Current user
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.User}
// @Router /v1/admin/user/current [get]
func CurrentUserHandler(service identity.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		resp, err := service.CurrentUser(ctx)
		httpx.HttpResult(c, resp, err)
	}
}
