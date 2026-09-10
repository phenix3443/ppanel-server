package auth

import (
	"context"
	"errors"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

type fakeEmailRegistrationPolicy struct {
	registrationMethods []string
	err                 error
}

func (p *fakeEmailRegistrationPolicy) EnsureRegistrationOpen(_ context.Context, method string) error {
	p.registrationMethods = append(p.registrationMethods, method)
	return p.err
}

func (p *fakeEmailRegistrationPolicy) VerifyHuman(context.Context, string, string) error { return nil }
func (p *fakeEmailRegistrationPolicy) TakeIPPermit(context.Context, string) error        { return nil }

func TestUserRegisterUsesInjectedRegistrationPolicy(t *testing.T) {
	blocked := errors.New("registration blocked")
	policy := &fakeEmailRegistrationPolicy{err: blocked}
	logic := NewUserRegisterLogic(context.Background(), UserRegisterDependencies{Policy: policy})

	_, err := logic.UserRegister(&dto.UserRegisterRequest{Email: "new@example.com"})
	if !errors.Is(err, blocked) {
		t.Fatalf("UserRegister error = %v, want registration policy error", err)
	}
	if len(policy.registrationMethods) != 1 || policy.registrationMethods[0] != "email" {
		t.Fatalf("registration methods = %#v, want [email]", policy.registrationMethods)
	}
}
