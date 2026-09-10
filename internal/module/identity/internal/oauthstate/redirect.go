package oauthstate

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateRedirect checks a client-supplied OAuth redirect target before it
// is stored as state and later used as a browser redirect. The scheme must
// be web-safe, and when the administrator configured a site host the
// redirect must stay on that host or one of its subdomains; an empty site
// host disables the host pin so existing deployments keep working.
func ValidateRedirect(redirect, siteHost string) error {
	u, err := url.Parse(strings.TrimSpace(redirect))
	if err != nil {
		return fmt.Errorf("parse redirect %q: %w", redirect, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("redirect scheme %q is not allowed", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return fmt.Errorf("redirect %q has no host", redirect)
	}
	allowed := siteHostname(siteHost)
	if allowed == "" || host == allowed || strings.HasSuffix(host, "."+allowed) {
		return nil
	}
	return fmt.Errorf("redirect host %q does not match the configured site host", host)
}

// siteHostname extracts the hostname from the configured site host, which
// administrators record either as a bare domain or as a full URL.
func siteHostname(siteHost string) string {
	raw := strings.TrimSpace(siteHost)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
