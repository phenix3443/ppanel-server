package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/support"
	ticket "github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/task/email"
	"github.com/perfect-panel/server/pkg/logger"
)

// newSupportModule wires the support module against the legacy store. The
// adapters below satisfy the module's ports until the owning modules exist
// (ADR-001).
func newSupportModule(store repository.Store, queue *taskqueue.Client, srv *Application) support.Service {
	return support.New(support.Deps{
		Announcements: store.Announcement(),
		Ads:           store.Ads(),
		Documents:     store.Document(),
		Tickets:       store.Ticket(),
		Tasks:         store.Task(),
		Subscriptions: subscriptionReader{store: store},
		Recipients:    store.User(),
		QuotaTargets:  store.UserSubscription(),
		Queue:         marketingQueue{client: queue},
		EmailStopper:  emailWorkerStopper{},
		TicketNotify:  ticketTopicNotifier{srv: srv},
	})
}

// ticketTopicNotifier mirrors ticket lifecycle into the Telegram admin
// group. Best-effort by the port's contract: the group being unconfigured
// or unreachable only logs — the ticket operation already succeeded. The
// mirror runs detached from the request: a user submitting a ticket must
// not wait on Telegram round-trips (the bot client's HTTP timeout is 60s).
type ticketTopicNotifier struct{ srv *Application }

func (n ticketTopicNotifier) enabled() bool {
	return n.srv.Runtime.Config().Telegram.GroupChatID != 0 && n.srv.Notification != nil
}

func (n ticketTopicNotifier) mirror(ctx context.Context, ticketID int64, what string, call func(ctx context.Context) error) {
	if !n.enabled() {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		mirrorCtx, cancel := context.WithTimeout(detached, 15*time.Second)
		defer cancel()
		if err := call(mirrorCtx); err != nil {
			logger.WithContext(mirrorCtx).Errorw("[TicketTopic] "+what+" mirror failed",
				logger.Field("error", err.Error()), logger.Field("ticket_id", ticketID))
		}
	}()
}

func (n ticketTopicNotifier) TicketCreated(ctx context.Context, t *ticket.Ticket) {
	n.mirror(ctx, t.Id, "create", func(ctx context.Context) error {
		return n.srv.Notification.NotifyTicketCreated(ctx, t)
	})
}

func (n ticketTopicNotifier) TicketReplied(ctx context.Context, ticketID int64, from, content string) {
	n.mirror(ctx, ticketID, "reply", func(ctx context.Context) error {
		return n.srv.Notification.NotifyTicketReplied(ctx, ticketID, from, content)
	})
}

func (n ticketTopicNotifier) TicketStatusChanged(ctx context.Context, ticketID int64, status uint8) {
	n.mirror(ctx, ticketID, "status", func(ctx context.Context) error {
		return n.srv.Notification.NotifyTicketStatusChanged(ctx, ticketID, status)
	})
}

// marketingQueue adapts the asynq client to the support module's
// MarketingQueue port, keeping queue task types out of the module.
type marketingQueue struct {
	client *taskqueue.Client
}

func (q marketingQueue) EnqueueBatchEmail(ctx context.Context, taskID int64, processAt time.Time) (string, error) {
	queueTaskID := fmt.Sprintf("marketing-email-%d-initial", taskID)
	t := asynq.NewTask(taskqueue.ScheduledBatchSendEmail, []byte(strconv.FormatInt(taskID, 10)))
	if err := q.enqueueIdempotent(ctx, t, queueTaskID, asynq.ProcessAt(processAt)); err != nil {
		return "", err
	}
	return queueTaskID, nil
}

func (q marketingQueue) EnqueueQuota(ctx context.Context, taskID int64) error {
	t := asynq.NewTask(taskqueue.ForthwithQuotaTask, []byte(strconv.FormatInt(taskID, 10)))
	return q.enqueueIdempotent(ctx, t, fmt.Sprintf("marketing-quota-%d", taskID))
}

// enqueueIdempotent retries once with the same task ID on a detached bounded
// context. This resolves the common "Redis accepted the write but the client
// lost the response" case as an ID conflict instead of falsely abandoning a
// durable database task.
func (q marketingQueue) enqueueIdempotent(ctx context.Context, t *asynq.Task, taskID string, opts ...asynq.Option) error {
	options := append(append([]asynq.Option{}, opts...), asynq.TaskID(taskID))
	_, err := q.client.EnqueueContext(ctx, t, options...)
	if err == nil || errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}

	retryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, retryErr := q.client.EnqueueContext(retryCtx, t, options...)
	if retryErr == nil || errors.Is(retryErr, asynq.ErrTaskIDConflict) {
		return nil
	}
	return errors.Join(err, retryErr)
}

// emailWorkerStopper adapts the global batch-email worker manager to the
// support module's BatchEmailStopper port.
type emailWorkerStopper struct{}

func (emailWorkerStopper) StopBatchEmail(taskID int64) {
	if email.Manager == nil {
		logger.Error("[StopBatchSendEmailTaskLogic] email worker manager is nil, cannot stop task")
		return
	}
	email.Manager.RemoveWorker(taskID)
}

// subscriptionReader adapts the legacy user-subscription repository to the
// support module's SubscriptionReader port.
type subscriptionReader struct {
	store repository.Store
}

func (r subscriptionReader) HasActiveSubscription(ctx context.Context, userID int64) (bool, error) {
	// status 1 = active
	subs, err := r.store.UserSubscription().QueryUserSubscribe(ctx, userID, 1)
	if err != nil {
		return false, err
	}
	return len(subs) > 0, nil
}
