package common

import (
	"testing"

	"github.com/perfect-panel/server/internal/model/auth"
)

func TestIsPublicAuthMethodAvailable(t *testing.T) {
	enabled := true
	disabled := false

	tests := []struct {
		name   string
		method *auth.Auth
		want   bool
	}{
		{
			name: "email enabled",
			method: &auth.Auth{
				Method:  "email",
				Enabled: &enabled,
			},
			want: true,
		},
		{
			name: "google enabled with config",
			method: &auth.Auth{
				Method:  "google",
				Enabled: &enabled,
				Config:  `{"client_id":"client-id","client_secret":"client-secret","redirect_url":""}`,
			},
			want: true,
		},
		{
			name: "telegram enabled without bot token",
			method: &auth.Auth{
				Method:  "telegram",
				Enabled: &enabled,
				Config:  `{"bot_token":"","enable_notify":false,"webhook_domain":""}`,
			},
			want: false,
		},
		{
			name: "github remains hidden",
			method: &auth.Auth{
				Method:  "github",
				Enabled: &enabled,
				Config:  `{"client_id":"client-id","client_secret":"client-secret","redirect_url":"https://example.com"}`,
			},
			want: false,
		},
		{
			name: "disabled method hidden",
			method: &auth.Auth{
				Method:  "telegram",
				Enabled: &disabled,
				Config:  `{"bot_token":"token"}`,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPublicAuthMethodAvailable(tt.method); got != tt.want {
				t.Fatalf("isPublicAuthMethodAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}
