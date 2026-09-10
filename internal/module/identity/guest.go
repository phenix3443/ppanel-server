package identity

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/internal/guestaccount"
)

type GuestAccountCommand = guestaccount.Command

// GuestAccounts is the identity-owned capability used by paid guest orders.
// Persistence is supplied once at construction, never by an operation caller.
type GuestAccounts interface {
	FindGuestAccount(context.Context, string) (int64, bool, error)
	EnsureGuestAccount(context.Context, GuestAccountCommand) (int64, error)
}

func NewGuestAccounts(store guestaccount.Store) GuestAccounts {
	return guestaccount.New(store)
}
