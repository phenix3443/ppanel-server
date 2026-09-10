package subscribe

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/subscription"
)

func TestHandlerFactories_return_native_hertz_handlers(t *testing.T) {
	var _ func(subscription.Service) app.HandlerFunc = BatchDeleteSubscribeGroupHandler
	var _ func(subscription.Service) app.HandlerFunc = BatchDeleteSubscribeHandler
	var _ func(subscription.Service) app.HandlerFunc = CreateSubscribeGroupHandler
	var _ func(subscription.Service) app.HandlerFunc = CreateSubscribeHandler
	var _ func(subscription.Service) app.HandlerFunc = DeleteSubscribeGroupHandler
	var _ func(subscription.Service) app.HandlerFunc = DeleteSubscribeHandler
	var _ func(subscription.Service) app.HandlerFunc = GetSubscribeDetailsHandler
	var _ func(subscription.Service) app.HandlerFunc = GetSubscribeGroupListHandler
	var _ func(subscription.Service) app.HandlerFunc = GetSubscribeListHandler
	var _ func(subscription.Service) app.HandlerFunc = ResetAllSubscribeTokenHandler
	var _ func(subscription.Service) app.HandlerFunc = SubscribeSortHandler
	var _ func(subscription.Service) app.HandlerFunc = UpdateSubscribeGroupHandler
	var _ func(subscription.Service) app.HandlerFunc = UpdateSubscribeHandler
}
