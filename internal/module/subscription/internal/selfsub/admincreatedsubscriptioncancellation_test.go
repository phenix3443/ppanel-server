package selfsub

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	usermodel "github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"gorm.io/gorm"
)

type adminCreatedSubscriptionUserRepo struct {
	repository.UserRepo
	repository.UserSubscriptionRepo
	repository.UserCacheRepo

	subscribe                 *usersub.Subscribe
	findOneSubscribeCalls     int
	findOneUserSubscribeCalls int
	updateSubscribeCalls      int
	clearSubscribeCacheCalls  int
}

func (r *adminCreatedSubscriptionUserRepo) FindOneSubscribe(_ context.Context, _ int64) (*usersub.Subscribe, error) {
	r.findOneSubscribeCalls++
	return r.subscribe, nil
}

func (r *adminCreatedSubscriptionUserRepo) FindOneSubscribeForUpdate(_ context.Context, _ int64) (*usersub.Subscribe, error) {
	return r.subscribe, nil
}

func (r *adminCreatedSubscriptionUserRepo) FindOneUserSubscribe(_ context.Context, _ int64) (*usersub.SubscribeDetails, error) {
	r.findOneUserSubscribeCalls++
	return &usersub.SubscribeDetails{OrderId: r.subscribe.OrderId}, nil
}

func (r *adminCreatedSubscriptionUserRepo) UpdateSubscribe(_ context.Context, subscribe *usersub.Subscribe, _ ...*gorm.DB) error {
	r.updateSubscribeCalls++
	r.subscribe = subscribe
	return nil
}

func (r *adminCreatedSubscriptionUserRepo) ClearSubscribeCache(_ context.Context, _ ...*usersub.Subscribe) error {
	r.clearSubscribeCacheCalls++
	return nil
}

type adminCreatedSubscriptionSubscribeRepo struct {
	repository.SubscribeRepo
	clearCacheCalls int
}

func (r *adminCreatedSubscriptionSubscribeRepo) ClearCache(_ context.Context, _ ...int64) error {
	r.clearCacheCalls++
	return nil
}

type adminCreatedSubscriptionStore struct {
	repository.Store
	wallet *adminCreatedWalletRepo

	userRepo      *adminCreatedSubscriptionUserRepo
	subscribeRepo repository.SubscribeRepo
	inbox         *fakeInboxRepo
	inTxCalls     int
	orderCalls    int
}

func (s *adminCreatedSubscriptionStore) Inbox() repository.InboxRepo {
	if s.inbox == nil {
		s.inbox = newFakeInboxRepo()
	}
	return s.inbox
}

func (s *adminCreatedSubscriptionStore) User() repository.UserRepo {
	return s.userRepo
}

func (s *adminCreatedSubscriptionStore) UserSubscription() repository.UserSubscriptionRepo {
	return s.userRepo
}

func (s *adminCreatedSubscriptionStore) UserCache() repository.UserCacheRepo {
	return s.userRepo
}

func (s *adminCreatedSubscriptionStore) Subscribe() repository.SubscribeRepo {
	return s.subscribeRepo
}

func (s *adminCreatedSubscriptionStore) Order() repository.OrderRepo {
	s.orderCalls++
	panic("admin-created subscription cancellation must not query an order")
}

func (s *adminCreatedSubscriptionStore) InTx(ctx context.Context, fn func(repository.Store) error) error {
	s.inTxCalls++
	return fn(s)
}

func (s *adminCreatedSubscriptionStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	s.inTxCalls++
	return fn(s)
}

func (s *adminCreatedSubscriptionStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	s.inTxCalls++
	return fn(s)
}

// adminCreatedWalletRepo fails the test if the skip-refund path touches the
// wallet at all.
type adminCreatedWalletRepo struct {
	repository.WalletRepo
	calls int
}

func (r *adminCreatedWalletRepo) FindOneForUpdate(_ context.Context, _ int64) (*walletEntity.Wallet, error) {
	r.calls++
	panic("admin-created subscription cancellation must not move money")
}

func (s *adminCreatedSubscriptionStore) Wallet() repository.WalletRepo { return s.wallet }

func TestUnsubscribe_AdminCreatedSubscription_SkipsRefund(t *testing.T) {
	logtest.Discard(t)

	const (
		userID      int64 = 100
		subscribeID int64 = 200
		planID      int64 = 300
	)

	currentUser := &usermodel.User{Id: userID}
	userSubscribe := &usersub.Subscribe{
		Id:          subscribeID,
		UserId:      userID,
		OrderId:     0,
		SubscribeId: planID,
		Status:      1,
	}
	userRepo := &adminCreatedSubscriptionUserRepo{subscribe: userSubscribe}
	subscribeRepo := &adminCreatedSubscriptionSubscribeRepo{}
	store := &adminCreatedSubscriptionStore{
		userRepo:      userRepo,
		subscribeRepo: subscribeRepo,
		wallet:        &adminCreatedWalletRepo{},
	}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, currentUser)
	logic := newUnsubscribeLogic(ctx, Deps{
		UserSubs: store.userRepo,
		Users:    store.userRepo,
		Cache:    store.userRepo,
		Plans:    store.subscribeRepo,
		Inbox:    store.Inbox(),
		Store:    store,
	})

	err := logic.Unsubscribe(&dto.UnsubscribeRequest{Id: subscribeID})

	if err != nil {
		t.Fatalf("Unsubscribe() error = %v, want nil", err)
	}
	if userRepo.subscribe.Status != 4 {
		t.Fatalf("subscription status = %d, want 4 (cancelled)", userRepo.subscribe.Status)
	}
	if store.wallet.calls != 0 {
		t.Fatalf("wallet touched %d time(s), want 0", store.wallet.calls)
	}
	// One subscription-domain cancellation transaction plus one billing
	// transaction that only records the settled-refund marker.
	if store.inTxCalls != 2 {
		t.Fatalf("InTx called %d time(s), want 2", store.inTxCalls)
	}
	if store.orderCalls != 0 {
		t.Fatalf("Order called %d time(s), want 0", store.orderCalls)
	}
	if userRepo.findOneSubscribeCalls != 1 {
		t.Fatalf("FindOneSubscribe called %d time(s), want 1", userRepo.findOneSubscribeCalls)
	}
	if userRepo.findOneUserSubscribeCalls != 1 {
		t.Fatalf("FindOneUserSubscribe called %d time(s), want 1", userRepo.findOneUserSubscribeCalls)
	}
	if userRepo.updateSubscribeCalls != 1 {
		t.Fatalf("UpdateSubscribe called %d time(s), want 1", userRepo.updateSubscribeCalls)
	}
	if userRepo.clearSubscribeCacheCalls != 1 {
		t.Fatalf("ClearSubscribeCache called %d time(s), want 1", userRepo.clearSubscribeCacheCalls)
	}
	if subscribeRepo.clearCacheCalls != 1 {
		t.Fatalf("Subscribe.ClearCache called %d time(s), want 1", subscribeRepo.clearCacheCalls)
	}
}
