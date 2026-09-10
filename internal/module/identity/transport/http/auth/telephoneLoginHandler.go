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

// TelephoneLoginHandler documents User Telephone login.
//
// @Summary User Telephone login
// @Tags common
// @Accept json
// @Produce json
// @Param request body dto.TelephoneLoginRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.LoginResponse}
// @Router /v1/auth/login/telephone [post]
func TelephoneLoginHandler(service identity.Service, verifyConfig func() config.Verify) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.TelephoneLoginRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}
		// get client ip
		req.IP = ctx.ClientIP()
		verify := verifyConfig()
		if verify.LoginVerify {
			verifyTurns := challenge.New(challenge.Config{
				Secret:  verify.TurnstileSecret,
				Timeout: 3 * time.Second,
			})
			if verify, err := verifyTurns.Verify(c, req.CfToken, req.IP); err != nil || !verify {
				err = errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "error: %v, verify: %v", err, verify)
				httpx.HttpResult(ctx, nil, err)
				return
			}
		}
		resp, err := service.TelephoneLogin(c, &req, ctx.ClientIP(), string(ctx.UserAgent()))
		httpx.HttpResult(ctx, resp, err)
	}
}
