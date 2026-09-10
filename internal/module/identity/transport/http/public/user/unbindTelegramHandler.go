package user

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/pkg/httpx"
)

// UnbindTelegramHandler documents Unbind Telegram.
//
// @Summary Unbind Telegram
// @Tags user
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/public/user/unbind_telegram [post]
func UnbindTelegramHandler(service identity.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		err := service.UnbindTelegram(c)
		httpx.HttpResult(ctx, nil, err)
	}
}
