package withdrawal

import (
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
)

var (
	_ func(billing.Service) app.HandlerFunc = GetWithdrawalListHandler
	_ func(billing.Service) app.HandlerFunc = ReviewWithdrawalHandler
)
