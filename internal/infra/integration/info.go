// Package integration defines provider metadata shared by payment and messaging integrations.
package integration

// Info describes a supported provider and the configuration fields it accepts.
// It deliberately lives outside the HTTP DTO layer so infrastructure packages
// can expose their capabilities without importing internal application types.
type Info struct {
	Platform                 string            `json:"platform"`
	PlatformURL              string            `json:"platform_url"`
	PlatformFieldDescription map[string]string `json:"platform_field_description"`
}
