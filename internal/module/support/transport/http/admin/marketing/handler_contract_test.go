package marketing

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/support"
)

func TestHandlerFactories_return_native_hertz_handlers(t *testing.T) {
	var _ func(support.Service) app.HandlerFunc = CreateBatchSendEmailTaskHandler
	var _ func(support.Service) app.HandlerFunc = CreateQuotaTaskHandler
	var _ func(support.Service) app.HandlerFunc = GetBatchSendEmailTaskListHandler
	var _ func(support.Service) app.HandlerFunc = GetBatchSendEmailTaskStatusHandler
	var _ func(support.Service) app.HandlerFunc = GetPreSendEmailCountHandler
	var _ func(support.Service) app.HandlerFunc = QueryQuotaTaskListHandler
	var _ func(support.Service) app.HandlerFunc = QueryQuotaTaskPreCountHandler
	var _ func(support.Service) app.HandlerFunc = QueryQuotaTaskStatusHandler
	var _ func(support.Service) app.HandlerFunc = StopBatchSendEmailTaskHandler
}
