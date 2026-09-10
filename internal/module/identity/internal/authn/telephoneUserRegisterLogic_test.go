package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

type fakeTelephoneRegistrationPolicy struct {
	registrationMethods []string
	err                 error
}

func (p *fakeTelephoneRegistrationPolicy) EnsureRegistrationOpen(_ context.Context, method string) error {
	p.registrationMethods = append(p.registrationMethods, method)
	return p.err
}

func (p *fakeTelephoneRegistrationPolicy) VerifyHuman(context.Context, string, string) error {
	return nil
}
func (p *fakeTelephoneRegistrationPolicy) TakeIPPermit(context.Context, string) error { return nil }

func TestTelephoneUserRegisterUsesInjectedRegistrationPolicy(t *testing.T) {
	blocked := errors.New("registration blocked")
	policy := &fakeTelephoneRegistrationPolicy{err: blocked}
	logic := NewTelephoneUserRegisterLogic(context.Background(), TelephoneUserRegisterDependencies{Policy: policy})

	_, err := logic.TelephoneUserRegister(&dto.TelephoneRegisterRequest{})
	if !errors.Is(err, blocked) {
		t.Fatalf("TelephoneUserRegister error = %v, want registration policy error", err)
	}
	if len(policy.registrationMethods) != 1 || policy.registrationMethods[0] != identifier.Mobile {
		t.Fatalf("registration methods = %#v, want [mobile]", policy.registrationMethods)
	}
}
