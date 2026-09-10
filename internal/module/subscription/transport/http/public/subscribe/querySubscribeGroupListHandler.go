package subscribe

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/pkg/httpx"
)

// Get subscribe group list
func QuerySubscribeGroupListHandler(service subscription.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {

		resp, err := service.QuerySubscribeGroupList(c)
		httpx.HttpResult(ctx, resp, err)
	}
}
