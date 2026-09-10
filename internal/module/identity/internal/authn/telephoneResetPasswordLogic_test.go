package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

type fakeTelephonePasswordResetPolicy struct {
	methods []string
	err     error
}

func (p *fakeTelephonePasswordResetPolicy) EnsureMethodEnabled(_ context.Context, method string) error {
	p.methods = append(p.methods, method)
	return p.err
}

func TestTelephoneResetPasswordUsesInjectedMethodPolicy(t *testing.T) {
	blocked := errors.New("mobile reset disabled")
	policy := &fakeTelephonePasswordResetPolicy{err: blocked}
	logic := NewTelephoneResetPasswordLogic(context.Background(), TelephoneResetPasswordDependencies{Policy: policy})

	_, err := logic.TelephoneResetPassword(&dto.TelephoneResetPasswordRequest{})
	if !errors.Is(err, blocked) {
		t.Fatalf("TelephoneResetPassword error = %v, want method policy error", err)
	}
	if len(policy.methods) != 1 || policy.methods[0] != identifier.Mobile {
		t.Fatalf("method policy calls = %#v, want [mobile]", policy.methods)
	}
}
