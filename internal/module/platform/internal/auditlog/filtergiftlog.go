package auditlog

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type FilterGiftLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Filter gift log
func newFilterGiftLogLogic(ctx context.Context, deps Deps) *FilterGiftLogLogic {
	return &FilterGiftLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *FilterGiftLogLogic) FilterGiftLog(req *dto.FilterGiftLogRequest) (resp *dto.FilterGiftLogResponse, err error) {
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeGift.Uint8(),
		ObjectID: req.UserId,
		Data:     req.Date,
		Search:   req.Search,
	})

	if err != nil {
		l.Errorf("[FilterGiftLog] failed to filter system log: %v", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "failed to filter system log: %v", err.Error())
	}

	var list []dto.GiftLog
	for _, datum := range data {
		var content log.Gift
		err = content.Unmarshal([]byte(datum.Content))
		if err != nil {
			l.Errorf("[FilterGiftLog] failed to unmarshal content: %v", err.Error())
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt gift log %d: %v", datum.Id, err)
		}
		list = append(list, dto.GiftLog{
			Type:             content.Type,
			UserId:           datum.ObjectID,
			OrderNo:          content.OrderNo,
			SubscribeId:      content.SubscribeId,
			Amount:           content.Amount,
			Balance:          content.Balance,
			Remark:           content.Remark,
			Timestamp:        content.Timestamp,
			ClientIP:         content.ClientIP,
			UserAgent:        content.UserAgent,
			ActorID:          content.ActorID,
			IPCountryCode:    content.IPCountryCode,
			IPCountry:        content.IPCountry,
			IPRegion:         content.IPRegion,
			IPCity:           content.IPCity,
			IPASN:            content.IPASN,
			IPASOrganization: content.IPASOrganization,
		})
	}

	return &dto.FilterGiftLogResponse{
		Total: total,
		List:  list,
	}, nil
}
