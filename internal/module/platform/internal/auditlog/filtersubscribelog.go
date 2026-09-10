package auditlog

import (
	"context"
	"strconv"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type FilterSubscribeLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewFilterSubscribeLogLogic Filter subscribe log
func newFilterSubscribeLogLogic(ctx context.Context, deps Deps) *FilterSubscribeLogLogic {
	return &FilterSubscribeLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *FilterSubscribeLogLogic) FilterSubscribeLog(req *dto.FilterSubscribeLogRequest) (resp *dto.FilterSubscribeLogResponse, err error) {
	params := &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeSubscribe.Uint8(),
		Data:     req.Date,
		ObjectID: req.UserId,
	}

	if req.UserSubscribeId != 0 {
		params.Search = `"user_subscribe_id":` + strconv.FormatInt(req.UserSubscribeId, 10)
	}

	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, params)
	if err != nil {
		l.Errorf("[FilterSubscribeLog] failed to filter system log: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "failed to filter system log")
	}

	var list []dto.SubscribeLog
	for _, datum := range data {
		var content log.Subscribe
		err = content.Unmarshal([]byte(datum.Content))
		if err != nil {
			l.Errorf("[FilterSubscribeLog] failed to unmarshal content: %v", err.Error())
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt subscription log %d: %v", datum.Id, err)
		}
		list = append(list, dto.SubscribeLog{
			UserId:           datum.ObjectID,
			Token:            content.Token,
			UserAgent:        content.UserAgent,
			ClientIP:         content.ClientIP,
			UserSubscribeId:  content.UserSubscribeId,
			Timestamp:        datum.CreatedAt.UnixMilli(),
			ActorID:          content.ActorID,
			IPCountryCode:    content.IPCountryCode,
			IPCountry:        content.IPCountry,
			IPRegion:         content.IPRegion,
			IPCity:           content.IPCity,
			IPASN:            content.IPASN,
			IPASOrganization: content.IPASOrganization,
		})
	}

	return &dto.FilterSubscribeLogResponse{
		Total: total,
		List:  list,
	}, nil
}
