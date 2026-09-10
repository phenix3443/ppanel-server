// Package adminorder implements the admin-side order management subdomain of
// the billing module. Only the module facade may reach it.
package adminorder

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/internal/orderaudit"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// Transactor mirrors the facade's billing-scoped transaction port.
type Transactor interface {
	InBillingTx(ctx context.Context, fn func(repository.BillingStore) error) error
}

// ActivationEnqueuer mirrors the facade's activation queue port.
type ActivationEnqueuer interface {
	EnqueueActivation(ctx context.Context, orderNo string) error
}

// PlanNameReader is the read port onto the subscription domain's plan
// catalogue, used to label the daily report's plan breakdown.
type PlanNameReader interface {
	FindOne(ctx context.Context, id int64) (*subscribe.Subscribe, error)
}

type Service struct {
	orders   repository.OrderRepo
	payments repository.PaymentRepo
	tx       Transactor
	queue    ActivationEnqueuer
	// plans resolves plan names for the daily report; optional so callers
	// that only manage orders need not provide it.
	plans PlanNameReader
}

func NewService(orders repository.OrderRepo, payments repository.PaymentRepo, tx Transactor, queue ActivationEnqueuer, plans PlanNameReader) *Service {
	return &Service{orders: orders, payments: payments, tx: tx, queue: queue, plans: plans}
}

func (s *Service) Create(ctx context.Context, req *dto.CreateOrderRequest) error {
	log := logger.WithContext(ctx)
	if req.Status != 0 && req.Status != 1 {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_INITIAL_ORDER_STATUS"), "admin-created orders must start pending")
	}
	paymentMethod, err := s.payments.FindOne(ctx, req.PaymentId)
	if err != nil {
		log.Error("[CreateOrder] PaymentMethod Not Found", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.PaymentMethodNotFound), "PaymentMethod not found: %v", err.Error())
	}

	orderInfo := &order.Order{
		UserId:         req.UserId,
		OrderNo:        order.GenerateTradeNo(),
		Type:           req.Type,
		Quantity:       req.Quantity,
		Price:          req.Price,
		Amount:         req.Amount,
		Discount:       req.Discount,
		Coupon:         req.Coupon,
		CouponDiscount: req.CouponDiscount,
		PaymentId:      req.PaymentId,
		Method:         paymentMethod.Platform,
		FeeAmount:      req.FeeAmount,
		TradeNo:        req.TradeNo,
		Status:         1,
		SubscribeId:    req.SubscribeId,
	}
	if err := s.tx.InBillingTx(ctx, func(txStore repository.BillingStore) error {
		if err := txStore.Order().Insert(ctx, orderInfo); err != nil {
			return err
		}
		return orderaudit.InsertCreated(ctx, txStore.Log(), orderInfo, orderaudit.SourceAdmin)
	}); err != nil {
		log.Error("[CreateOrder] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "Insert error: %v", err.Error())
	}
	return nil
}

func (s *Service) List(ctx context.Context, req *dto.GetOrderListRequest) (*dto.GetOrderListResponse, error) {
	total, list, err := s.orders.QueryOrderListByPage(ctx, int(req.Page), int(req.Size), req.Status, req.UserId, req.SubscribeId, req.Search)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetOrderList] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryOrderListByPage error: %v", err.Error())
	}
	resp := &dto.GetOrderListResponse{}
	resp.List = make([]dto.Order, 0)
	mapping.DeepCopy(&resp.List, list)
	resp.Total = total
	return resp, nil
}

func (s *Service) UpdateStatus(ctx context.Context, req *dto.UpdateOrderStatusRequest) error {
	log := logger.WithContext(ctx)
	info, err := s.orders.FindOne(ctx, req.Id)
	if err != nil {
		log.Errorw("[UpdateOrderStatus] FindOne error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOne error: %v", err.Error())
	}

	// Orders have a deliberately narrow state machine. Arbitrary status writes
	// could resurrect terminal orders or skip the activation workflow.
	if req.Status != 2 && req.Status != 3 {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_ORDER_TRANSITION"), "only pending orders may be marked paid or closed")
	}
	if req.Status == 2 && req.TradeNo == "" {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "TRADE_NO_REQUIRED"), "trade_no is required when marking an order paid")
	}
	if req.Status == 3 && (req.PaymentId != 0 || req.TradeNo != "") {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_ORDER_CLOSE_REQUEST"), "payment_id and trade_no are not allowed when closing an order")
	}

	var transitioned bool
	err = s.tx.InBillingTx(ctx, func(txStore repository.BillingStore) error {
		orderStore := txStore.Order()
		current, err := orderStore.FindOneByOrderNoForUpdate(ctx, info.OrderNo)
		if err != nil {
			return err
		}
		if current.Status != 1 {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}

		if req.Status == 2 {
			if req.PaymentId != 0 {
				paymentMethod, err := txStore.Payment().FindOne(ctx, req.PaymentId)
				if err != nil {
					return errors.Wrapf(xerr.NewErrCode(xerr.PaymentMethodNotFound), "payment method not found: %v", err)
				}
				current.PaymentId = paymentMethod.Id
				current.Method = paymentMethod.Platform
				if err := orderStore.Update(ctx, current); err != nil {
					return err
				}
			}
			transitioned, err = orderStore.MarkOrderPaid(ctx, current.OrderNo, req.TradeNo)
		} else {
			transitioned, err = orderStore.UpdateOrderStatusFrom(ctx, current.OrderNo, 1, 3)
		}
		if err != nil {
			return err
		}
		if !transitioned {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}
		return nil
	})
	if err != nil {
		log.Errorw("[UpdateOrderStatus] Transaction error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Transaction error: %v", err.Error())
	}
	if req.Status != 2 || !transitioned {
		return nil
	}
	if err := s.queue.EnqueueActivation(ctx, info.OrderNo); err != nil {
		// The committed Paid state is a durable outbox; reconciliation will
		// repair an enqueue failure without reversing a real payment.
		return errors.Wrapf(xerr.NewErrCode(xerr.QueueEnqueueError), "enqueue activation: %v", err)
	}
	return nil
}
