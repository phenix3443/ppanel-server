// Package orderaudit writes the durable audit record created alongside an
// order. Callers invoke it with the transaction-scoped log repository so the
// order and its audit row commit or roll back together.
package orderaudit

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/pkg/errors"
)

const (
	SourceUser  = "user"
	SourceGuest = "guest"
	SourceAdmin = "admin"
)

// InsertCreated stores a safe order summary and request risk metadata.
func InsertCreated(ctx context.Context, logs repository.LogRepo, data *order.Order, source string) error {
	if logs == nil {
		return errors.New("order audit log repository is unavailable")
	}
	if data == nil {
		return errors.New("order audit data is nil")
	}

	metadata, _ := requestmeta.From(ctx)
	now := timeutil.Now()
	content, err := (&logEntity.OrderCreated{
		Metadata:       metadata,
		OrderNo:        data.OrderNo,
		OrderType:      data.Type,
		Quantity:       data.Quantity,
		Price:          data.Price,
		Amount:         data.Amount,
		GiftAmount:     data.GiftAmount,
		Discount:       data.Discount,
		CouponDiscount: data.CouponDiscount,
		PaymentID:      data.PaymentId,
		Method:         data.Method,
		FeeAmount:      data.FeeAmount,
		SubscribeID:    data.SubscribeId,
		Source:         source,
		Timestamp:      now.UnixMilli(),
	}).Marshal()
	if err != nil {
		return errors.Wrap(err, "marshal order audit log")
	}

	return logs.Insert(ctx, &logEntity.SystemLog{
		Type:     logEntity.TypeOrderCreated.Uint8(),
		Date:     now.Format(time.DateOnly),
		ObjectID: data.UserId,
		Content:  string(content),
	})
}
