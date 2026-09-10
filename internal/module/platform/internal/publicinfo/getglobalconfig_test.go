package publicinfo

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/platform/entity/system"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
)

type globalConfigTestStore struct {
	repository.Store
	system repository.SystemRepo
	auth   repository.AuthRepo
}

func (s globalConfigTestStore) System() repository.SystemRepo { return s.system }
func (s globalConfigTestStore) Auth() repository.AuthRepo     { return s.auth }

type failingGlobalConfigSystemRepo struct {
	repository.SystemRepo
	err   error
	calls int
}

func (r *failingGlobalConfigSystemRepo) GetCurrencyConfig(context.Context) ([]*system.System, error) {
	r.calls++
	return nil, r.err
}

func TestGetGlobalConfigUsesInjectedStore(t *testing.T) {
	logtest.Discard(t)
	failure := errors.New("currency config unavailable")
	systems := &failingGlobalConfigSystemRepo{err: failure}
	logic := NewGetGlobalConfigLogic(context.Background(), GetGlobalConfigDependencies{
		Store: globalConfigTestStore{system: systems},
	})

	_, err := logic.GetGlobalConfig()
	if err == nil {
		t.Fatal("GetGlobalConfig error = nil, want currency config error")
	}
	if systems.calls != 1 {
		t.Fatalf("GetCurrencyConfig calls = %d, want 1", systems.calls)
	}
}

type publicConfigSystemRepo struct {
	repository.SystemRepo
}

func (publicConfigSystemRepo) GetCurrencyConfig(context.Context) ([]*system.System, error) {
	return nil, nil
}

func (publicConfigSystemRepo) GetVerifyCodeConfig(context.Context) ([]*system.System, error) {
	return nil, nil
}

func (publicConfigSystemRepo) FindOneByKey(context.Context, string) (*system.System, error) {
	return &system.System{Value: "false"}, nil
}

type publicConfigAuthRepo struct {
	repository.AuthRepo
}

func (publicConfigAuthRepo) FindAll(context.Context) ([]*auth.Auth, error) {
	return nil, nil
}

func TestGetGlobalConfigPreservesSubscribePath(t *testing.T) {
	for _, path := range []string{"", "/subscribe", "/custom/subscription", "/sub"} {
		t.Run(path, func(t *testing.T) {
			logic := NewGetGlobalConfigLogic(context.Background(), GetGlobalConfigDependencies{
				Store: globalConfigTestStore{system: publicConfigSystemRepo{}, auth: publicConfigAuthRepo{}},
				Config: GlobalConfigSnapshot{
					Subscribe: config.SubscribeConfig{SubscribePath: path},
				},
			})
			resp, err := logic.GetGlobalConfig()
			if err != nil {
				t.Fatal(err)
			}
			if got := resp.Subscribe.SubscribePath; got != path {
				t.Fatalf("SubscribePath = %q, want %q", got, path)
			}
		})
	}
}
