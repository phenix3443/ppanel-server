package payment

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
)

func TestHandlerFactories_return_native_hertz_handlers(t *testing.T) {
	var _ func(billing.Service) app.HandlerFunc = CreatePaymentMethodHandler
	var _ func(billing.Service) app.HandlerFunc = DeletePaymentMethodHandler
	var _ func(billing.Service) app.HandlerFunc = GetPaymentMethodListHandler
	var _ func(billing.Service) app.HandlerFunc = GetPaymentPlatformHandler
	var _ func(billing.Service) app.HandlerFunc = UpdatePaymentMethodHandler
}
