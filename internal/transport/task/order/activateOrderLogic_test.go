package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/billing"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	inboxEntity "github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription"
	subscribeEntity "github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

// newActivationDeps wires the real subscription/billing modules over the fake
// store so the saga tests exercise the same facade path production uses.
func newActivationDeps(store *activationStore, singleModel bool) Dependencies {
	subMod := subscription.New(subscription.Deps{
		Plans:       store.subscribes,
		UserSubs:    store.users,
		Cache:       store.users,
		Orders:      store.orders,
		Operations:  store,
		SingleModel: func() bool { return singleModel },
	})
	bilMod := billing.New(billing.Deps{
		PaidOrders:   billing.PaidOrderDependencies{Subscriptions: subMod},
		Orders:       store.orders,
		Store:        store,
		Tx:           store,
		UserProfiles: store.users,
		InvitePolicy: func() (uint8, bool) { return 0, false },
		SingleModel:  func() bool { return false },
		CurrencyUnit: func() string { return "CNY" },
	})
	return Dependencies{Store: store, Subscription: subMod, Billing: bilMod}
}

type activationStore struct {
	repository.Store
	wallet     *activationWalletRepo
	orders     *activationOrderRepo
	users      *activationUserRepo
	subscribes *activationSubscribeRepo
	logs       *activationLogRepo
	inbox      *activationInboxRepo
}

func (s *activationStore) InTx(_ context.Context, fn func(repository.Store) error) error {
	return fn(s)
}

func (s *activationStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s)
}

func (s *activationStore) InIdentityTx(_ context.Context, fn func(repository.IdentityStore) error) error {
	return fn(s)
}

func (s *activationStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	return fn(s)
}

func (s *activationStore) Wallet() repository.WalletRepo { return s.walletRepo() }
func (s *activationStore) Order() repository.OrderRepo   { return s.orders }
func (s *activationStore) User() repository.UserRepo     { return s.users }
func (s *activationStore) UserSubscription() repository.UserSubscriptionRepo {
	return s.users
}
func (s *activationStore) UserCache() repository.UserCacheRepo { return s.users }
func (s *activationStore) Log() repository.LogRepo             { return s.logs }
func (s *activationStore) Subscribe() repository.SubscribeRepo { return s.subscribes }
func (s *activationStore) Inbox() repository.InboxRepo         { return s.inbox }

type activationInboxRepo struct {
	repository.InboxRepo
	records map[string]*inboxEntity.Record
}

func newActivationInboxRepo() *activationInboxRepo {
	return &activationInboxRepo{records: map[string]*inboxEntity.Record{}}
}

func (r *activationInboxRepo) Find(_ context.Context, consumer, eventKey string) (*inboxEntity.Record, error) {
	record, ok := r.records[consumer+"|"+eventKey]
	if !ok {
		return nil, nil
	}
	copy := *record
	return &copy, nil
}

func (r *activationInboxRepo) Insert(_ context.Context, consumer, eventKey, result string) error {
	key := consumer + "|" + eventKey
	if _, ok := r.records[key]; ok {
		return fmt.Errorf("duplicate inbox record %s", key)
	}
	r.records[key] = &inboxEntity.Record{Consumer: consumer, EventKey: eventKey, Result: result}
	return nil
}

type activationOrderRepo struct {
	repository.OrderRepo
	order            *orderEntity.Order
	finalizeFailures int
}

