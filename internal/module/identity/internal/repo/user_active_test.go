package repo

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFindEnabledUserIDsExcludesDeletedAndDisabledUsers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:active-user-ids?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&user.User{}); err != nil {
		t.Fatal(err)
	}
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })
	repo := NewUserRepo(repository.ModuleConn{DB: db, Redis: redisClient}.Conn(), repository.IdentityBridges{})

	enabled, disabled := true, false
	users := []*user.User{{Enable: &enabled}, {Enable: &disabled}, {Enable: &enabled}}
	for _, item := range users {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Delete(users[2]).Error; err != nil {
		t.Fatal(err)
	}
	ids, err := repo.FindEnabledUserIDs(context.Background(), []int64{users[0].Id, users[1].Id, users[2].Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != users[0].Id {
		t.Fatalf("enabled ids = %v, want [%d]", ids, users[0].Id)
	}
}
