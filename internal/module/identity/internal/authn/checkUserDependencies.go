package auth

import "github.com/perfect-panel/server/internal/repository"

// AccountIdentityStore is the persistence surface used by account existence
// checks. It excludes unrelated application repositories.
type AccountIdentityStore interface {
	UserAuth() repository.UserAuthRepo
}

// CheckUserDependencies explicitly declares the collaborators of account
// existence checks instead of passing Application to business logic.
type CheckUserDependencies struct {
	Store AccountIdentityStore
}