func (r *activationOrderRepo) FindOneByOrderNo(_ context.Context, orderNo string) (*orderEntity.Order, error) {
	if r.order.OrderNo != orderNo {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.order
	return &copy, nil
}

func (r *activationOrderRepo) FindOneByOrderNoForUpdate(ctx context.Context, orderNo string) (*orderEntity.Order, error) {
	return r.FindOneByOrderNo(ctx, orderNo)
}

func (r *activationOrderRepo) UpdateOrderStatusFrom(_ context.Context, orderNo string, from, to uint8, _ ...*gorm.DB) (bool, error) {
	if to == OrderStatusFinished && r.finalizeFailures > 0 {
		r.finalizeFailures--
		return false, errors.New("finalize write unavailable")
	}
	if r.order.OrderNo != orderNo || r.order.Status != from {
		return false, nil
	}
	r.order.Status = to
	return true, nil
}

type activationWalletRepo struct {
	repository.WalletRepo
	wallet *walletEntity.Wallet
}

func (r *activationWalletRepo) FindWallet(_ context.Context, userId int64) (*walletEntity.Wallet, error) {
	if r.wallet == nil || r.wallet.UserId != userId {
		return nil, nil
	}
	copy := *r.wallet
	return &copy, nil
}

func (r *activationWalletRepo) FindOneForUpdate(_ context.Context, id int64) (*walletEntity.Wallet, error) {
	if r.wallet == nil {
		r.wallet = &walletEntity.Wallet{UserId: id}
	}
	if r.wallet.UserId != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.wallet
	return &copy, nil
}

func (r *activationWalletRepo) UpdateBalanceFields(_ context.Context, data *walletEntity.Wallet, _ ...*gorm.DB) error {
	r.wallet.Balance = data.Balance
	r.wallet.GiftAmount = data.GiftAmount
	return nil
}

func (r *activationWalletRepo) UpdateCommission(_ context.Context, data *walletEntity.Wallet, _ ...*gorm.DB) error {
	r.wallet.Commission = data.Commission
	return nil
}

type activationUserRepo struct {
	repository.UserRepo
	repository.UserSubscriptionRepo
	repository.UserCacheRepo
	user             *userEntity.User
	profiles         map[int64]*userEntity.User
	updateCacheCalls int
	quotaCount       int64
	quotaCountCalls  int
	blocking         bool
	hasBlockingCalls int
	subscription     *usersub.Subscribe
}

func (r *activationUserRepo) FindOne(_ context.Context, id int64) (*userEntity.User, error) {
	if profile := r.profiles[id]; profile != nil {
		copy := *profile
		return &copy, nil
	}
	if r.user == nil || r.user.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.user
	return &copy, nil
}

func TestCommissionIsNotCreditedAgainAfterFinalizeFailure(t *testing.T) {
	expire := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	store := &activationStore{
		orders: &activationOrderRepo{finalizeFailures: 1, order: &orderEntity.Order{
			OrderNo: "commission-retry", UserId: 7, Type: OrderTypeRenewal, Status: OrderStatusPaid,
			SubscribeId: 9, SubscribeToken: "renewal-token", Quantity: 1, Amount: 10000, FeeAmount: 100,
		}},
		wallet: &activationWalletRepo{wallet: &walletEntity.Wallet{UserId: 99}},
		users: &activationUserRepo{
			user:         &userEntity.User{Id: 7, RefererId: 99},
			profiles:     map[int64]*userEntity.User{99: {Id: 99, ReferralPercentage: 20}},
			subscription: &usersub.Subscribe{Id: 11, UserId: 7, SubscribeId: 9, Token: "renewal-token", ExpireTime: expire, Status: usersub.SubscribeStatusActive},
		},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9, UnitTime: "Month"}},
		logs:       &activationLogRepo{}, inbox: newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(newActivationDeps(store, false).Billing)
	payload, err := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: "commission-retry"})
	if err != nil {
		t.Fatal(err)
	}
	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, payload)
	if err := logic.ProcessTask(context.Background(), task); err == nil {
		t.Fatal("expected finalize failure")
	}
	if store.wallet.wallet.Commission != 1980 || store.orders.order.Status != OrderStatusPaid {
		t.Fatal("commission must commit before a retryable finalize failure")
	}
	extendedOnce := store.users.subscription.ExpireTime
	for range 2 {
		if err := logic.ProcessTask(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	if store.wallet.wallet.Commission != 1980 || !store.users.subscription.ExpireTime.Equal(extendedOnce) || store.orders.order.Status != OrderStatusFinished {
		t.Fatal("replay duplicated commission or subscription fulfillment")
	}
	commissionLogs := 0
	for _, entry := range store.logs.logs {
		if entry.Type == logEntity.TypeCommission.Uint8() {
			commissionLogs++
		}
	}
	if commissionLogs != 1 {
		t.Fatalf("commission logs = %d, want 1", commissionLogs)
	}
}

func (r *activationUserRepo) FindOneForUpdate(_ context.Context, id int64) (*userEntity.User, error) {
	return r.FindOne(context.Background(), id)
}

func (r *activationUserRepo) Update(_ context.Context, _ *userEntity.User, _ ...*gorm.DB) error {
	return nil
}

func (r *activationUserRepo) UpdateUserCache(_ context.Context, _ *userEntity.User) error {
	r.updateCacheCalls++
	return nil
}

func (r *activationUserRepo) LockUserSerial(_ context.Context, _ int64) error {
	return nil
}

func (r *activationUserRepo) CountQuotaConsumingSubscriptions(_ context.Context, _ int64, _ int64) (int64, error) {
	r.quotaCountCalls++
	return r.quotaCount, nil
}

func (r *activationUserRepo) HasBlockingSubscription(_ context.Context, _ int64) (bool, error) {
	r.hasBlockingCalls++
	return r.blocking, nil
}

func (r *activationUserRepo) FindOneSubscribeByToken(_ context.Context, token string) (*usersub.Subscribe, error) {
	if r.subscription == nil || r.subscription.Token != token {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.subscription
	return &copy, nil
}

func (r *activationUserRepo) FindOneSubscribeByTokenForUpdate(ctx context.Context, token string) (*usersub.Subscribe, error) {
	return r.FindOneSubscribeByToken(ctx, token)
}

func (r *activationUserRepo) UpdateSubscribe(_ context.Context, data *usersub.Subscribe, _ ...*gorm.DB) error {
	copy := *data
	r.subscription = &copy
	return nil
}

func (r *activationUserRepo) ClearSubscribeCache(_ context.Context, _ ...*usersub.Subscribe) error {
	return nil
}

type activationLogRepo struct {
	repository.LogRepo
	logs []*logEntity.SystemLog
}

func (r *activationLogRepo) Insert(_ context.Context, data *logEntity.SystemLog) error {
	r.logs = append(r.logs, data)
	return nil
}

type activationSubscribeRepo struct {
	repository.SubscribeRepo
	subscribe *subscribeEntity.Subscribe
}

func (r *activationSubscribeRepo) FindOne(_ context.Context, id int64) (*subscribeEntity.Subscribe, error) {
	if r.subscribe == nil || r.subscribe.Id != id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.subscribe
	return &copy, nil
}

func (r *activationSubscribeRepo) ClearCache(_ context.Context, _ ...int64) error {
	return nil
}

func TestActivateRechargeCommitsSettlementOnlyOnce(t *testing.T) {
	store := &activationStore{
		orders: &activationOrderRepo{order: &orderEntity.Order{
			OrderNo: "recharge-order", UserId: 7, Type: OrderTypeRecharge, Price: 1250, Status: OrderStatusPaid,
		}},
		users:  &activationUserRepo{user: &userEntity.User{Id: 7}},
		wallet: &activationWalletRepo{wallet: &walletEntity.Wallet{UserId: 7, Balance: 500}},
		logs:   &activationLogRepo{},
		inbox:  newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(newActivationDeps(store, false).Billing)
	payload, err := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: "recharge-order"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, payload)

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("duplicate activation: %v", err)
	}
	if store.orders.order.Status != OrderStatusFinished {
		t.Fatalf("order status = %d, want finished", store.orders.order.Status)
	}
	if store.wallet.wallet.Balance != 1750 {
		t.Fatalf("balance = %d, want 1750", store.wallet.wallet.Balance)
	}
	if len(store.logs.logs) != 1 {
		t.Fatalf("recharge logs = %d, want 1", len(store.logs.logs))
	}
}

// TestActivateRechargeReplayAfterFulfillmentSkipsSecondCredit simulates a
// crash between the fulfillment and finalize stages: the balance credit
// committed but the order is still Paid, so the reconciler replays the task.
// The inbox marker must prevent a second credit.
func TestActivateRechargeReplayAfterFulfillmentSkipsSecondCredit(t *testing.T) {
	store := &activationStore{
		orders: &activationOrderRepo{order: &orderEntity.Order{
			OrderNo: "recharge-replay", UserId: 7, Type: OrderTypeRecharge, Price: 1250, Status: OrderStatusPaid,
		}},
		users:  &activationUserRepo{user: &userEntity.User{Id: 7}},
		wallet: &activationWalletRepo{wallet: &walletEntity.Wallet{UserId: 7, Balance: 500}},
		logs:   &activationLogRepo{},
		inbox:  newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(newActivationDeps(store, false).Billing)
	payload, err := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: "recharge-replay"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, payload)

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	// Simulate the finalize stage having been lost: the order is Paid again.
	store.orders.order.Status = OrderStatusPaid

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("replayed activation: %v", err)
	}
	if store.wallet.wallet.Balance != 1750 {
		t.Fatalf("balance = %d, want 1750 (credited exactly once)", store.wallet.wallet.Balance)
	}
	if len(store.logs.logs) != 1 {
		t.Fatalf("balance logs = %d, want 1", len(store.logs.logs))
	}
	if store.orders.order.Status != OrderStatusFinished {
		t.Fatalf("order status = %d, want finished after replay", store.orders.order.Status)
	}
}

