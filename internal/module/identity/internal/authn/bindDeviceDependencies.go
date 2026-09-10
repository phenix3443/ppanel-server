package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
)

// BindDeviceStore is the persistence surface used by device binding. It
// excludes unrelated application repositories.
type BindDeviceStore interface {
	User() repository.UserRepo
	UserAuth() repository.UserAuthRepo
	UserDevice() repository.UserDeviceRepo
	InIdentityTx(ctx context.Context, fn func(repository.IdentityStore) error) error
}

// BindDeviceDependencies explicitly declares the collaborators of device
// binding instead of passing Application to business logic.
type BindDeviceDependencies struct {
	Store BindDeviceStore
}
