package task

import (
	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/eventbus"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/billing"
	moduleSubscription "github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/task/email"
	"github.com/perfect-panel/server/internal/transport/task/events"
	"github.com/perfect-panel/server/internal/transport/task/maintenance"
	"github.com/perfect-panel/server/internal/transport/task/order"
	"github.com/perfect-panel/server/internal/transport/task/sms"
	"github.com/perfect-panel/server/internal/transport/task/subscription"
	"github.com/perfect-panel/server/internal/transport/task/traffic"
)

type Dependencies struct {
	Email        email.Dependencies
	SMS          sms.Dependencies
	Order        order.Dependencies
	EventBus     *eventbus.Bus
	Traffic      traffic.Dependencies
	Subscription moduleSubscription.Service
	Store        repository.Store
	ExchangeRate *billing.CurrencyRateCache
}

func RegisterHandlers(mux *asynq.ServeMux, deps Dependencies) {
	var taskRepo repository.TaskRepo
	if deps.Store != nil {
		taskRepo = deps.Store.Task()
	}
	// Send email task
	mux.Handle(taskqueue.ForthwithSendEmail, email.NewSendEmailLogic(deps.Email))
	// Send sms task
	mux.Handle(taskqueue.ForthwithSendSms, sms.NewSendSmsLogic(deps.SMS))
	// Defer close order task
	mux.Handle(taskqueue.DeferCloseOrder, order.NewDeferCloseOrderLogic(deps.Order))
	// Forthwith activate order task
	mux.Handle(taskqueue.ForthwithActivateOrder, order.NewActivateOrderLogic(deps.Order.Billing))
	// Recover paid orders whose activation enqueue was interrupted.
	mux.Handle(taskqueue.SchedulerReconcilePaidOrders, order.NewReconcilePaidOrdersLogic(deps.Order))
	// Close stale pending orders even when their one-shot deferred task was
	// lost during a Redis outage or exhausted its retries.
	mux.Handle(taskqueue.SchedulerReconcilePendingOrders, order.NewReconcilePendingOrdersLogic(deps.Order))
	// Deliver durable order events to Redis Pub/Sub. The database remains the
	// source of truth for SSE replay when publication is delayed or duplicated.
	mux.Handle(taskqueue.SchedulerPublishOrderEvents, order.NewPublishOrderEventsLogic(deps.Order))
	// Domain events: the pump publishes outbox rows onto the queue; the
	// delivery worker runs the topic's subscribers per event.
	mux.Handle(taskqueue.SchedulerDispatchDomainEvents, events.NewDispatchDomainEventsLogic(deps.EventBus))
	mux.Handle(taskqueue.EventDeliver, events.NewDeliverDomainEventLogic(deps.EventBus))
	mux.Handle(taskqueue.SchedulerCleanupOrderEvents, order.NewCleanupOrderEventsLogic(deps.Order))
	// Daily settlement summary for administrators bound on Telegram.
	mux.Handle(taskqueue.SchedulerDailyOrderReport, order.NewDailyOrderReportLogic(deps.Order))

	// Forthwith traffic statistics
	mux.Handle(taskqueue.ForthwithTrafficStatistics, traffic.NewTrafficStatisticsLogic(deps.Traffic))
	// Flush aggregated traffic
	mux.Handle(taskqueue.SchedulerFlushTraffic, traffic.NewFlushTrafficLogic(deps.Traffic))

	// Schedule check subscription
	mux.Handle(taskqueue.SchedulerCheckSubscription, subscription.NewCheckSubscriptionLogic(deps.Subscription))
	// Warn owners before their subscription expires.
	mux.Handle(taskqueue.SchedulerRemindExpiringSubscriptions, subscription.NewRemindExpiringLogic(deps.Subscription))

	// Schedule total server data
	mux.Handle(taskqueue.SchedulerTotalServerData, traffic.NewServerDataLogic(deps.Traffic))

	// Schedule reset traffic
	mux.Handle(taskqueue.SchedulerResetTraffic, traffic.NewResetTrafficLogic(deps.Traffic))

	// ScheduledBatchSendEmail
	mux.Handle(taskqueue.ScheduledBatchSendEmail, email.NewBatchEmailLogic(deps.Email))

	// ScheduledTrafficStat
	mux.Handle(taskqueue.SchedulerTrafficStat, traffic.NewStatLogic(deps.Traffic))
	// ScheduledLogCleanup is independent from traffic aggregation so either
	// task can retry without suppressing the other.
	mux.Handle(taskqueue.SchedulerLogCleanup, traffic.NewLogCleanupLogic(deps.Traffic.Store, deps.Traffic.Log))

	// ForthwithQuotaTask
	mux.Handle(taskqueue.ForthwithQuotaTask, maintenance.NewQuotaTaskLogic(deps.Subscription, taskRepo))
	// SchedulerExchangeRate
	mux.Handle(taskqueue.SchedulerExchangeRate, maintenance.NewRateLogic(maintenance.RateDependencies{Store: deps.Store, ExchangeRate: deps.ExchangeRate}))
}
