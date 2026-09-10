package inventory

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type inventoryStore struct {
	repository.Store
	inbox      *fakeInbox
	subscribes *fakeSubscribeRepo
}

func (s *inventoryStore) InTx(_ context.Context, fn func(repository.Store) error) error {
	return fn(s)
}

func (s *inventoryStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	return fn(s)
}
func (s *inventoryStore) Inbox() repository.InboxRepo         { return s.inbox }
func (s *inventoryStore) Subscribe() repository.SubscribeRepo { return s.subscribes }

type fakeInbox struct {
	repository.InboxRepo
	records map[string]string
}

func (f *fakeInbox) Find(_ context.Context, consumer, key string) (*inbox.Record, error) {
	result, ok := f.records[consumer+"|"+key]
	if !ok {
		return nil, nil
	}
	return &inbox.Record{Consumer: consumer, EventKey: key, Result: result}, nil
}

func (f *fakeInbox) Insert(_ context.Context, consumer, key, result string) error {
	k := consumer + "|" + key
	if _, ok := f.records[k]; ok {
		return fmt.Errorf("duplicate inbox record %s", k)
	}
	f.records[k] = result
	return nil
}

type fakeSubscribeRepo struct {
	repository.SubscribeRepo
	stock    int64
	reserves int
	restores int
}

func (f *fakeSubscribeRepo) ReserveInventory(_ context.Context, _ int64, _ ...*gorm.DB) (bool, error) {
	if f.stock <= 0 {
		return false, nil
	}
	f.stock--
	f.reserves++
	return true, nil
}

func (f *fakeSubscribeRepo) RestoreInventory(_ context.Context, _ int64, _ ...*gorm.DB) error {
	f.stock++
	f.restores++
	return nil
}

func newInventoryStore(stock int64) *inventoryStore {
	return &inventoryStore{
		inbox:      &fakeInbox{records: map[string]string{}},
		subscribes: &fakeSubscribeRepo{stock: stock},
	}
}

func TestReserveInventoryOnceIsIdempotent(t *testing.T) {
	store := newInventoryStore(1)

	if err := New(store).Reserve(context.Background(), "order-1", 9); err != nil {
		t.Fatalf("first reserve: %v", err)
	}
	if err := New(store).Reserve(context.Background(), "order-1", 9); err != nil {
		t.Fatalf("replayed reserve: %v", err)
	}
	if store.subscribes.reserves != 1 {
		t.Fatalf("ReserveInventory calls = %d, want 1", store.subscribes.reserves)
	}
}

func TestReserveInventoryOnceReportsOutOfStock(t *testing.T) {
	store := newInventoryStore(0)

	err := New(store).Reserve(context.Background(), "order-2", 9)
	if !errors.Is(err, ErrOutOfStock) {
		t.Fatalf("expected ErrOutOfStock, got %v", err)
	}
	if _, ok := store.inbox.records[InventoryReserveConsumer+"|order-2"]; ok {
		t.Fatal("out-of-stock attempt must not leave a reserve marker")
	}
}

func TestRestoreInventoryOnceSkipsUnreservedOrders(t *testing.T) {
	store := newInventoryStore(1)

	if err := New(store).Restore(context.Background(), "never-reserved", 9); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if store.subscribes.restores != 0 {
		t.Fatal("orders that never reserved must not restore stock")
	}
}

func TestRestoreInventoryOnceRestoresExactlyOnce(t *testing.T) {
	store := newInventoryStore(1)

	if err := New(store).Reserve(context.Background(), "order-3", 9); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := New(store).Restore(context.Background(), "order-3", 9); err != nil {
		t.Fatalf("first restore: %v", err)
	}
	if err := New(store).Restore(context.Background(), "order-3", 9); err != nil {
		t.Fatalf("replayed restore: %v", err)
	}
	if store.subscribes.restores != 1 {
		t.Fatalf("RestoreInventory calls = %d, want 1", store.subscribes.restores)
	}
	if store.subscribes.stock != 1 {
		t.Fatalf("stock = %d, want 1 (reserve then restore)", store.subscribes.stock)
	}
}
