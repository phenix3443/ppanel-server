package oauth

import (
	"context"
	"net/url"
	"strings"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/repository"
)

type telegramRedirectStore struct {
	auths repository.AuthRepo
}

func (s telegramRedirectStore) Auth() repository.AuthRepo { return s.auths }

type telegramRedirectAuthRepo struct {
	repository.AuthRepo
	config string
}

func (r telegramRedirectAuthRepo) FindOneByMethod(_ context.Context, _ string) (*auth.Auth, error) {
	enabled := true
	return &auth.Auth{Method: "telegram", Config: r.config, Enabled: &enabled}, nil
}

func newTelegramLoginLogic(siteHost string) *OAuthLoginLogic {
	return NewOAuthLoginLogic(context.Background(), OAuthLoginURLDependencies{
		Store: telegramRedirectStore{auths: telegramRedirectAuthRepo{
			config: `{"bot_token":"123456:AA-secret","enable_notify":false,"webhook_domain":""}`,
		}},
		Policy:   &fakeOAuthLoginURLPolicy{},
		SiteHost: siteHost,
	})
}

// Telegram delivers the signed widget result to this redirect, so an
// attacker-supplied target would hand them a victim's credential. The host
// pin must not depend solely on the URL list registered with BotFather.
func TestTelegramLoginPinsRedirectToSiteHost(t *testing.T) {
	tests := []struct {
		name     string
		siteHost string
		redirect string
		wantOK   bool
	}{
		{name: "same host", siteHost: "panel.example.com", redirect: "https://panel.example.com/oauth/telegram", wantOK: true},
		{name: "subdomain", siteHost: "example.com", redirect: "https://panel.example.com/oauth/telegram", wantOK: true},
		{name: "site host as url", siteHost: "https://panel.example.com", redirect: "https://panel.example.com/oauth/telegram", wantOK: true},
		{name: "unpinned deployment", siteHost: "", redirect: "https://panel.example.com/oauth/telegram", wantOK: true},
		{name: "foreign host", siteHost: "panel.example.com", redirect: "https://evil.example.net/steal", wantOK: false},
		{name: "lookalike suffix", siteHost: "example.com", redirect: "https://evilexample.com/steal", wantOK: false},
		{name: "non-web scheme", siteHost: "panel.example.com", redirect: "javascript:alert(1)", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uri, err := newTelegramLoginLogic(tt.siteHost).telegram(&dto.OAthLoginRequest{
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

// A malformed bot token must surface as an error rather than an empty URL.
func TestTelegramLoginRejectsMalformedBotToken(t *testing.T) {
	logic := NewOAuthLoginLogic(context.Background(), OAuthLoginURLDependencies{
		Store: telegramRedirectStore{auths: telegramRedirectAuthRepo{
			config: `{"bot_token":"no-colon-token"}`,
		}},
		Policy:   &fakeOAuthLoginURLPolicy{},
		SiteHost: "panel.example.com",
	})

	if _, err := logic.telegram(&dto.OAthLoginRequest{
		Method:   "telegram",
		Redirect: "https://panel.example.com/oauth/telegram",
	}); err == nil {
		t.Fatal("telegram() error = nil, want malformed-token rejection")
	}
}
