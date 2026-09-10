// Package guestaccount owns the account-creation stage of paid guest orders.
package guestaccount

import (
	"context"
	"fmt"
	"strconv"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

// Consumer is persisted; changing it would recreate accounts during replay.
const Consumer = "identity.guest_account"

type Store interface {
	Inbox() repository.InboxRepo
	repository.IdentityTransactor
}

type Command struct {
	OrderNo      string
	AuthType     string
	Identifier   string
	PasswordHash string
	// LegacyPassword supports orders written before password hashes were
	// persisted. It is never written back or included in logs.
	LegacyPassword string
	InviteCode     string
}

type Service struct{ store Store }

func New(store Store) *Service { return &Service{store: store} }

// FindGuestAccount lets billing recover the committed account even when its
// historical Redis checkout snapshot has already expired.
func (s *Service) FindGuestAccount(ctx context.Context, orderNo string) (int64, bool, error) {
	mark, err := s.store.Inbox().Find(ctx, Consumer, orderNo)
	if err != nil || mark == nil {
		return 0, false, err
	}
	id, err := strconv.ParseInt(mark.Result, 10, 64)
	if err != nil {
		return 0, false, fmt.Errorf("corrupt guest account marker %q: %w", mark.Result, err)
	}
	return id, true, nil
}

func (s *Service) EnsureGuestAccount(ctx context.Context, command Command) (int64, error) {
	if id, found, err := s.FindGuestAccount(ctx, command.OrderNo); err != nil || found {
		return id, err
	}
	passwordHash := command.PasswordHash
	if passwordHash == "" {
		if command.LegacyPassword == "" {
			return 0, fmt.Errorf("guest order password hash is missing")
		}
		passwordHash = password.EncodePassWord(command.LegacyPassword)
	}
	u := &user.User{Password: passwordHash, Algo: password.PasswordAlgoForHash(passwordHash)}
	err := s.store.InIdentityTx(ctx, func(tx repository.IdentityStore) error {
		if err := tx.User().Insert(ctx, u); err != nil {
			return err
		}
		u.ReferCode = user.GenerateInviteCode(u.Id)
		if err := tx.User().Update(ctx, u); err != nil {
			return err
		}
		if err := tx.UserAuth().InsertUserAuthMethods(ctx, &user.AuthMethods{
			UserId: u.Id, AuthType: command.AuthType, AuthIdentifier: command.Identifier,
		}); err != nil {
			return err
		}
		if command.InviteCode != "" {
			if referer, err := tx.User().FindOneByReferCode(ctx, command.InviteCode); err == nil {
				u.RefererId = referer.Id
				if err := tx.User().Update(ctx, u); err != nil {
					return err
				}
			} else {
				logger.WithContext(ctx).Error("Find referer failed", logger.Field("error", err.Error()), logger.Field("refer_code", command.InviteCode))
			}
		}
		return tx.Inbox().Insert(ctx, Consumer, command.OrderNo, strconv.FormatInt(u.Id, 10))
	})
	if err != nil {
		return 0, err
	}
	return u.Id, nil
}
