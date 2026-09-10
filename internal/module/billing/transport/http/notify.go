package handler

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/billing/transport/http/notify"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/http/middleware"
)

func RegisterNotifyHandlers(router *server.Hertz, store Store, service billing.Service) {
	group := router.Group("/v1/notify/")
	group.Use(middleware.NotifyMiddleware(store))
	handler := notify.PaymentNotifyHandler(service)
	group.GET("/:platform/:token", handler)
	group.POST("/:platform/:token", handler)
	group.PUT("/:platform/:token", handler)
	group.DELETE("/:platform/:token", handler)
	group.PATCH("/:platform/:token", handler)
	group.OPTIONS("/:platform/:token", handler)
	group.HEAD("/:platform/:token", handler)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	Payment() repository.PaymentRepo
}
