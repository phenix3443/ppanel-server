package profile

import (
	"context"
	"net/url"
	"strings"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/repository"
)

type telegramBindAuthRepo struct {
	repository.AuthRepo
	config string
}

func (r telegramBindAuthRepo) FindOneByMethod(_ context.Context, _ string) (*auth.Auth, error) {
	enabled := true
	return &auth.Auth{Method: "telegram", Config: r.config, Enabled: &enabled}, nil
}

// Binding sends the signed widget result to this redirect just like login
// does, so it carries the same host pin.
func TestTelegramBindPinsRedirectToSiteHost(t *testing.T) {
	tests := []struct {
		name     string
		siteHost string
		redirect string
		wantOK   bool
	}{
		{name: "same host", siteHost: "panel.example.com", redirect: "https://panel.example.com/bind/telegram", wantOK: true},
		{name: "unpinned deployment", siteHost: "", redirect: "https://panel.example.com/bind/telegram", wantOK: true},
		{name: "foreign host", siteHost: "panel.example.com", redirect: "https://evil.example.net/steal", wantOK: false},
		{name: "non-web scheme", siteHost: "panel.example.com", redirect: "javascript:alert(1)", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logic := newBindOAuthLogic(context.Background(), Deps{
				Auth: telegramBindAuthRepo{
					config: `{"bot_token":"123456:AA-secret","enable_notify":false,"webhook_domain":""}`,
				},
				SiteHost: func() string { return tt.siteHost },
			})

			uri, err := logic.telegram(&dto.BindOAuthRequest{
				Method:   "telegram",
				Redirect: tt.redirect,
			})
			if !tt.wantOK {
				if err == nil {
					t.Fatalf("telegram() error = nil for redirect %q, want rejection", tt.redirect)
				}
				return
			}
			if err != nil {
				t.Fatalf("telegram() error = %v, want nil", err)
			}
			parsed, parseErr := url.Parse(uri)
			if parseErr != nil {
				t.Fatalf("parse url %q: %v", uri, parseErr)
			}
			if got := parsed.Query().Get("return_to"); got != tt.redirect {
				t.Fatalf("return_to = %q, want %q", got, tt.redirect)
			}
			if strings.Contains(uri, "AA-secret") {
				t.Fatalf("url %q leaks the bot token secret", uri)
			}
		})
	}
}
