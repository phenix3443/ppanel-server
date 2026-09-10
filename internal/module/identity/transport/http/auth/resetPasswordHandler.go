package auth

import (
	"context"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/auth/challenge"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// ResetPasswordHandler documents Reset password.
//
// @Summary Reset password
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.ResetPasswordRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/reset [post]
func ResetPasswordHandler(service identity.Service, verifyConfig func() config.Verify) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		var req dto.ResetPasswordRequest
		if err := httpx.ShouldBind(c, &req); err != nil {
			httpx.ParamErrorResult(c, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(c, validateErr)
			return
		}
		// get client ip
		req.IP = c.ClientIP()
		req.UserAgent = string(c.UserAgent())
		verify := verifyConfig()
		if verify.ResetPasswordVerify {
			verifyTurns := challenge.New(challenge.Config{
				Secret:  verify.TurnstileSecret,
				Timeout: 3 * time.Second,
			})
			if verify, err := verifyTurns.Verify(ctx, req.CfToken, req.IP); err != nil || !verify {
				err = errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "error: %v, verify: %v", err, verify)
				httpx.HttpResult(c, nil, err)
				return
			}
		}
		resp, err := service.ResetPassword(ctx, &req)
		httpx.HttpResult(c, resp, err)
	}
}
