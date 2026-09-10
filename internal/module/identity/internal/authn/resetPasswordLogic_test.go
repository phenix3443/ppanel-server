package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

type fakeEmailPasswordResetPolicy struct {
	methods []string
	err     error
}

func (p *fakeEmailPasswordResetPolicy) EnsureMethodEnabled(_ context.Context, method string) error {
	p.methods = append(p.methods, method)
	return p.err
}

func TestResetPasswordUsesInjectedMethodPolicy(t *testing.T) {
	blocked := errors.New("email reset disabled")
	policy := &fakeEmailPasswordResetPolicy{err: blocked}
	logic := NewResetPasswordLogic(context.Background(), ResetPasswordDependencies{Policy: policy})

	_, err := logic.ResetPassword(&dto.ResetPasswordRequest{})
	if !errors.Is(err, blocked) {
		t.Fatalf("ResetPassword error = %v, want method policy error", err)
	}
	if len(policy.methods) != 1 || policy.methods[0] != identifier.Email {
		t.Fatalf("method policy calls = %#v, want [email]", policy.methods)
	}
}
