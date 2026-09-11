package server

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// SetServerTargetVersionHandler documents Set Server Target Version.
//
// 期望版本可以填比当前更旧的——新版本出问题时用同一条路回退。
//
// @Summary Set the desired node version (supports downgrade)
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.SetServerTargetVersionRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/admin/server/target_version [post]
func SetServerTargetVersionHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.SetServerTargetVersionRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		if validateErr := validation.Validate(&req); validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		err := service.SetServerTargetVersion(c, &req)
		httpx.HttpResult(ctx, nil, err)
	}
}
