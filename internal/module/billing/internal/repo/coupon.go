package repo

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/perfect-panel/server/internal/repository"

	"github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/orm"
	"gorm.io/gorm"
)

var (
	cacheCouponIdPrefix   = "cache:coupon:id:"
	cacheCouponCodePrefix = "cache:coupon:code:"
)

var _ repository.CouponRepo = (*couponRepo)(nil)

type couponRepo struct {
	cache.CachedConn
	table string
}

// NewCouponRepo builds the module-owned implementation over the shared
// cached connection.
func NewCouponRepo(conn cache.CachedConn) repository.CouponRepo {
	return &couponRepo{
		CachedConn: conn,
		table:      "coupon",
	}
}

//nolint:unused
func (m *couponRepo) batchGetCacheKeys(Coupons ...*coupon.Coupon) []string {
	var keys []string
	for _, coupon := range Coupons {
		keys = append(keys, m.getCacheKeys(coupon)...)
	}
	return keys

}

func (m *couponRepo) getCacheKeys(data *coupon.Coupon) []string {
	if data == nil {
		return []string{}
	}
	couponIdKey := fmt.Sprintf("%s%v", cacheCouponIdPrefix, data.Id)
	couponCodeKey := fmt.Sprintf("%s%v", cacheCouponCodePrefix, data.Code)
	cacheKeys := []string{
		couponIdKey,
		couponCodeKey,
	}
	return cacheKeys
}

func (m *couponRepo) Insert(ctx context.Context, data *coupon.Coupon) error {
	err := m.ExecCtx(ctx, func(conn *gorm.DB) error {
		return conn.Create(&data).Error
	}, m.getCacheKeys(data)...)
	return err
}

func (m *couponRepo) FindOne(ctx context.Context, id int64) (*coupon.Coupon, error) {
	var resp coupon.Coupon
	err := m.QueryNoCacheCtx(ctx, &resp, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&coupon.Coupon{}).Where("id = ?", id).First(&resp).Error
	})
	switch {
	case err == nil:
		return &resp, nil
	default:
		return nil, err
	}
}

func (m *couponRepo) FindOneByCode(ctx context.Context, code string) (*coupon.Coupon, error) {
	var resp coupon.Coupon
	err := m.QueryNoCacheCtx(ctx, &resp, func(conn *gorm.DB, v interface{}) error {
		return conn.Model(&coupon.Coupon{}).Where("code = ?", code).First(&resp).Error
	})
	switch {
	case err == nil:
		return &resp, nil
	default:
		return nil, err
	}
}

func (m *couponRepo) Update(ctx context.Context, data *coupon.Coupon) error {
	old, err := m.FindOne(ctx, data.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		db := conn
		return db.Save(data).Error
	}, m.getCacheKeys(old)...)
	return err
}

func (m *couponRepo) Delete(ctx context.Context, id int64) error {
	data, err := m.FindOne(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		db := conn
		return db.Delete(&coupon.Coupon{}, id).Error
	}, m.getCacheKeys(data)...)
	return err
}

// QueryCouponListByPage query coupon list by page
func (m *couponRepo) QueryCouponListByPage(ctx context.Context, page, size int, subscribe int64, search string) (total int64, list []*coupon.Coupon, err error) {
	page, size = repository.NormalizePage(page, size)
	err = m.QueryNoCacheCtx(ctx, &list, func(conn *gorm.DB, v interface{}) error {
		db := conn.Model(&coupon.Coupon{})
		if subscribe != 0 {
			db = db.Scopes(orm.CommaSeparatedContains("subscribe", []string{strconv.FormatInt(subscribe, 10)}))
		}
		if search != "" {
			db = db.Scopes(orm.PrefixLike([]string{"name", "code"}, search))
		}
		return db.Count(&total).Limit(size).Offset((page - 1) * size).Find(v).Error
	})
	return total, list, err
}

func (m *couponRepo) BatchDelete(ctx context.Context, ids []int64) error {
	var err error
	for _, id := range ids {
		if err = m.Delete(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (m *couponRepo) UpdateCount(ctx context.Context, code string) error {
	data, err := m.FindOneByCode(ctx, code)
	if err != nil {
		return err
	}
	data.UsedCount++
	return m.Update(ctx, data)
}

// ReserveUsage atomically reserves one coupon use for a pending order.  A
// reservation is made at order creation (rather than after payment) so a
// limited coupon cannot be oversold by concurrent checkouts.
func (m *couponRepo) ReserveUsage(ctx context.Context, code string, now int64, tx ...*gorm.DB) (bool, error) {
	data, err := m.FindOneByCode(ctx, code)
	if err != nil {
		return false, err
	}
	var reserved bool
	err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		result := conn.Model(&coupon.Coupon{}).
			Where("code = ? AND enable = ? AND start_time <= ? AND expire_time >= ? AND (count = 0 OR used_count < count)", code, true, now, now).
			UpdateColumn("used_count", gorm.Expr("used_count + 1"))
		reserved = result.RowsAffected == 1
		return result.Error
	}, m.getCacheKeys(data)...)
	return reserved, err
}

// ReleaseUsage returns a reservation when its pending order is closed. The
// conditional expression makes repeated close processing harmless.
func (m *couponRepo) ReleaseUsage(ctx context.Context, code string, tx ...*gorm.DB) error {
	data, err := m.FindOneByCode(ctx, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A deleted coupon must not prevent closing and refunding an already
			// pending order. There is no remaining counter to release.
			return nil
		}
		return err
	}
	return m.ExecCtx(ctx, func(conn *gorm.DB) error {
		if len(tx) > 0 {
			conn = tx[0]
		}
		return conn.Model(&coupon.Coupon{}).
			Where("code = ?", code).
			UpdateColumn("used_count", gorm.Expr("CASE WHEN used_count > 0 THEN used_count - 1 ELSE 0 END")).Error
	}, m.getCacheKeys(data)...)
}
