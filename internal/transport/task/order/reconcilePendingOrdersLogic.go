package order

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

const (
	pendingOrderReconcileBatchSize = 500
	pendingOrderExpiry             = 15 * time.Minute
)

// ReconcilePendingOrdersLogic is a durable fallback for deferred close tasks.
// State transitions in CloseOrder remain conditional, so a late callback can
// safely race this scan without turning a paid order back into a close order.
type ReconcilePendingOrdersLogic struct {
	deps Dependencies
}

func NewReconcilePendingOrdersLogic(deps Dependencies) *ReconcilePendingOrdersLogic {
	return &ReconcilePendingOrdersLogic{deps: deps}
}

func (l *ReconcilePendingOrdersLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	var afterID int64
	var unconfirmed int
	cutoff := time.Now().Add(-pendingOrderExpiry)
	for {
		orders, err := l.deps.Store.Order().QueryOrdersByStatusAfterID(ctx, OrderStatusPending, afterID, pendingOrderReconcileBatchSize)
		if err != nil {
			return err
		}
		for _, orderInfo := range orders {
			afterID = orderInfo.Id
			if orderInfo.CreatedAt.After(cutoff) {
				continue
			}
			if err := l.deps.Billing.CloseOrder(ctx, &dto.CloseOrderRequest{OrderNo: orderInfo.OrderNo}); err != nil {
				// Staying pending until the gateway confirms payment is the
				// intended behaviour for EPay orders, and the same orders
				// recur every scan — count them once instead of emitting a
				// per-order error each minute.
				if errors.Is(err, billing.ErrGatewayUnconfirmed) {
					unconfirmed++
					continue
				}
				// Keep the order pending for a later reconciliation instead of
				// failing the entire batch because one gateway is unavailable.
				logger.WithContext(ctx).Error("[ReconcilePendingOrders] close failed",
					logger.Field("orderNo", orderInfo.OrderNo),
					logger.Field("error", err.Error()),
				)
			}
		}
		if len(orders) < pendingOrderReconcileBatchSize {
			if unconfirmed > 0 {
				logger.WithContext(ctx).Infow("[ReconcilePendingOrders] EPay orders remain pending awaiting gateway payment confirmation",
					logger.Field("count", unconfirmed),
				)
			}
			return nil
		}
	}
}
