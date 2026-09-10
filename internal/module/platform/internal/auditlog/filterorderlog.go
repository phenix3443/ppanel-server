package auditlog

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type FilterOrderLogLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newFilterOrderLogLogic(ctx context.Context, deps Deps) *FilterOrderLogLogic {
	return &FilterOrderLogLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// FilterOrderLog returns durable order-creation audit entries.
func (l *FilterOrderLogLogic) FilterOrderLog(req *dto.FilterOrderLogRequest) (*dto.FilterOrderLogResponse, error) {
	data, total, err := l.deps.Logs.FilterSystemLog(l.ctx, &log.FilterParams{
		Page:     req.Page,
		Size:     req.Size,
		Type:     log.TypeOrderCreated.Uint8(),
		ObjectID: req.UserId,
		Data:     req.Date,
		Search:   req.Search,
	})
	if err != nil {
		l.Errorf("[FilterOrderLog] failed to filter system log: %v", err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "failed to filter system log: %v", err)
	}

	list := make([]dto.OrderLog, 0, len(data))
	for _, datum := range data {
		var content log.OrderCreated
		if err := content.Unmarshal([]byte(datum.Content)); err != nil {
			l.Errorf("[FilterOrderLog] failed to unmarshal content: %v", err)
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "corrupt order log %d: %v", datum.Id, err)
		}
		list = append(list, dto.OrderLog{
			Id:               datum.Id,
			UserId:           datum.ObjectID,
			OrderNo:          content.OrderNo,
			OrderType:        content.OrderType,
			Quantity:         content.Quantity,
			Price:            content.Price,
			Amount:           content.Amount,
			GiftAmount:       content.GiftAmount,
			Discount:         content.Discount,
			CouponDiscount:   content.CouponDiscount,
			PaymentId:        content.PaymentID,
			Method:           content.Method,
			FeeAmount:        content.FeeAmount,
			SubscribeId:      content.SubscribeID,
			Source:           content.Source,
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

	return &dto.FilterOrderLogResponse{Total: total, List: list}, nil
}
