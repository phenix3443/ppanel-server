package usersub

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	subEntity "github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type resetTokenRepo struct {
	repository.UserSubscriptionRepo
	stored  subEntity.Subscribe
	updated *subEntity.Subscribe
}

func (r *resetTokenRepo) FindOneSubscribe(_ context.Context, id int64) (*subEntity.Subscribe, error) {
	sub := r.stored
	sub.Id = id
	return &sub, nil
}

func (r *resetTokenRepo) UpdateSubscribe(_ context.Context, data *subEntity.Subscribe, _ ...*gorm.DB) error {
	r.updated = data
	return nil
}

type resetTokenCache struct {
	repository.UserCacheRepo
}

func (resetTokenCache) ClearSubscribeCache(_ context.Context, _ ...*subEntity.Subscribe) error {
	return nil
}

type resetTokenPlans struct {
	repository.SubscribeRepo
}

func (resetTokenPlans) ClearCache(_ context.Context, _ ...int64) error { return nil }

// Resetting a subscription must rotate the node-side credential as well as the
// subscription token. Rotating only the token leaves anyone who already pulled
// the config connecting indefinitely, which defeats the purpose of the reset.
func TestResetUserSubscribeTokenRotatesTokenAndUUID(t *testing.T) {
	repo := &resetTokenRepo{
		stored: subEntity.Subscribe{
			UserId: 7,
			Token:  "old-token",
			UUID:   "old-uuid",
		},
	}
	svc := NewService(Deps{
		UserSubs: repo,
		Cache:    resetTokenCache{},
		Plans:    resetTokenPlans{},
	})

	if err := svc.ResetUserSubscribeToken(context.Background(), &dto.ResetUserSubscribeTokenRequest{
		UserSubscribeId: 1,
	}); err != nil {
		t.Fatalf("ResetUserSubscribeToken error = %v", err)
	}

	if repo.updated == nil {
		t.Fatal("UpdateSubscribe was not called")
	}
	if repo.updated.Token == "" || repo.updated.Token == repo.stored.Token {
		t.Fatalf("Token = %q, want a new non-empty value", repo.updated.Token)
	}
	if repo.updated.UUID == "" || repo.updated.UUID == repo.stored.UUID {
		t.Fatalf("UUID = %q, want a new non-empty value", repo.updated.UUID)
	}
}
