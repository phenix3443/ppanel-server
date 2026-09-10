package authmethodadmin

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/sms"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type GetSmsPlatformLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get sms support platform
func newGetSmsPlatformLogic(ctx context.Context, deps Deps) *GetSmsPlatformLogic {
	return &GetSmsPlatformLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetSmsPlatformLogic) GetSmsPlatform() (resp *dto.AuthPlatformResponse, err error) {
	return &dto.AuthPlatformResponse{
		List: sms.GetSupportedPlatforms(),
	}, nil
}
