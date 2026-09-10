package usersub

import (
	"context"
	"strings"
	"testing"

	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/repository"
)

type adminSubscriptionPolicyStore struct {
	repository.Store
	users *adminSubscriptionPolicyUserRepo
}

func (s adminSubscriptionPolicyStore) User() repository.UserRepo { return s.users }
func (s adminSubscriptionPolicyStore) UserSubscription() repository.UserSubscriptionRepo {
	return s.users
}

type adminSubscriptionPolicyUserRepo struct {
	repository.UserRepo
	repository.UserSubscriptionRepo
	blocking         bool
	hasBlockingCalls int
}

func (r *adminSubscriptionPolicyUserRepo) FindOne(_ context.Context, _ int64) (*userEntity.User, error) {
	return &userEntity.User{Id: 7}, nil
}

func (r *adminSubscriptionPolicyUserRepo) HasBlockingSubscription(_ context.Context, _ int64) (bool, error) {
	r.hasBlockingCalls++
	return r.blocking, nil
}

func TestCreateUserSubscribeUsesBlockingSubscriptionPolicy(t *testing.T) {
	users := &adminSubscriptionPolicyUserRepo{blocking: true}
	svc := NewService(Deps{
		Users:       users,
		UserSubs:    users,
		SingleModel: func() bool { return true },
	})

	err := svc.CreateUserSubscribe(context.Background(), &dto.CreateUserSubscribeRequest{UserId: 7})
	if err == nil || !strings.Contains(err.Error(), "Single subscribe mode exceeds limit") {
		t.Fatalf("CreateUserSubscribe error = %v, want single-model rejection", err)
	}
	if users.hasBlockingCalls != 1 {
		t.Fatalf("HasBlockingSubscription calls = %d, want 1", users.hasBlockingCalls)
	}
}