// TestActivateRenewalReplayExtendsSubscriptionOnce guards the most dangerous
// replay: extending a renewal twice would silently gift subscription time.
func TestActivateRenewalReplayExtendsSubscriptionOnce(t *testing.T) {
	expire := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	store := &activationStore{
		orders: &activationOrderRepo{order: &orderEntity.Order{
			OrderNo: "renewal-replay", UserId: 7, Type: OrderTypeRenewal, Status: OrderStatusPaid,
			SubscribeId: 9, SubscribeToken: "renewal-token", Quantity: 1,
		}},
		users: &activationUserRepo{
			user: &userEntity.User{Id: 7},
			subscription: &usersub.Subscribe{
				Id: 11, UserId: 7, SubscribeId: 9, Token: "renewal-token",
				ExpireTime: expire, Status: usersub.SubscribeStatusActive,
			},
		},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9, UnitTime: "Month"}},
		logs:       &activationLogRepo{},
		inbox:      newActivationInboxRepo(),
	}
	logic := NewActivateOrderLogic(newActivationDeps(store, false).Billing)
	payload, err := json.Marshal(taskqueue.ForthwithActivateOrderPayload{OrderNo: "renewal-replay"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, payload)

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("first activation: %v", err)
	}
	extendedOnce := store.users.subscription.ExpireTime
	if !extendedOnce.After(expire) {
		t.Fatalf("first activation must extend the subscription: %v -> %v", expire, extendedOnce)
	}

	// Simulate the finalize stage having been lost: the order is Paid again.
	store.orders.order.Status = OrderStatusPaid

	if err := logic.ProcessTask(context.Background(), task); err != nil {
		t.Fatalf("replayed activation: %v", err)
	}
	if !store.users.subscription.ExpireTime.Equal(extendedOnce) {
		t.Fatalf("replay extended the subscription twice: %v -> %v", extendedOnce, store.users.subscription.ExpireTime)
	}
	if store.orders.order.Status != OrderStatusFinished {
		t.Fatalf("order status = %d, want finished after replay", store.orders.order.Status)
	}
}

