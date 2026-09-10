// Service assembly for the tool subdomain of the platform module: system log
// tail, version info, IP geolocation and process restart. Only the module
// facade may reach it.
package tool

import (
	"context"

	"github.com/oschwald/geoip2-golang"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	// LogPath is the logger output file read by the system-log tail.
	LogPath string
	// GeoIP returns the GeoIP database reader; nil (or a nil result) means
	// no database is configured.
	GeoIP func() *geoip2.Reader
	// Restart restarts the transport server.
	Restart func() error
}

// Service is the tool entry point used by the platform facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) GetSystemLog(ctx context.Context) (*dto.LogResponse, error) {
	return newGetSystemLogLogic(ctx, s.deps).GetSystemLog()
}

func (s *Service) GetVersion(ctx context.Context) (*dto.VersionResponse, error) {
	return newGetVersionLogic(ctx, s.deps).GetVersion()
}

func (s *Service) QueryIPLocation(ctx context.Context, req *dto.QueryIPLocationRequest) (*dto.QueryIPLocationResponse, error) {
	return newQueryIPLocationLogic(ctx, s.deps).QueryIPLocation(req)
}

func (s *Service) RestartSystem(ctx context.Context) error {
	return newRestartSystemLogic(ctx, s.deps).RestartSystem()
}
