package verifycode

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
)

type fakeSmsCodePolicy struct {
	method string
	err    error
}

func (p *fakeSmsCodePolicy) EnsureRegistrationOpen(_ context.Context, method string) error {
	p.method = method
	return p.err
}

func (p *fakeSmsCodePolicy) EnsureMethodEnabled(context.Context, string) error { return nil }

func TestSendSmsCodeUsesInjectedRegistrationPolicy(t *testing.T) {
	blocked := errors.New("registration disabled")
	policy := &fakeSmsCodePolicy{err: blocked}
	logic := NewSendSmsCodeLogic(context.Background(), SendSmsCodeDependencies{Policy: policy})

	_, err := logic.SendSmsCode(&dto.SendSmsCodeRequest{Type: uint8(auth.Register)})
	if !errors.Is(err, blocked) {
		t.Fatalf("SendSmsCode error = %v, want registration policy error", err)
	}
	if policy.method != identifier.Mobile {
		t.Fatalf("registration method = %q, want %q", policy.method, identifier.Mobile)
	}
}
