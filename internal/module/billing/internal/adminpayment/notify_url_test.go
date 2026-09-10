package adminpayment

import (
	"context"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/repository"
)

type listPaymentRepo struct {
	repository.PaymentRepo
	rows []*payment.Payment
}

func (r listPaymentRepo) FindListByPage(context.Context, int, int, *payment.Filter) (int64, []*payment.Payment, error) {
	return int64(len(r.rows)), r.rows, nil
}

func TestListPaymentNotifyURLs(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		domain   string
		host     string
		want     string
	}{
		{"stripe domain", "Stripe", "https://pay.example.test/", "panel.example.test", "https://pay.example.test/v1/notify/Stripe/test-token"},
		{"cryptomus domain", "Cryptomus", "https://pay.example.test", "panel.example.test", "https://pay.example.test/v1/notify/Cryptomus/test-token"},
		{"configured base path", "EPay", "https://pay.example.test/custom/", "panel.example.test", "https://pay.example.test/custom/v1/notify/EPay/test-token"},
		{"host fallback", "EPay", "", "panel.example.test/", "https://panel.example.test/v1/notify/EPay/test-token"},
		{"balance has no callback", "balance", "https://pay.example.test", "panel.example.test", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enabled := true
			repo := listPaymentRepo{rows: []*payment.Payment{{
				Platform: tt.platform, Domain: tt.domain, Token: "test-token", Enable: &enabled,
			}}}
			svc := NewService(repo, nil, nil, tt.host)
			resp, err := svc.List(context.Background(), &dto.GetPaymentMethodListRequest{Page: 1, Size: 10})
			if err != nil {
				t.Fatal(err)
			}
			if resp.Total != 1 || len(resp.List) != 1 {
				t.Fatalf("unexpected payment list: %+v", resp)
			}
			if got := resp.List[0].NotifyURL; got != tt.want {
				t.Fatalf("NotifyURL = %q, want %q", got, tt.want)
			}
		})
	}
}
