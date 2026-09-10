package repository

import (
	"context"

	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"gorm.io/gorm"
)

// SubscribeRepo subscribe 数据访问接口
type SubscribeRepo interface {
	Insert(ctx context.Context, data *subscribe.Subscribe, tx ...*gorm.DB) error
	FindOne(ctx context.Context, id int64) (*subscribe.Subscribe, error)
	Update(ctx context.Context, data *subscribe.Subscribe, tx ...*gorm.DB) error
	ReserveInventory(ctx context.Context, id int64, tx ...*gorm.DB) (bool, error)
	RestoreInventory(ctx context.Context, id int64, tx ...*gorm.DB) error
	Delete(ctx context.Context, id int64, tx ...*gorm.DB) error
	FilterList(ctx context.Context, params *subscribe.FilterParams) (int64, []*subscribe.Subscribe, error)
	FindByNodeScope(ctx context.Context, nodeIDs []int64, tags []string) ([]*subscribe.Subscribe, error)
	ClearCache(ctx context.Context, id ...int64) error
	QuerySubscribeMinSortByIds(ctx context.Context, ids []int64) (int64, error)
	QueryResetCycleSubscribeIds(ctx context.Context, resetCycle int) ([]int64, error)
	UpdateSort(ctx context.Context, data []*subscribe.Subscribe) error
	QueryGroupList(ctx context.Context) (int64, []*subscribe.Group, error)
	CreateGroup(ctx context.Context, data *subscribe.Group) error
	UpdateGroup(ctx context.Context, data *subscribe.Group) error
	DeleteGroup(ctx context.Context, id int64) error
	BatchDeleteGroup(ctx context.Context, ids []int64) error
}
