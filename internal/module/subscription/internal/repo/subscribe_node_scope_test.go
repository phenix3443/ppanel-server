package repo

import (
	"context"
	"fmt"
	"testing"

	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/pkg/cache"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestFindByNodeScopeIsUnpaginatedAndUsesORSemantics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:subscribe-node-scope?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&subscribe.Subscribe{}); err != nil {
		t.Fatal(err)
	}
	plans := make([]subscribe.Subscribe, 125)
	for i := range plans {
		plans[i] = subscribe.Subscribe{Name: fmt.Sprintf("plan-%03d", i), Sort: int64(i + 1)}
		if i%2 == 0 {
			plans[i].Nodes = "9"
		} else {
			plans[i].NodeTags = "edge"
		}
	}
	if err := db.CreateInBatches(&plans, 50).Error; err != nil {
		t.Fatal(err)
	}

	repo := NewSubscribeRepo(cache.NewConn(db, nil), nil)
	matched, err := repo.FindByNodeScope(context.Background(), []int64{9}, []string{"edge"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matched) != len(plans) {
		t.Fatalf("matched plans = %d, want %d (must not inherit the 100-row admin cap)", len(matched), len(plans))
	}
}
