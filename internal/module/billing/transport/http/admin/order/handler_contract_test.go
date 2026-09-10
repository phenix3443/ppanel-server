package order

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
)

func TestHandlerFactories_return_native_hertz_handlers(t *testing.T) {
	var _ func(billing.Service) app.HandlerFunc = CreateOrderHandler
	var _ func(billing.Service) app.HandlerFunc = GetOrderListHandler
	var _ func(billing.Service) app.HandlerFunc = UpdateOrderStatusHandler
}
