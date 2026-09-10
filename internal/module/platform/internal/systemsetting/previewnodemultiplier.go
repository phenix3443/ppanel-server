package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type PreViewNodeMultiplierLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// PreView Node Multiplier
func newPreViewNodeMultiplierLogic(ctx context.Context, deps Deps) *PreViewNodeMultiplierLogic {
	return &PreViewNodeMultiplierLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *PreViewNodeMultiplierLogic) PreViewNodeMultiplier() (resp *dto.PreViewNodeMultiplierResponse, err error) {
	now := timeutil.Now()
	ratio := l.deps.multiplier(now)
	return &dto.PreViewNodeMultiplierResponse{
		Ratio:       ratio,
		CurrentTime: now.Format("2006-01-02 15:04:05"),
	}, nil
}
