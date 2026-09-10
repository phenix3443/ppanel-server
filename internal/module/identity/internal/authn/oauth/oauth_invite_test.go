package oauth

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	platformlog "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

type oauthInviteUserRepo struct {
	repository.UserRepo
	referer     *user.User
	foundUser   *user.User
	findErr     error
	wantInvite  string
	inserted    *user.User
	updatedUser *user.User
}

func (r *oauthInviteUserRepo) FindOne(_ context.Context, _ int64) (*user.User, error) {
	return r.foundUser, r.findErr
}

func (r *oauthInviteUserRepo) FindOneByReferCode(_ context.Context, invite string) (*user.User, error) {
	if invite != r.wantInvite {
		return nil, errors.New("unexpected invite code")
	}
	return r.referer, r.findErr
}

func (r *oauthInviteUserRepo) Insert(_ context.Context, data *user.User, _ ...*gorm.DB) error {
	data.Id = 42
	copy := *data
	r.inserted = &copy
	return nil
}

func (r *oauthInviteUserRepo) Update(_ context.Context, data *user.User, _ ...*gorm.DB) error {
	copy := *data
	r.updatedUser = &copy
	return nil
}

type oauthInviteUserAuthRepo struct {
	repository.UserAuthRepo
	found *user.AuthMethods
	err   error
}

func (r oauthInviteUserAuthRepo) FindUserAuthMethodByOpenID(context.Context, string, string) (*user.AuthMethods, error) {
	return r.found, r.err
}

func (oauthInviteUserAuthRepo) InsertUserAuthMethods(context.Context, *user.AuthMethods, ...*gorm.DB) error {
	return nil
}

type oauthInviteOutboxRepo struct {
	repository.OutboxRepo
}

func (oauthInviteOutboxRepo) Append(context.Context, string, string, string) error {
	return nil
}

type oauthInviteLogRepo struct {
	repository.LogRepo
}

func (oauthInviteLogRepo) Insert(context.Context, *platformlog.SystemLog) error {
	return nil
}

type oauthInviteIdentityStore struct {
	repository.IdentityStore
	users    repository.UserRepo
	userAuth repository.UserAuthRepo
	outbox   repository.OutboxRepo
	logs     repository.LogRepo
}

func (s oauthInviteIdentityStore) User() repository.UserRepo         { return s.users }
func (s oauthInviteIdentityStore) UserAuth() repository.UserAuthRepo { return s.userAuth }
func (s oauthInviteIdentityStore) Outbox() repository.OutboxRepo     { return s.outbox }
func (s oauthInviteIdentityStore) Log() repository.LogRepo           { return s.logs }

type oauthInviteStore struct {
	OAuthLoginStore
	users    repository.UserRepo
	userAuth repository.UserAuthRepo
	identity repository.IdentityStore
}

func (s oauthInviteStore) User() repository.UserRepo { return s.users }
func (s oauthInviteStore) UserAuth() repository.UserAuthRepo {
	return s.userAuth
}
func (s oauthInviteStore) InIdentityTx(ctx context.Context, fn func(repository.IdentityStore) error) error {
	return fn(s.identity)
}

func TestOAuthRegistrationRequiresInviteWhenForced(t *testing.T) {
	logic := NewOAuthLoginGetTokenLogic(context.Background(), OAuthLoginDependencies{
		Config: OAuthLoginConfig{InviteForced: true},
		Policy: &fakeOAuthRegistrationPolicy{},
	})

	_, err := logic.register("", "", OAuthTelegram, "openid", "", "request-id", "192.0.2.1", "test-agent")
	assertOAuthInviteError(t, err)
}

func TestExistingOAuthLoginDoesNotRequireInvite(t *testing.T) {
	enabled := true
	existing := &user.User{Id: 9, Enable: &enabled}
	logic := NewOAuthLoginGetTokenLogic(context.Background(), OAuthLoginDependencies{
		Store: oauthInviteStore{
			users: &oauthInviteUserRepo{foundUser: existing},
			userAuth: oauthInviteUserAuthRepo{
				found: &user.AuthMethods{UserId: existing.Id},
			},
		},
		Config: OAuthLoginConfig{InviteForced: true},
	})

	found, err := logic.findOrRegisterUser(OAuthGoogle, "openid", "", "", "", "request-id", "192.0.2.1", "test-agent")
	if err != nil {
		t.Fatalf("findOrRegisterUser() error = %v", err)
	}
	if found != existing {
		t.Fatalf("found user = %#v, want existing user %#v", found, existing)
	}
}

func TestOAuthRegistrationBindsValidInviteReferer(t *testing.T) {
	users := &oauthInviteUserRepo{
		referer:    &user.User{Id: 7},
		wantInvite: "VALID-CODE",
	}
	identity := oauthInviteIdentityStore{
		users:    users,
		userAuth: oauthInviteUserAuthRepo{},
		outbox:   oauthInviteOutboxRepo{},
		logs:     oauthInviteLogRepo{},
	}
	logic := NewOAuthLoginGetTokenLogic(context.Background(), OAuthLoginDependencies{
		Store: oauthInviteStore{
			users:    users,
			identity: identity,
		},
		Config: OAuthLoginConfig{InviteForced: true},
		Policy: &fakeOAuthRegistrationPolicy{},
	})

	registered, err := logic.register("", "avatar", OAuthTelegram, "openid", "VALID-CODE", "request-id", "192.0.2.1", "test-agent")
	if err != nil {
		t.Fatalf("register() error = %v", err)
	}
	if registered.RefererId != 7 {
		t.Fatalf("registered RefererId = %d, want 7", registered.RefererId)
	}
	if users.inserted == nil || users.inserted.RefererId != 7 {
		t.Fatalf("inserted user = %#v, want RefererId 7", users.inserted)
	}
}

func TestOAuthRegistrationRejectsInvalidInvite(t *testing.T) {
	users := &oauthInviteUserRepo{
		findErr:    gorm.ErrRecordNotFound,
		wantInvite: "INVALID-CODE",
	}
	logic := NewOAuthLoginGetTokenLogic(context.Background(), OAuthLoginDependencies{
		Store:  oauthInviteStore{users: users},
		Policy: &fakeOAuthRegistrationPolicy{},
	})

	_, err := logic.register("", "", OAuthTelegram, "openid", "INVALID-CODE", "request-id", "192.0.2.1", "test-agent")
	assertOAuthInviteError(t, err)
}

func assertOAuthInviteError(t *testing.T, err error) {
	t.Helper()
	var codeErr *xerr.CodeError
	if !errors.As(err, &codeErr) {
		t.Fatalf("error = %v, want *xerr.CodeError", err)
	}
	if codeErr.GetErrCode() != xerr.InviteCodeError {
		t.Fatalf("error code = %d, want %d", codeErr.GetErrCode(), xerr.InviteCodeError)
	}
}
