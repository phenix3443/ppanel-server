package profile

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	usermodel "github.com/perfect-panel/server/internal/module/identity/entity/user"
)

type fakeBindOAuthMethodPolicy struct {
	methods []string
}

func (p *fakeBindOAuthMethodPolicy) EnsureMethodEnabled(_ context.Context, method string) error {
	p.methods = append(p.methods, method)
	return nil
}

func TestBindOAuthCallbackUsesInjectedMethodPolicy(t *testing.T) {
	policy := &fakeBindOAuthMethodPolicy{}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &usermodel.User{Id: 7})
	logic := newBindOAuthCallbackLogic(ctx, Deps{Policy: policy})

	err := logic.BindOAuthCallback(&dto.BindOAuthCallbackRequest{
		Method:   "google",
		Callback: "not-an-object",
	})
	if err == nil {
		t.Fatal("expected invalid callback error")
	}
	if len(policy.methods) != 1 || policy.methods[0] != "google" {
		t.Fatalf("policy methods = %#v, want [google]", policy.methods)
	}
}
