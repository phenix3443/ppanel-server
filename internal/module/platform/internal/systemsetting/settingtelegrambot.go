package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/pkg/logger"
)

type SettingTelegramBotLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewSettingTelegramBotLogic setting telegram bot
func newSettingTelegramBotLogic(ctx context.Context, deps Deps) *SettingTelegramBotLogic {
	return &SettingTelegramBotLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *SettingTelegramBotLogic) SettingTelegramBot() error {
	l.deps.reinit("telegram")
	return nil
}
