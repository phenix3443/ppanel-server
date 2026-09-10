// Package devicestate owns consistent device removal shared by user and admin
// entry points. Redis generations revoke sessions; the database transaction
// removes both copies of the device identity.
package devicestate

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/devicesession"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func Delete(ctx context.Context, store Store, client *redis.Client, id, ownerID int64) (*user.Device, error) {
	var removed *user.Device
	err := store.InIdentityTx(ctx, func(tx repository.IdentityStore) error {
		device, err := tx.UserDevice().FindDeviceForAuth(ctx, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if ownerID > 0 && device.UserId != ownerID {
			return xerr.NewErrCode(xerr.InvalidAccess)
		}
		if err := devicesession.Revoke(ctx, client, device.Id); err != nil {
			return err
		}
		if err := tx.UserDevice().DeleteDevice(ctx, device.Id); err != nil {
			return err
		}
		if err := tx.UserAuth().DeleteUserAuthMethodByIdentifier(ctx, "device", device.Identifier); err != nil {
			return err
		}
		removed = device
		return nil
	})
	return removed, err
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.IdentityTransactor
}
