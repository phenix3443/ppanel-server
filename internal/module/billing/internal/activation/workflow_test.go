package activation

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var errGuestOrderBind = errors.New("order binding failed")

type workflowOrders struct {
	repository.OrderRepo
	bindFails bool
	boundUser int64
}

func (r *workflowOrders) Update(_ context.Context, o *order.Order, _ ...*gorm.DB) error {
	if r.bindFails {
		return errGuestOrderBind
	}
	r.boundUser = o.UserId
	return nil
}

type workflowGuests struct {
	id      int64
	creates int
	command identity.GuestAccountCommand
}

func (g *workflowGuests) FindGuestAccount(context.Context, string) (int64, bool, error) {
	return g.id, g.id != 0, nil
}
func (g *workflowGuests) EnsureGuestAccount(_ context.Context, command identity.GuestAccountCommand) (int64, error) {
	g.creates++
	g.id = 11
	g.command = command
	return g.id, nil
}

func TestGuestBindingRetryWorksAfterLegacyCacheExpires(t *testing.T) {
	redisServer := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()
	legacy := order.TemporaryOrderInfo{OrderNo: "legacy-order", AuthType: "email", Identifier: "guest@example.test", PasswordHash: "existing-hash", InviteCode: "referral"}
	payload, err := legacy.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	key := fmt.Sprintf(order.TempOrderCacheKey, legacy.OrderNo)
	if err := client.Set(ctx, key, payload, 0).Err(); err != nil {
		t.Fatal(err)
	}
	orders := &workflowOrders{bindFails: true}
	guests := &workflowGuests{}
	workflow := NewWorkflow(WorkflowDeps{Orders: orders, GuestAccounts: guests, LegacyGuestCache: client}, nil)
	if err := workflow.ensureGuestAccount(ctx, &order.Order{OrderNo: legacy.OrderNo}); !errors.Is(err, errGuestOrderBind) {
		t.Fatalf("expected failed billing bind, got %v", err)
	}
	if guests.creates != 1 || guests.command.PasswordHash != legacy.PasswordHash || guests.command.InviteCode != legacy.InviteCode {
		t.Fatal("identity did not receive the historical checkout data")
	}
	if err := client.Del(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	orders.bindFails = false
	if err := workflow.ensureGuestAccount(ctx, &order.Order{OrderNo: legacy.OrderNo}); err != nil {
		t.Fatalf("committed identity should be reusable without Redis checkout data: %v", err)
	}
	if orders.boundUser != 11 || guests.creates != 1 {
		t.Fatal("retry created a second identity or bound the wrong user")
	}
}

func TestDurableGuestSnapshotDoesNotRequireRedis(t *testing.T) {
	orders := &workflowOrders{}
	guests := &workflowGuests{}
	workflow := NewWorkflow(WorkflowDeps{Orders: orders, GuestAccounts: guests}, nil)
	err := workflow.ensureGuestAccount(context.Background(), &order.Order{
		OrderNo: "durable-order", GuestAuthType: "email", GuestIdentifier: "guest@example.test", GuestPasswordHash: "durable-hash", GuestInviteCode: "referral",
	})
	if err != nil {
		t.Fatal(err)
	}
	if orders.boundUser != 11 || guests.command.PasswordHash != "durable-hash" || guests.command.LegacyPassword != "" {
		t.Fatal("durable checkout data was not used")
	}
}
