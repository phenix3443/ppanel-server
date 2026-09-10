package application

import (
	"context"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/internal/render"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type PreviewSubscribeTemplateLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Preview Template
func newPreviewSubscribeTemplateLogic(ctx context.Context, deps Deps) *PreviewSubscribeTemplateLogic {
	return &PreviewSubscribeTemplateLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *PreviewSubscribeTemplateLogic) PreviewSubscribeTemplate(req *dto.PreviewSubscribeTemplateRequest) (resp *dto.PreviewSubscribeTemplateResponse, err error) {
	enable := true
	_, servers, err := l.deps.Nodes.FilterNodeList(l.ctx, &node.FilterNodeParams{
		Page:    1,
		Size:    1000,
		Preload: true,
		Enabled: &enable,
	})
	if err != nil {
		l.Errorf("[PreviewSubscribeTemplateLogic] FindAllServer error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindAllServer error: %v", err.Error())
	}

	data, err := l.deps.Clients.FindOne(l.ctx, req.Id)
	if err != nil {
		l.Errorf("[PreviewSubscribeTemplateLogic] FindOne error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneClient error: %v", err.Error())
	}

	// Preview renders with the application's own defaults so it matches what a
	// client receives when its subscription URL carries no params of its own.
	defaultParams, err := data.DefaultParamValues()
	if err != nil {
		l.Errorf("[PreviewSubscribeTemplateLogic] Ignoring malformed default params %q: %v", data.DefaultParams, err)
	}

	sub := render.NewAdapter(data.SubscribeTemplate, render.WithServers(servers),
		render.WithParams(defaultParams),
		render.WithSiteName("PerfectPanel"),
		render.WithSubscribeName("Test Subscribe"),
		render.WithOutputFormat(data.OutputFormat),
		render.WithUserInfo(render.User{
			ID:           10000,
			Password:     "test-password",
			ExpiredAt:    timeutil.Now().AddDate(1, 0, 0),
			Download:     0,
			Upload:       0,
			Traffic:      1000,
			SubscribeURL: "https://example.com/subscribe",
		}))
	// Get client config
	a, err := sub.Client()
	if err != nil {
		l.Errorf("[PreviewSubscribeTemplateLogic] Client error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrMsg(err.Error()), "Client error: %v", err.Error())
	}
	bytes, err := a.Build()
	if err != nil {
		l.Errorf("[PreviewSubscribeTemplateLogic] Build error: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrMsg(err.Error()), "Build error: %v", err.Error())
	}
	return &dto.PreviewSubscribeTemplateResponse{
		Template: string(bytes),
	}, nil
}
