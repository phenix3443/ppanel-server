package auth

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// DeviceLoginHandler documents Device Login.
//
// @Summary Device Login
// @Description When device security is enabled, requests and response data use a signed data/time/sign envelope. Each request needs a fresh nonce; see docs/design/device-authentication.md. The User-Agent is read from the HTTP header.
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.DeviceLoginRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/login/device [post]
func DeviceLoginHandler(service identity.Service) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.DeviceLoginRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		req.IP = c.ClientIP()
		req.UserAgent = string(c.UserAgent())
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}
		resp, err := service.DeviceLogin(ctx, &req)
		httpx.HttpResult(c, resp, err)
	}
}
