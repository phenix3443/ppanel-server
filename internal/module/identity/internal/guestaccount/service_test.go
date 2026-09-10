package guestaccount

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

var errGuestWrite = errors.New("guest write failed")

type guestStore struct {
	repository.Store
	account      *user.User
	credential   *user.AuthMethods
	markers      map[string]string
	fail         string
	transactions int
}

func (s *guestStore) User() repository.UserRepo         { return guestUsers{s: s} }
func (s *guestStore) UserAuth() repository.UserAuthRepo { return guestAuth{s: s} }
func (s *guestStore) Inbox() repository.InboxRepo       { return guestInbox{s: s} }
func (s *guestStore) InIdentityTx(_ context.Context, fn func(repository.IdentityStore) error) error {
	beforeUser, beforeAuth, beforeMarkers := s.account, s.credential, maps.Clone(s.markers)
	s.transactions++
	if err := fn(s); err != nil {
		s.account, s.credential, s.markers = beforeUser, beforeAuth, beforeMarkers
		return err
	}
	return nil
}

type guestUsers struct {
	repository.UserRepo
	s *guestStore
}

func (r guestUsers) Insert(_ context.Context, u *user.User, _ ...*gorm.DB) error {
	u.Id = 11
	copy := *u
	r.s.account = &copy
	return nil
}
func (r guestUsers) Update(_ context.Context, u *user.User, _ ...*gorm.DB) error {
	if r.s.fail == "user" {
		return errGuestWrite
	}
	copy := *u
	r.s.account = &copy
	return nil
}
func (r guestUsers) FindOneByReferCode(_ context.Context, code string) (*user.User, error) {
	if code == "referral" {
		return &user.User{Id: 99}, nil
	}
	return nil, gorm.ErrRecordNotFound
}

type guestAuth struct {
	repository.UserAuthRepo
	s *guestStore
}

func (r guestAuth) InsertUserAuthMethods(_ context.Context, a *user.AuthMethods, _ ...*gorm.DB) error {
	if r.s.fail == "auth" {
		return errGuestWrite
	}
	copy := *a
	r.s.credential = &copy
	return nil
}

type guestInbox struct {
	repository.InboxRepo
	s *guestStore
}

func (r guestInbox) Find(_ context.Context, consumer, key string) (*inbox.Record, error) {
	result, ok := r.s.markers[consumer+"|"+key]
	if !ok {
		return nil, nil
	}
	return &inbox.Record{Consumer: consumer, EventKey: key, Result: result}, nil
}
func (r guestInbox) Insert(_ context.Context, consumer, key, result string) error {
	if r.s.fail == "inbox" {
		return errGuestWrite
	}
	if _, exists := r.s.markers[consumer+"|"+key]; exists {
		return errors.New("duplicate inbox marker")
	}
	r.s.markers[consumer+"|"+key] = result
	return nil
}

func TestGuestAccountReplayKeepsIdentityAndPasswordHash(t *testing.T) {
	store := &guestStore{markers: map[string]string{}}
	svc := New(store)
	hash := password.EncodePassWord("guest-password")
	command := Command{OrderNo: "order-1", AuthType: "email", Identifier: "guest@example.test", PasswordHash: hash, InviteCode: "referral"}
	id, err := svc.EnsureGuestAccount(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if id != 11 || store.account.Password != hash || store.account.Algo != password.PasswordAlgoForHash(hash) || store.account.RefererId != 99 || store.account.ReferCode == "" {
		t.Fatal("guest account lost its identity, password hash or referral settings")
	}
	if store.credential.UserId != id || store.credential.AuthIdentifier != command.Identifier || store.credential.AuthType != command.AuthType {
		t.Fatal("authentication identity was not bound to the account")
	}
	if store.markers["identity.guest_account|order-1"] != "11" {
		t.Fatal("durable consumer identity changed")
	}
	replayed, err := svc.EnsureGuestAccount(context.Background(), Command{OrderNo: command.OrderNo})
	if err != nil || replayed != id || store.transactions != 1 {
		t.Fatalf("replay re-created account: id=%d tx=%d err=%v", replayed, store.transactions, err)
	}
}

func TestGuestAccountLegacyPasswordIsHashedByIdentity(t *testing.T) {
	store := &guestStore{markers: map[string]string{}}
	_, err := New(store).EnsureGuestAccount(context.Background(), Command{OrderNo: "legacy", AuthType: "email", Identifier: "legacy@example.test", LegacyPassword: "old-password"})
	if err != nil {
		t.Fatal(err)
	}
	if store.account.Password == "old-password" || !password.VerifyPassWord("old-password", store.account.Password) {
		t.Fatal("legacy plaintext was not converted to a valid password hash")
	}
}

func TestGuestAccountMissingPasswordDoesNotCreateAccount(t *testing.T) {
	store := &guestStore{markers: map[string]string{}}
	_, err := New(store).EnsureGuestAccount(context.Background(), Command{
		OrderNo: "missing-password", AuthType: "email", Identifier: "guest@example.test",
	})
	if err == nil {
		t.Fatal("missing credentials must not be converted into an empty-password account")
	}
	if store.transactions != 0 || store.account != nil || store.credential != nil || len(store.markers) != 0 {
		t.Fatal("missing credentials must fail before any identity writes")
	}
}

func TestGuestAccountFailureRollsBackAccountAuthAndMarker(t *testing.T) {
	for _, failure := range []string{"user", "auth", "inbox"} {
		t.Run(failure, func(t *testing.T) {
			store := &guestStore{markers: map[string]string{}, fail: failure}
			svc := New(store)
			command := Command{OrderNo: "retry", AuthType: "email", Identifier: "retry@example.test", PasswordHash: "stored-hash"}
			if _, err := svc.EnsureGuestAccount(context.Background(), command); !errors.Is(err, errGuestWrite) {
				t.Fatalf("expected write failure, got %v", err)
			}
			if store.account != nil || store.credential != nil || len(store.markers) != 0 {
				t.Fatal("failed transaction left partial account state")
			}
			store.fail = ""
			if _, err := svc.EnsureGuestAccount(context.Background(), command); err != nil {
				t.Fatal(err)
			}
			if store.account == nil || store.credential == nil || len(store.markers) != 1 {
				t.Fatal("retry did not commit the complete account")
			}
		})
	}
}

func TestGuestAccountCorruptMarkerDoesNotCreateAccount(t *testing.T) {
	store := &guestStore{markers: map[string]string{"identity.guest_account|corrupt": "not-an-id"}}
	if _, err := New(store).EnsureGuestAccount(context.Background(), Command{OrderNo: "corrupt"}); err == nil {
		t.Fatal("corrupt durable marker was accepted")
	}
	if store.transactions != 0 || store.account != nil {
		t.Fatal("corrupt marker caused account creation")
	}
}
