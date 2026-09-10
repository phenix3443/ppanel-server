package tool

import (
	"context"
	"net"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type QueryIPLocationLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewQueryIPLocationLogic Query IP Location
func newQueryIPLocationLogic(ctx context.Context, deps Deps) *QueryIPLocationLogic {
	return &QueryIPLocationLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryIPLocationLogic) QueryIPLocation(req *dto.QueryIPLocationRequest) (resp *dto.QueryIPLocationResponse, err error) {
	if l.deps.GeoIP == nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), " GeoIP database not configured")
	}

	ip := net.ParseIP(req.IP)
	record, err := l.deps.GeoIP().City(ip)
	if err != nil {
		l.Errorf("Failed to query IP location: %v", err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Failed to query IP location")
	}

	var country, region, city string
	if record.Country.Names != nil {
		country = record.Country.Names["en"]
	}
	if len(record.Subdivisions) > 0 && record.Subdivisions[0].Names != nil {
		region = record.Subdivisions[0].Names["en"]
	}
	if record.City.Names != nil {
		city = record.City.Names["en"]
	}

	return &dto.QueryIPLocationResponse{
		Country: country,
		Region:  region,
		City:    city,
	}, nil
}