func TestFulfillmentEnforcesQuota(t *testing.T) {
	users := &activationUserRepo{quotaCount: 1, user: &userEntity.User{Id: 7}}
	store := &activationStore{
		users:      users,
		orders:     &activationOrderRepo{order: &orderEntity.Order{OrderNo: "quota-order", UserId: 7, SubscribeId: 9, Type: OrderTypeSubscribe, Status: OrderStatusPaid}},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9, Quota: 1}},
		inbox:      newActivationInboxRepo(),
	}
	svcCtx := newActivationDeps(store, false)

	_, err := svcCtx.Subscription.FulfillPaidOrder(context.Background(), "quota-order")
	if err == nil {
		t.Fatal("activation created a subscription after quota was exhausted")
	}
	if users.quotaCountCalls != 1 {
		t.Fatalf("CountQuotaConsumingSubscriptions calls = %d, want 1", users.quotaCountCalls)
	}
}

func TestFulfillmentEnforcesSingleModel(t *testing.T) {
	users := &activationUserRepo{blocking: true, user: &userEntity.User{Id: 7}}
	store := &activationStore{
		users:      users,
		orders:     &activationOrderRepo{order: &orderEntity.Order{OrderNo: "single-order", UserId: 7, SubscribeId: 9, Type: OrderTypeSubscribe, Status: OrderStatusPaid}},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9}},
		inbox:      newActivationInboxRepo(),
	}
	svcCtx := newActivationDeps(store, true)

	_, err := svcCtx.Subscription.FulfillPaidOrder(context.Background(), "single-order")
	if err == nil {
		t.Fatal("activation created a subscription despite a blocking subscription")
	}
	if users.hasBlockingCalls != 1 {
		t.Fatalf("HasBlockingSubscription calls = %d, want 1", users.hasBlockingCalls)
	}
}

func TestFulfillResetTrafficClearsFinishedAt(t *testing.T) {
	finishedAt := time.Now().Add(-time.Hour)
	store := &activationStore{
		users: &activationUserRepo{
			user: &userEntity.User{Id: 7},
			subscription: &usersub.Subscribe{
				Id: 11, UserId: 7, SubscribeId: 9, Token: "subscription-token",
				Download: 100, Upload: 200, Status: usersub.SubscribeStatusFinished, FinishedAt: &finishedAt,
			},
		},
		orders:     &activationOrderRepo{order: &orderEntity.Order{OrderNo: "reset-order", UserId: 7, SubscribeToken: "subscription-token", Type: OrderTypeResetTraffic, Status: OrderStatusPaid}},
		subscribes: &activationSubscribeRepo{subscribe: &subscribeEntity.Subscribe{Id: 9}},
		logs:       &activationLogRepo{},
		inbox:      newActivationInboxRepo(),
	}
	svcCtx := newActivationDeps(store, false)

	if _, err := svcCtx.Subscription.FulfillPaidOrder(context.Background(), "reset-order"); err != nil {
		t.Fatalf("activate reset traffic: %v", err)
	}
	if store.users.subscription.FinishedAt != nil {
		t.Fatal("reset traffic left FinishedAt set")
	}
	if store.users.subscription.Status != usersub.SubscribeStatusActive {
		t.Fatalf("status = %d, want active", store.users.subscription.Status)
	}
}

func (s *activationStore) walletRepo() *activationWalletRepo {
	if s.wallet == nil {
		s.wallet = &activationWalletRepo{}
	}
	return s.wallet
}
