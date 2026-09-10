package registerpolicy

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestTakeIPPermit(t *testing.T) {
	server := miniredis.RunT(t)
	policy := New(Deps{
		Redis: redis.NewClient(&redis.Options{Addr: server.Addr()}),
		Config: func() Snapshot {
			return Snapshot{
				EnableIpRegisterLimit:   true,
				IpRegisterLimit:         2,
				IpRegisterLimitDuration: 10,
			}
		},
	})

	for i := 0; i < 2; i++ {
		if err := policy.TakeIPPermit(context.Background(), "192.0.2.8"); err != nil {
			t.Fatalf("permit %d: %v", i+1, err)
		}
	}
	if err := policy.TakeIPPermit(context.Background(), "192.0.2.8"); err == nil {
		t.Fatal("expected third registration to exceed quota")
	}
	if err := policy.TakeIPPermit(context.Background(), "192.0.2.9"); err != nil {
		t.Fatalf("different IP should have its own quota: %v", err)
	}
}

func TestEnsureRegistrationOpenForEmail(t *testing.T) {
	stopped := false
	policy := New(Deps{Config: func() Snapshot {
		return Snapshot{EmailEnabled: true, StopRegister: stopped}
	}})
	if err := policy.EnsureRegistrationOpen(context.Background(), MethodEmail); err != nil {
		t.Fatalf("enabled registration rejected: %v", err)
	}
	stopped = true
	if err := policy.EnsureRegistrationOpen(context.Background(), MethodEmail); err == nil {
		t.Fatal("stopped registration was accepted")
	}
}
