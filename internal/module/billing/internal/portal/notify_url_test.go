package portal

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
)

func TestPaymentPublicBaseURL(t *testing.T) {
	tests := []struct {
		name        string
		domain      string
		requestHost string
		configHost  string
		want        string
	}{
		{"payment domain wins", "https://pay.example.test/", "request.example.test", "panel.example.test", "https://pay.example.test"},
		{"configured base path", "https://pay.example.test/custom/", "", "panel.example.test", "https://pay.example.test/custom"},
		{"request host fallback", "", "request.example.test/", "panel.example.test", "https://request.example.test"},
		{"config host fallback", "", "", "panel.example.test/", "https://panel.example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.requestHost != "" {
				ctx = context.WithValue(ctx, requestctx.CtxKeyRequestHost, tt.requestHost)
			}
			logic := NewPurchaseCheckoutLogic(ctx, CheckoutDependencies{
				Config: CheckoutConfig{Host: tt.configHost},
			})
			if got := logic.paymentPublicBaseURL(&payment.Payment{Domain: tt.domain}); got != tt.want {
				t.Fatalf("paymentPublicBaseURL = %q, want %q", got, tt.want)
			}
		})
	}
}
