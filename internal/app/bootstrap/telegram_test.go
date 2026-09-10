package bootstrap

import (
	"context"
	"reflect"
	"testing"

	tgbot "github.com/go-telegram/bot"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/repository"
)

type emptyTelegramTokenStore struct {
	repository.Store
	auth repository.AuthRepo
}

func (s emptyTelegramTokenStore) Auth() repository.AuthRepo { return s.auth }

type emptyTelegramTokenAuthRepo struct {
	repository.AuthRepo
}

func (emptyTelegramTokenAuthRepo) FindOneByMethod(context.Context, string) (*auth.Auth, error) {
	enabled := false
	return &auth.Auth{
		Method:  "telegram",
		Config:  `{"bot_token":"","enable_notify":false,"webhook_domain":"","group_chat_id":""}`,
		Enabled: &enabled,
	}, nil
}

func TestTelegramEmptyTokenClearsPublishedRuntimeState(t *testing.T) {
	runtimeConfig := config.Config{Telegram: config.Telegram{
		Enable:        true,
		BotID:         123,
		BotName:       "old-bot",
		BotToken:      "old-token",
		EnableNotify:  true,
		WebHookDomain: "https://old.example.com",
		GroupChatID:   -100123,
	}}
	botSetterCalled := false
	var publishedBot *tgbot.Bot = new(tgbot.Bot)
	deps := &Dependencies{
		Config: func() config.Config { return runtimeConfig },
		UpdateConfig: func(update func(*config.Config)) {
			update(&runtimeConfig)
		},
		Store: emptyTelegramTokenStore{auth: emptyTelegramTokenAuthRepo{}},
		SetTelegramBot: func(bot *tgbot.Bot) {
			botSetterCalled = true
			publishedBot = bot
		},
	}

	Telegram(deps)

	if !botSetterCalled {
		t.Fatal("Telegram() did not revoke the published bot client")
	}
	if publishedBot != nil {
		t.Fatal("Telegram() retained the previous bot client after clearing the token")
	}
	if !reflect.DeepEqual(runtimeConfig.Telegram, config.Telegram{}) {
		t.Fatalf("runtime Telegram config = %#v, want zero config", runtimeConfig.Telegram)
	}
}
