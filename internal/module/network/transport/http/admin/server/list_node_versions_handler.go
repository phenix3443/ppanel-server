package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/pkg/httpx"
)

// ListNodeVersionsHandler documents List Node Versions.
//
// @Summary List the node versions that can be pushed to servers
// @Tags admin
// @Produce json
// @Security BearerAuth
// @Success 200 {object} httpx.ResponseSuccessBean{data=dto.ListNodeVersionsResponse}
// @Router /v1/admin/server/node/versions [get]
func ListNodeVersionsHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		resp, err := service.ListNodeVersions(c)
		httpx.HttpResult(ctx, resp, err)
	}
}
