package oauthstate

import "testing"

func TestValidateRedirect(t *testing.T) {
	tests := []struct {
		name     string
		redirect string
		siteHost string
		wantErr  bool
	}{
		{name: "same host passes", redirect: "https://panel.example/callback", siteHost: "https://panel.example", wantErr: false},
		{name: "subdomain passes", redirect: "https://app.panel.example/cb", siteHost: "panel.example", wantErr: false},
		{name: "bare domain site host passes", redirect: "http://panel.example:3000/cb", siteHost: "panel.example", wantErr: false},
		{name: "host is case insensitive", redirect: "https://Panel.Example/cb", siteHost: "https://panel.example", wantErr: false},
		{name: "foreign host rejected", redirect: "https://evil.example/phish", siteHost: "https://panel.example", wantErr: true},
		{name: "suffix lookalike rejected", redirect: "https://evilpanel.example/cb", siteHost: "panel.example", wantErr: true},
		{name: "empty site host skips pin", redirect: "https://anywhere.example/cb", siteHost: "", wantErr: false},
		{name: "javascript scheme rejected", redirect: "javascript:alert(1)", siteHost: "", wantErr: true},
		{name: "data scheme rejected", redirect: "data:text/html,x", siteHost: "", wantErr: true},
		{name: "relative redirect rejected", redirect: "/local/path", siteHost: "", wantErr: true},
		{name: "empty redirect rejected", redirect: "", siteHost: "", wantErr: true},
		{name: "scheme-relative redirect rejected", redirect: "//evil.example/cb", siteHost: "panel.example", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRedirect(tt.redirect, tt.siteHost)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateRedirect(%q, %q) error = %v, wantErr %v", tt.redirect, tt.siteHost, err, tt.wantErr)
			}
		})
	}
}
