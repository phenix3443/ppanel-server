package auth

import (
	"context"
	"testing"

	identifier2 "github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
)

type checkUserTestStore struct {
	repository.Store
	userAuth repository.UserAuthRepo
}

func (s checkUserTestStore) UserAuth() repository.UserAuthRepo { return s.userAuth }

type checkUserAuthRepo struct {
	repository.UserAuthRepo
	method     string
	identifier string
	result     *user.AuthMethods
}

func (r *checkUserAuthRepo) FindUserAuthMethodByOpenID(_ context.Context, method, identifier string) (*user.AuthMethods, error) {
	r.method = method
	r.identifier = identifier
	return r.result, nil
}

func TestCheckUserUsesInjectedIdentityStore(t *testing.T) {
	repo := &checkUserAuthRepo{result: &user.AuthMethods{UserId: 7}}
	logic := NewCheckUserLogic(context.Background(), CheckUserDependencies{
		Store: checkUserTestStore{userAuth: repo},
	})

	resp, err := logic.CheckUser(&dto.CheckUserRequest{Email: "user@example.com"})
	if err != nil {
		t.Fatalf("CheckUser error = %v", err)
	}
	if !resp.Exist || repo.method != identifier2.Email || repo.identifier != "user@example.com" {
		t.Fatalf("response/repository call = %#v, %q, %q", resp, repo.method, repo.identifier)
	}
}

func TestCheckUserTelephoneUsesInjectedIdentityStore(t *testing.T) {
	repo := &checkUserAuthRepo{result: &user.AuthMethods{UserId: 7}}
	logic := NewCheckUserTelephoneLogic(context.Background(), CheckUserDependencies{
		Store: checkUserTestStore{userAuth: repo},
	})

	resp, err := logic.CheckUserTelephone(&dto.TelephoneCheckUserRequest{TelephoneAreaCode: "86", Telephone: "15502505555"})
	if err != nil {
		t.Fatalf("CheckUserTelephone error = %v", err)
	}
	if !resp.Exist || repo.method != identifier2.Mobile || repo.identifier != "+8615502505555" {
		t.Fatalf("response/repository call = %#v, %q, %q", resp, repo.method, repo.identifier)
	}
}
