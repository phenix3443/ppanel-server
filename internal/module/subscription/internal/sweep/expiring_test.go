package sweep

import (
	"context"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type expiringReminder struct {
	userID        int64
	planName      string
	expireAt      time.Time
	renewalAmount int64
}

type recordingNotifier struct {
	Notifier
	reminders []expiringReminder
}

func (n *recordingNotifier) NotifySubscriptionExpiring(_ context.Context, userID int64, planName string, expireAt time.Time, renewalAmount int64) {
	n.reminders = append(n.reminders, expiringReminder{
		userID: userID, planName: planName, expireAt: expireAt, renewalAmount: renewalAmount,
	})
}

type expiringSubsRepo struct {
	repository.UserSubscriptionRepo
	subs []*usersub.Subscribe
	from time.Time
	to   time.Time
}

func (r *expiringSubsRepo) FindExpiringSubscribes(_ context.Context, from, to time.Time) ([]*usersub.Subscribe, error) {
	r.from, r.to = from, to
	return r.subs, nil
}

type expiringPlansRepo struct {
	repository.SubscribeRepo
	plans map[int64]*subscribe.Subscribe
}

func (r *expiringPlansRepo) FindOne(_ context.Context, id int64) (*subscribe.Subscribe, error) {
	plan, ok := r.plans[id]
	if !ok {
		return nil, context.Canceled
	}
	return plan, nil
}

type expiringInbox struct {
	repository.InboxRepo
	records map[string]bool
}

func (r *expiringInbox) Find(_ context.Context, consumer, eventKey string) (*inbox.Record, error) {
	if r.records[consumer+"|"+eventKey] {
		return &inbox.Record{Consumer: consumer, EventKey: eventKey}, nil
	}
	return nil, nil
}

func (r *expiringInbox) Insert(_ context.Context, consumer, eventKey, _ string) error {
	r.records[consumer+"|"+eventKey] = true
	return nil
}

type expiringStore struct {
	repository.Store
	inbox *expiringInbox
}

func (s *expiringStore) Inbox() repository.InboxRepo { return s.inbox }

func newExpiringService(subs []*usersub.Subscribe) (*Service, *recordingNotifier, *expiringSubsRepo) {
	notifier := &recordingNotifier{}
	userSubs := &expiringSubsRepo{subs: subs}
	svc := NewService(Deps{
		UserSubs: userSubs,
		Plans:    &expiringPlansRepo{plans: map[int64]*subscribe.Subscribe{9: {Id: 9, Name: "Pro 月付", UnitPrice: 1890}}},
		Store:    &expiringStore{inbox: &expiringInbox{records: map[string]bool{}}},
		Notify:   notifier,
	})
	return svc, notifier, userSubs
}

// A subscription sits inside the reminder window for days, so the notice must
// be announced once per expiry rather than on every daily pass.
func TestRemindExpiringSubscribesAnnouncesOncePerExpiry(t *testing.T) {
	expireAt := timeutil.Now().Add(48 * time.Hour)
	svc, notifier, userSubs := newExpiringService([]*usersub.Subscribe{
		{Id: 1, UserId: 7, SubscribeId: 9, ExpireTime: expireAt},
	})

	if err := svc.RemindExpiringSubscribes(context.Background()); err != nil {
		t.Fatalf("RemindExpiringSubscribes error = %v", err)
	}
	if err := svc.RemindExpiringSubscribes(context.Background()); err != nil {
		t.Fatalf("second pass error = %v", err)
	}

	if len(notifier.reminders) != 1 {
		t.Fatalf("reminders = %d, want 1", len(notifier.reminders))
	}
	got := notifier.reminders[0]
	if got.userID != 7 || got.planName != "Pro 月付" || got.renewalAmount != 1890 {
		t.Fatalf("reminder = %+v, want the owner, plan name and renewal price", got)
	}
	if !got.expireAt.Equal(expireAt) {
		t.Fatalf("expireAt = %v, want %v", got.expireAt, expireAt)
	}
	// The window starts now and reaches the reminder horizon.
	if window := userSubs.to.Sub(userSubs.from); window != expiryReminderWindow {
		t.Fatalf("query window = %v, want %v", window, expiryReminderWindow)
	}
}

// A renewal moves the expiry, which makes the subscription eligible again —
// the marker is keyed by the expiry it announced.
func TestRemindExpiringSubscribesAnnouncesAgainAfterRenewal(t *testing.T) {
	first := timeutil.Now().Add(24 * time.Hour)
	sub := &usersub.Subscribe{Id: 1, UserId: 7, SubscribeId: 9, ExpireTime: first}
	svc, notifier, _ := newExpiringService([]*usersub.Subscribe{sub})

	if err := svc.RemindExpiringSubscribes(context.Background()); err != nil {
		t.Fatalf("RemindExpiringSubscribes error = %v", err)
	}
	sub.ExpireTime = first.AddDate(0, 1, 0)
	if err := svc.RemindExpiringSubscribes(context.Background()); err != nil {
		t.Fatalf("second pass error = %v", err)
	}

	if len(notifier.reminders) != 2 {
		t.Fatalf("reminders = %d, want 2 (one per expiry)", len(notifier.reminders))
	}
}

// An unreadable plan must still produce a notice: the owner needs the warning
// more than the plan's name.
func TestRemindExpiringSubscribesToleratesMissingPlan(t *testing.T) {
	svc, notifier, _ := newExpiringService([]*usersub.Subscribe{
		{Id: 2, UserId: 8, SubscribeId: 404, ExpireTime: timeutil.Now().Add(time.Hour)},
	})

	if err := svc.RemindExpiringSubscribes(context.Background()); err != nil {
		t.Fatalf("RemindExpiringSubscribes error = %v", err)
	}

	if len(notifier.reminders) != 1 {
		t.Fatalf("reminders = %d, want 1", len(notifier.reminders))
	}
	if got := notifier.reminders[0]; got.planName != "" || got.renewalAmount != 0 {
		t.Fatalf("reminder = %+v, want an empty plan summary", got)
	}
}
