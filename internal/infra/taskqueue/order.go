package taskqueue

import (
	"crypto/sha256"
	"encoding/hex"
)

const (
	DeferCloseOrder                 = "defer:order:close"
	ForthwithActivateOrder          = "forthwith:order:activate"
	SchedulerReconcilePaidOrders    = "scheduler:order:reconcile-paid"
	SchedulerReconcilePendingOrders = "scheduler:order:reconcile-pending"
	SchedulerPublishOrderEvents     = "scheduler:order:publish-events"
	SchedulerCleanupOrderEvents     = "scheduler:order:cleanup-events"
	// SchedulerDailyOrderReport pushes the previous day's settlement summary
	// to the administrators bound on Telegram.
	SchedulerDailyOrderReport = "scheduler:order:daily-report"
)

type (
	DeferCloseOrderPayload struct {
		OrderNo string `json:"order_no"`
	}
	ForthwithActivateOrderPayload struct {
		OrderNo string `json:"order_no"`
	}
)

func ActivationTaskID(orderNo string) string {
	digest := sha256.Sum256([]byte(orderNo))
	return "order-activation:" + hex.EncodeToString(digest[:])
}
