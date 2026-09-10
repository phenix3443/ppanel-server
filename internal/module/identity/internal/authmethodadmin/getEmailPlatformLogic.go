package authmethodadmin

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mail"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type GetEmailPlatformLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get email support platform
func newGetEmailPlatformLogic(ctx context.Context, deps Deps) *GetEmailPlatformLogic {
	return &GetEmailPlatformLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetEmailPlatformLogic) GetEmailPlatform() (resp *dto.AuthPlatformResponse, err error) {
	return &dto.AuthPlatformResponse{
		List: mail.GetSupportedPlatforms(),
	}, nil
}
