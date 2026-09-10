package tool

import (
	"context"

	"github.com/perfect-panel/server/internal/app/buildinfo"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type GetVersionLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetVersionLogic Get Version
func newGetVersionLogic(ctx context.Context, deps Deps) *GetVersionLogic {
	return &GetVersionLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetVersionLogic) GetVersion() (resp *dto.VersionResponse, err error) {
	return &dto.VersionResponse{
		Version: buildinfo.Display(),
	}, nil
}
