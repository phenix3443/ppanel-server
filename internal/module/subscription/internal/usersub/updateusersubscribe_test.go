package usersub

import (
	"context"
	"testing"
	"time"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	subEntity "github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type updateUserSubscribeRepo struct {
	repository.UserSubscriptionRepo
	updated *subEntity.Subscribe
}

func (r *updateUserSubscribeRepo) FindOneSubscribe(_ context.Context, id int64) (*subEntity.Subscribe, error) {
	return &subEntity.Subscribe{Id: id, UserId: 7}, nil
}

func (r *updateUserSubscribeRepo) UpdateSubscribe(_ context.Context, data *subEntity.Subscribe, _ ...*gorm.DB) error {
	r.updated = data
	return nil
}

type updateUserSubscribeCache struct {
	repository.UserCacheRepo
}

func (updateUserSubscribeCache) ClearSubscribeCache(_ context.Context, _ ...*subEntity.Subscribe) error {
	return nil
}

type updateUserSubscribePlans struct {
	repository.SubscribeRepo
}

func (updateUserSubscribePlans) ClearCache(_ context.Context, _ ...int64) error { return nil }

func TestUpdateUserSubscribeStatusFromExpiredAt(t *testing.T) {
	tests := []struct {
		name       string
		expiredAt  int64
		wantStatus uint8
	}{
		// ExpiredAt == 0 is the NoLimit sentinel and must stay active,
		// otherwise the node allowlist (status IN (1, 0)) drops the user.
		{name: "no-limit sentinel stays active", expiredAt: 0, wantStatus: 1},
		{name: "past expiry marks expired", expiredAt: time.Now().Add(-time.Hour).UnixMilli(), wantStatus: 3},
		{name: "future expiry stays active", expiredAt: time.Now().Add(time.Hour).UnixMilli(), wantStatus: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &updateUserSubscribeRepo{}
			svc := NewService(Deps{
				UserSubs: repo,
				Cache:    updateUserSubscribeCache{},
				Plans:    updateUserSubscribePlans{},
			})

			err := svc.UpdateUserSubscribe(context.Background(), &dto.UpdateUserSubscribeRequest{
				UserSubscribeId: 1,
				ExpiredAt:       tt.expiredAt,
			})
			if err != nil {
				t.Fatalf("UpdateUserSubscribe error = %v", err)
			}
			if repo.updated == nil {
				t.Fatal("UpdateSubscribe was not called")
			}
			if repo.updated.Status != tt.wantStatus {
				t.Fatalf("Status = %d, want %d", repo.updated.Status, tt.wantStatus)
			}
		})
	}
}
