// Package requestmeta carries bounded HTTP request metadata across application
// layers and queue boundaries. It intentionally stores the raw User-Agent and
// the address resolved by the HTTP framework without parsing either value.
package requestmeta

import (
	"context"
	"unicode/utf8"
)

const (
	MaxClientIPBytes       = 255
	MaxUserAgentBytes      = 512
	MaxCountryCodeBytes    = 8
	MaxCountryBytes        = 128
	MaxRegionBytes         = 128
	MaxCityBytes           = 128
	MaxASOrganizationBytes = 256
)

type contextKey struct{}

// Metadata identifies the request that caused an auditable operation.
// ActorID is the authenticated caller. It can differ from the object ID of an
// administrative log entry, which identifies the account being changed.
type Metadata struct {
	ClientIP  string `json:"client_ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	ActorID   int64  `json:"actor_id,omitempty"`
	IPMetadata
}

// IPMetadata is derived locally from MaxMind databases. ASNOrganization is
// the organization registered for the autonomous system; it is useful for
// risk analysis but is not guaranteed to be the end user's retail ISP.
type IPMetadata struct {
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

// Enricher adds derived metadata to a request. Implementations must be
// best-effort: unavailable databases and lookup misses must not fail requests.
type Enricher func(Metadata) Metadata

// New returns bounded metadata suitable for logs and durable queue payloads.
func New(clientIP, userAgent string) Metadata {
	return Metadata{
		ClientIP:  Bound(clientIP, MaxClientIPBytes),
		UserAgent: Bound(userAgent, MaxUserAgentBytes),
	}
}

// With stores request metadata in ctx. Empty metadata is still stored so an
// authenticated actor can be attached later by middleware.
func With(ctx context.Context, metadata Metadata) context.Context {
	return context.WithValue(ctx, contextKey{}, Normalize(metadata))
}

// WithActor returns a copy of ctx metadata carrying the authenticated caller.
func WithActor(ctx context.Context, actorID int64) context.Context {
	metadata, _ := From(ctx)
	metadata.ActorID = actorID
	return With(ctx, metadata)
}

// From returns request metadata when the context originated from HTTP or was
// explicitly restored from a durable queue payload.
func From(ctx context.Context) (Metadata, bool) {
	if ctx == nil {
		return Metadata{}, false
	}
	metadata, ok := ctx.Value(contextKey{}).(Metadata)
	return metadata, ok
}

// Normalize bounds every string field before it reaches a process log, audit
// row or durable queue payload.
func Normalize(metadata Metadata) Metadata {
	metadata.ClientIP = Bound(metadata.ClientIP, MaxClientIPBytes)
	metadata.UserAgent = Bound(metadata.UserAgent, MaxUserAgentBytes)
	metadata.IPCountryCode = Bound(metadata.IPCountryCode, MaxCountryCodeBytes)
	metadata.IPCountry = Bound(metadata.IPCountry, MaxCountryBytes)
	metadata.IPRegion = Bound(metadata.IPRegion, MaxRegionBytes)
	metadata.IPCity = Bound(metadata.IPCity, MaxCityBytes)
	metadata.IPASOrganization = Bound(metadata.IPASOrganization, MaxASOrganizationBytes)
	return metadata
}

// Bound truncates an attacker-controlled value by bytes while preserving
// valid UTF-8. This keeps audit rows and access-log entries bounded.
func Bound(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
