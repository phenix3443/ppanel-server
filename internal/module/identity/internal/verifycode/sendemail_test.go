package verifycode

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
)

type fakeEmailCodePolicy struct {
	method string
	err    error
}

func (p *fakeEmailCodePolicy) EnsureRegistrationOpen(_ context.Context, method string) error {
	p.method = method
	return p.err
}

func (p *fakeEmailCodePolicy) EnsureMethodEnabled(context.Context, string) error { return nil }

func TestSendEmailCodeUsesInjectedRegistrationPolicy(t *testing.T) {
	blocked := errors.New("registration disabled")
	policy := &fakeEmailCodePolicy{err: blocked}
	logic := NewSendEmailCodeLogic(context.Background(), SendEmailCodeDependencies{Policy: policy})

	_, err := logic.SendEmailCode(&dto.SendCodeRequest{Email: "new@example.com", Type: uint8(auth.Register)})
	if !errors.Is(err, blocked) {
		t.Fatalf("SendEmailCode error = %v, want registration policy error", err)
	}
	if policy.method != identifier.Email {
		t.Fatalf("registration method = %q, want %q", policy.method, identifier.Email)
	}
}
