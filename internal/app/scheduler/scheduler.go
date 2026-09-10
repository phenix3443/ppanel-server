package scheduler

import (
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/pkg/logger"
)

type Service struct {
	server *asynq.Scheduler
}

func NewService(redisConfig config.RedisConfig, appLocation string) *Service {
	return &Service{
		server: initService(redisConfig, appLocation),
	}
}

func (m *Service) Start() {
	logger.Infof("start scheduler service")
	// schedule check subscription task: every 60 seconds
	checkTask := asynq.NewTask(taskqueue.SchedulerCheckSubscription, nil)
	if _, err := m.server.Register("@every 60s", checkTask); err != nil {
		logger.Errorf("register check subscription task failed: %s", err.Error())
	}
	// schedule aggregated traffic flush task: every 60 seconds
	flushTrafficTask := asynq.NewTask(taskqueue.SchedulerFlushTraffic, nil)
	if _, err := m.server.Register("@every 60s", flushTrafficTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register flush traffic task failed: %s", err.Error())
	}
	// Paid order state doubles as a durable activation outbox. Reconcile it
	// periodically so a transient Redis outage cannot strand a paid order.
	reconcilePaidOrdersTask := asynq.NewTask(taskqueue.SchedulerReconcilePaidOrders, nil)
	if _, err := m.server.Register("@every 60s", reconcilePaidOrdersTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register paid order reconciliation task failed: %s", err.Error())
	}
	// A one-shot close task can be lost before enqueue or exhaust retries. The
	// pending-state reconciler is the durable backstop for stock/coupon holds.
	reconcilePendingOrdersTask := asynq.NewTask(taskqueue.SchedulerReconcilePendingOrders, nil)
	if _, err := m.server.Register("@every 60s", reconcilePendingOrdersTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register pending order reconciliation task failed: %s", err.Error())
	}
	// Drain the order-event outbox frequently enough for interactive checkout,
	// while retaining the database event record as the recovery path.
	publishOrderEventsTask := asynq.NewTask(taskqueue.SchedulerPublishOrderEvents, nil)
	if _, err := m.server.Register("@every 5s", publishOrderEventsTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register order event publisher task failed: %s", err.Error())
	}
	// Drain the generic domain-event outbox: registration trials and future
	// cross-module events ride on it.
	dispatchDomainEventsTask := asynq.NewTask(taskqueue.SchedulerDispatchDomainEvents, nil)
	if _, err := m.server.Register("@every 5s", dispatchDomainEventsTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register domain event dispatcher task failed: %s", err.Error())
	}
	cleanupOrderEventsTask := asynq.NewTask(taskqueue.SchedulerCleanupOrderEvents, nil)
	if _, err := m.server.Register("0 3 * * *", cleanupOrderEventsTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register order event cleanup task failed: %s", err.Error())
	}
	//// schedule total server data task: every 5 minutes
	//totalServerDataTask := asynq.NewTask(taskqueue.SchedulerTotalServerData, nil)
	//if _, err := m.server.Register("@every 180s", totalServerDataTask); err != nil {
	//	logger.Errorf("register total server data task failed: %s", err.Error())
	//}
	// schedule reset traffic task: every day at 00:30
	resetTrafficTask := asynq.NewTask(taskqueue.SchedulerResetTraffic, nil)
	if _, err := m.server.Register("30 0 * * *", resetTrafficTask); err != nil {
		logger.Errorf("register reset traffic task failed: %s", err.Error())
	}

	// schedule pre-expiry reminder task: every day at 10:00. A reminder is
	// user-facing, so it goes out during the day rather than overnight, and
	// once daily rather than on the minute-by-minute lifecycle sweep.
	remindExpiringTask := asynq.NewTask(taskqueue.SchedulerRemindExpiringSubscriptions, nil)
	if _, err := m.server.Register("0 10 * * *", remindExpiringTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register expiring subscription reminder task failed: %s", err.Error())
	}

	// schedule daily order report task: every day at 00:10, reporting the
	// day that just ended
	dailyOrderReportTask := asynq.NewTask(taskqueue.SchedulerDailyOrderReport, nil)
	if _, err := m.server.Register("10 0 * * *", dailyOrderReportTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register daily order report task failed: %s", err.Error())
	}

	// schedule traffic stat task: every day at 00:00
	trafficStatTask := asynq.NewTask(taskqueue.SchedulerTrafficStat, nil)
	if _, err := m.server.Register("0 0 * * *", trafficStatTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register traffic stat task failed: %s", err.Error())
	}
	// Retention runs independently after daily statistics. It retries failures
	// instead of waiting silently for the next day's statistics task.
	logCleanupTask := asynq.NewTask(taskqueue.SchedulerLogCleanup, nil)
	if _, err := m.server.Register("30 2 * * *", logCleanupTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register log cleanup task failed: %s", err.Error())
	}

	// schedule update exchange rate task: every day at 01:00
	rateTask := asynq.NewTask(taskqueue.SchedulerExchangeRate, nil)
	if _, err := m.server.Register("0 1 * * *", rateTask, asynq.MaxRetry(3)); err != nil {
		logger.Errorf("register update exchange rate task failed: %s", err.Error())
	}

	if err := m.server.Run(); err != nil {
		logger.Errorf("run scheduler failed: %s", err.Error())
	}
}

func (m *Service) Stop() {
	logger.Info("stop scheduler service")
	m.server.Shutdown()
}

func initService(redisConfig config.RedisConfig, appLocation string) *asynq.Scheduler {
	location, err := time.LoadLocation(appLocation)
	if err != nil {
		logger.Errorf("load timezone location %q failed: %v, falling back to Local", appLocation, err)
		location = time.Local
	}
	return asynq.NewScheduler(
		asynq.RedisClientOpt{Addr: redisConfig.Host, Password: redisConfig.Pass, DB: 5},
		&asynq.SchedulerOpts{
			Location: location,
		},
	)
}
