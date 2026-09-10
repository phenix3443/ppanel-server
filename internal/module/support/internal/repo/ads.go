package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/perfect-panel/server/internal/repository"

	"github.com/perfect-panel/server/internal/module/support/entity/ads"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

var cacheAdsIdPrefix = "cache:ads:id:"

var _ repository.AdsRepo = (*adsRepo)(nil)

type adsRepo struct {
	cache.CachedConn
	table string
}

// NewAdsRepo builds the module-owned implementation over the shared
// cached connection.
func NewAdsRepo(conn cache.CachedConn) repository.AdsRepo {
	return &adsRepo{
		CachedConn: conn,
		table:      "ads",
	}
}

func (m *adsRepo) getCacheKeys(data *ads.Ads) []string {
	if data == nil {
		return []string{}
	}
	adsIdKey := fmt.Sprintf("%s%v", cacheAdsIdPrefix, data.Id)
	return []string{
		adsIdKey,
	}
}

func (m *adsRepo) Insert(ctx context.Context, data *ads.Ads) error {
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Create(&data).Error
	}, m.getCacheKeys(data)...)
}

func (m *adsRepo) FindOne(ctx context.Context, id int64) (*ads.Ads, error) {
	var resp ads.Ads
	err := m.QueryNoCacheCtx(ctx, &resp, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&ads.Ads{}).Where("id = ?", id).First(&resp).Error
	})
	return &resp, err
}

func (m *adsRepo) Update(ctx context.Context, data *ads.Ads) error {
	old, err := m.FindOne(ctx, data.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Save(data).Error
	}, m.getCacheKeys(old)...)
}

func (m *adsRepo) Delete(ctx context.Context, id int64) error {
	data, err := m.FindOne(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Delete(&ads.Ads{}, id).Error
	}, m.getCacheKeys(data)...)
}

// GetAdsListByPage get ads list by page
func (m *adsRepo) GetAdsListByPage(ctx context.Context, page, size int, filter ads.Filter) (int64, []*ads.Ads, error) {
	var list []*ads.Ads
	var total int64
	page, size = repository.NormalizePage(page, size)
	err := m.QueryNoCacheCtx(ctx, &list, func(conn *gorm.DB, v interface{}) error {
		conn = conn.Model(&ads.Ads{})
		if filter.Status != nil {
			conn = conn.Where("status = ?", *filter.Status)
		}
		if filter.Search != "" {
			conn = conn.Scopes(orm.ContainsLike([]string{"title", "content"}, filter.Search))
		}
		return conn.Count(&total).Offset((page - 1) * size).Limit(size).Find(v).Error
	})
	return total, list, err
}
