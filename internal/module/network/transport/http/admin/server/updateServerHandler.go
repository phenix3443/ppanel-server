package server

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/transport/http/validation"
	"github.com/perfect-panel/server/pkg/httpx"
)

// UpdateServerHandler documents Update Server.
//
// @Summary Update Server
// @Tags admin
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UpdateServerRequest true "Request parameters"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/admin/server/update [post]
func UpdateServerHandler(service network.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		var req dto.UpdateServerRequest
		if err := httpx.ShouldBind(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		if err := bindProtocolFieldSets(ctx, &req); err != nil {
			httpx.ParamErrorResult(ctx, err)
			return
		}
		validateErr := validation.Validate(&req)
		if validateErr != nil {
			httpx.ParamErrorResult(ctx, validateErr)
			return
		}

		err := service.UpdateServer(c, &req)
		httpx.HttpResult(ctx, nil, err)
	}
}

func bindProtocolFieldSets(ctx *app.RequestContext, req *dto.UpdateServerRequest) error {
	body := ctx.Request.Body()
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil
	}
	var raw struct {
		Protocols []map[string]json.RawMessage `json:"protocols"`
	}
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return err
	}
	if len(raw.Protocols) == 0 {
		return nil
	}
	req.ProtocolFieldSets = make([]map[string]struct{}, len(raw.Protocols))
	for index, protocol := range raw.Protocols {
		fields := make(map[string]struct{}, len(protocol))
		for field := range protocol {
			fields[field] = struct{}{}
		}
		req.ProtocolFieldSets[index] = fields
	}
	return nil
}
