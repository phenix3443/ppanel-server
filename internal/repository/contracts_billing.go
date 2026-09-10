package repository

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"gorm.io/gorm"
)

// OrderRepo order 数据访问接口
type OrderRepo interface {
	Insert(ctx context.Context, data *order.Order, tx ...*gorm.DB) error
	FindOne(ctx context.Context, id int64) (*order.Order, error)
	FindOneByOrderNo(ctx context.Context, orderNo string) (*order.Order, error)
	FindOneByIdempotencyKey(ctx context.Context, key string) (*order.Order, error)
	FindOneByOrderNoForUpdate(ctx context.Context, orderNo string) (*order.Order, error)
	Update(ctx context.Context, data *order.Order, tx ...*gorm.DB) error
	Delete(ctx context.Context, id int64, tx ...*gorm.DB) error
	UpdateOrderStatusFrom(ctx context.Context, orderNo string, from, status uint8, tx ...*gorm.DB) (bool, error)
	UpdatePaymentExpectation(ctx context.Context, orderNo string, amount int64, currency string, tx ...*gorm.DB) (bool, error)
	SetPaymentTradeNoIfEmpty(ctx context.Context, orderNo, tradeNo string, tx ...*gorm.DB) (bool, error)
	MarkOrderPaid(ctx context.Context, orderNo, tradeNo string, tx ...*gorm.DB) (bool, error)
	CountPendingByPaymentID(ctx context.Context, paymentID int64) (int64, error)
	QueryOrdersByStatusAfterID(ctx context.Context, status uint8, afterID int64, limit int) ([]*order.Order, error)
	CountUserCouponUsage(ctx context.Context, userID int64, coupon string) (int64, error)
	QueryOrderListByPage(ctx context.Context, page, size int, status uint8, user, subscribe int64, search string) (int64, []*order.Details, error)
	FindOneDetails(ctx context.Context, id int64) (*order.Details, error)
	FindOneDetailsByOrderNo(ctx context.Context, orderNo string) (*order.Details, error)
	QueryMonthlyOrders(ctx context.Context, date time.Time) (order.OrdersTotal, error)
	QueryDateOrders(ctx context.Context, date time.Time) (order.OrdersTotal, error)
	QueryTotalOrders(ctx context.Context) (order.OrdersTotal, error)
	QueryMonthlyUserCounts(ctx context.Context, date time.Time) (int64, int64, error)
	QueryDateUserCounts(ctx context.Context, date time.Time) (int64, int64, error)
	QueryTotalUserCounts(ctx context.Context) (int64, int64, error)
	IsUserEligibleForNewOrder(ctx context.Context, userID int64) (bool, error)
	QueryDailyOrdersList(ctx context.Context, date time.Time) ([]order.OrdersTotalWithDate, error)
	// QueryDailyReport totals one day's settled orders with the plan and
	// payment-method breakdowns the daily operations report needs.
	QueryDailyReport(ctx context.Context, date time.Time) (*order.DailyReport, error)
	QueryMonthlyOrdersList(ctx context.Context, date time.Time) ([]order.OrdersTotalWithDate, error)
}

// OrderEventRepo is deliberately separate from OrderRepo: order mutations
// write outbox rows atomically, while delivery workers and SSE handlers only
// need to read and mark these durable records.
type OrderEventRepo interface {
	FindOne(ctx context.Context, id int64) (*order.Event, error)
	ListAfter(ctx context.Context, orderNo string, afterID int64, limit int) ([]*order.Event, error)
	EarliestID(ctx context.Context, orderNo string) (int64, error)
	ListUnpublished(ctx context.Context, limit int) ([]*order.Event, error)
	MarkPublished(ctx context.Context, id int64, publishedAt time.Time) (bool, error)
	DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// PaymentRepo payment 数据访问接口
type PaymentRepo interface {
	Insert(ctx context.Context, data *payment.Payment, tx ...*gorm.DB) error
	FindOne(ctx context.Context, id int64) (*payment.Payment, error)
	Update(ctx context.Context, data *payment.Payment, tx ...*gorm.DB) error
	Delete(ctx context.Context, id int64, tx ...*gorm.DB) error
	FindOneByPaymentToken(ctx context.Context, token string) (*payment.Payment, error)
	FindAll(ctx context.Context) ([]*payment.Payment, error)
	FindListByPage(ctx context.Context, page, size int, req *payment.Filter) (int64, []*payment.Payment, error)
	FindAvailableMethods(ctx context.Context) ([]*payment.Payment, error)
}

// CouponRepo coupon 数据访问接口
type CouponRepo interface {
	Insert(ctx context.Context, data *coupon.Coupon) error
	FindOne(ctx context.Context, id int64) (*coupon.Coupon, error)
	FindOneByCode(ctx context.Context, code string) (*coupon.Coupon, error)
	Update(ctx context.Context, data *coupon.Coupon) error
	Delete(ctx context.Context, id int64) error
	UpdateCount(ctx context.Context, code string) error
	// ReserveUsage atomically claims one coupon use. now is a Unix
	// millisecond timestamp, matching the stored start/expire columns.
	ReserveUsage(ctx context.Context, code string, now int64, tx ...*gorm.DB) (bool, error)
	ReleaseUsage(ctx context.Context, code string, tx ...*gorm.DB) error
	QueryCouponListByPage(ctx context.Context, page, size int, subscribe int64, search string) (total int64, list []*coupon.Coupon, err error)
	BatchDelete(ctx context.Context, ids []int64) error
}
