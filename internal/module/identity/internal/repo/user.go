// Package repo holds the identity module's repository implementations: the
// user account rows, auth methods, devices and the cross-domain cache
// facade (ADR-001 step-6 preparation).
package repo

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/cache"
	"github.com/perfect-panel/server/pkg/logger"
)

// Cache key prefixes shared with the user entity's key derivation.
const (
	cacheUserIdPrefix           = "cache:user:id:"
	cacheUserStatePrefix        = "cache:user:state:"
	cacheUserEmailPrefix        = "cache:user:email:v2:"
	cacheUserDeviceNumberPrefix = "cache:user:device:number:"
	cacheUserDeviceIdPrefix     = "cache:user:device:id:"
)

var _ repository.UserRepo = (*UserRepo)(nil)
var _ repository.UserAuthRepo = (*UserRepo)(nil)
var _ repository.UserDeviceRepo = (*UserRepo)(nil)
var _ repository.UserCacheRepo = (*UserRepo)(nil)

type UserRepo struct {
	cache.CachedConn
	table string
	// bridges are the identity bundle's cross-domain windows: the
	// subscription cache cascade, the subscription-membership filters and
	// billing's order statistics all go through them instead of touching
	// foreign tables from identity SQL.
	bridges repository.IdentityBridges
}

// NewUserRepo builds the module-owned implementation over the shared cached
// connection; the bridges feed the cross-domain cascades and filters.
func NewUserRepo(conn cache.CachedConn, bridges repository.IdentityBridges) *UserRepo {
	return &UserRepo{
		CachedConn: conn,
		table:      "user",
		bridges:    bridges,
	}
}

// --- internal helpers ---

func (m *UserRepo) getCacheKeys(data *user.User) []string {
	if data == nil {
		return []string{}
	}
	return data.GetCacheKeys()
}

func (m *UserRepo) batchGetCacheKeys(users ...*user.User) []string {
	var keys []string
	for _, u := range users {
		keys = append(keys, u.GetCacheKeys()...)
	}
	return keys
}

// --- cache helpers ---

func (m *UserRepo) ClearUserCache(ctx context.Context, users ...*user.User) error {
	if len(users) == 0 {
		return nil
	}
	var keys []string
	for _, u := range users {
		if u != nil {
			keys = append(keys, u.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

func (m *UserRepo) ClearDeviceCache(ctx context.Context, devices ...*user.Device) error {
	if len(devices) == 0 {
		return nil
	}
	var keys []string
	for _, d := range devices {
		if d != nil {
			keys = append(keys, d.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

func (m *UserRepo) ClearAuthMethodCache(ctx context.Context, authMethods ...*user.AuthMethods) error {
	if len(authMethods) == 0 {
		return nil
	}
	var keys []string
	for _, a := range authMethods {
		if a != nil {
			keys = append(keys, a.GetCacheKeys()...)
		}
	}
	return m.CachedConn.DelCacheCtx(ctx, keys...)
}

func (m *UserRepo) BatchClearRelatedCache(ctx context.Context, u *user.User) error {
	if u == nil {
		return nil
	}
	var allKeys []string
	allKeys = append(allKeys, u.GetCacheKeys()...)

	for _, auth := range u.AuthMethods {
		allKeys = append(allKeys, auth.GetCacheKeys()...)
	}

	for _, device := range u.UserDevices {
		allKeys = append(allKeys, device.GetCacheKeys()...)
	}

	subscribes, err := m.bridges.SubscriptionCache.QueryUserSubscribe(ctx, u.Id)
	if err != nil {
		logger.Errorf("failed to query user subscribes for cache clearing: %v", err)
	} else {
		for _, sub := range subscribes {
			subModel := &usersub.Subscribe{
				Id:          sub.Id,
				UserId:      sub.UserId,
				Token:       sub.Token,
				SubscribeId: sub.SubscribeId,
			}
			allKeys = append(allKeys, subModel.GetCacheKeys()...)
		}
	}

	return m.CachedConn.DelCacheCtx(ctx, allKeys...)
}

// ClearSubscribeCache and UpdateUserSubscribeCache delegate to the
// subscription repo: the cache facade (UserCacheRepo) stays one object for
// its consumers while each domain owns its keys.
func (m *UserRepo) ClearSubscribeCache(ctx context.Context, data ...*usersub.Subscribe) error {
	return m.bridges.SubscriptionCache.ClearSubscribeCache(ctx, data...)
}

func (m *UserRepo) UpdateUserSubscribeCache(ctx context.Context, data *usersub.Subscribe) error {
	return m.bridges.SubscriptionCache.UpdateUserSubscribeCache(ctx, data)
}
