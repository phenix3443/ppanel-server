package adminpayment

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	paymentModel "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type updatePaymentRepo struct {
	repository.PaymentRepo
	stored  *paymentModel.Payment
	updated *paymentModel.Payment
}

func TestCryptomusConfigRequiresBothCredentials(t *testing.T) {
	tests := []struct {
		name   string
		config interface{}
	}{
		{"empty object", map[string]interface{}{}},
		{"missing merchant", map[string]interface{}{"api_key": "test-key"}},
		{"missing key", map[string]interface{}{"merchant_id": "merchant-1"}},
		{"blank merchant", map[string]interface{}{"merchant_id": " \n", "api_key": "test-key"}},
		{"blank key", map[string]interface{}{"merchant_id": "merchant-1", "api_key": " \t"}},
		{"wrong field type", map[string]interface{}{"merchant_id": 123, "api_key": "test-key"}},
		{"null", nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enable := true
			repo := &updatePaymentRepo{stored: &paymentModel.Payment{
				Id: 1, Platform: "Cryptomus", Config: `{"merchant_id":"merchant-1","api_key":"test-key"}`, Enable: &enable,
			}}
			orders := &updatePaymentOrders{}
			svc := NewService(repo, orders, nil, "")
			_, err := svc.Create(context.Background(), &dto.CreatePaymentMethodRequest{
				Name: "Cryptomus", Platform: "Cryptomus", Config: test.config, Enable: &enable,
			})
			if err == nil || !strings.Contains(err.Error(), "INVALID_PAYMENT_CONFIG") {
				t.Fatalf("Create must reject before opening a transaction, got %v", err)
			}
			_, err = svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
				Id: 1, Name: "Cryptomus", Platform: "Cryptomus", Config: test.config, Enable: &enable,
			})
			if err == nil || !strings.Contains(err.Error(), "INVALID_PAYMENT_CONFIG") {
				t.Fatalf("Update must reject incomplete credentials, got %v", err)
			}
			if repo.updated != nil || orders.pendingCalls != 0 {
				t.Fatal("invalid config must not reach persistence or the pending-order check")
			}
		})
	}
}

func TestCryptomusConfigTrimsCredentials(t *testing.T) {
	encoded := parsePaymentPlatformConfig(context.Background(), payment.Cryptomus, map[string]interface{}{
		"merchant_id": " merchant-1 \n", "api_key": "\ttest-key ",
	})
	var config paymentModel.CryptomusConfig
	if err := json.Unmarshal([]byte(encoded), &config); err != nil {
		t.Fatal(err)
	}
	if config.MerchantID != "merchant-1" || config.APIKey != "test-key" {
		t.Fatal("credentials must be trimmed before persisting")
	}
}

func (r *updatePaymentRepo) FindOne(_ context.Context, _ int64) (*paymentModel.Payment, error) {
	return r.stored, nil
}

func (r *updatePaymentRepo) Update(_ context.Context, data *paymentModel.Payment, _ ...*gorm.DB) error {
	r.updated = data
	return nil
}

type updatePaymentOrders struct {
	repository.OrderRepo
	pendingCalls int
}

func (r *updatePaymentOrders) CountPendingByPaymentID(_ context.Context, _ int64) (int64, error) {
	r.pendingCalls++
	return 1, nil
}

// The seeded balance method (config is an empty string) must be toggleable:
// it has no platform config to validate and no callback, so the update must
// neither fail with INVALID_PAYMENT_CONFIG nor hit the pending-order guard.
func TestUpdateBalancePaymentMethodTogglesEnable(t *testing.T) {
	enable := true
	repo := &updatePaymentRepo{stored: &paymentModel.Payment{
		Id:       -1,
		Name:     "Balance",
		Platform: "balance",
		Enable:   new(bool),
	}}
	orders := &updatePaymentOrders{}
	svc := NewService(repo, orders, nil, "")

	_, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id:       -1,
		Name:     "Balance",
		Platform: "balance",
		Config:   map[string]interface{}{},
		Enable:   &enable,
	})
	if err != nil {
		t.Fatalf("Update error = %v, want success", err)
	}
	if repo.updated == nil {
		t.Fatal("payment method was not updated")
	}
	if repo.updated.Enable == nil || !*repo.updated.Enable {
		t.Fatal("Enable was not toggled on")
	}
	if repo.updated.Config != "" {
		t.Fatalf("Config = %q, want stored config preserved", repo.updated.Config)
	}
	if orders.pendingCalls != 0 {
		t.Fatalf("CountPendingByPaymentID calls = %d, want 0 for balance", orders.pendingCalls)
	}
}

// The storefront depends on the seeded balance method (id -1); deleting it
// breaks every balance purchase with a record-not-found at PreCreateOrder.
func TestDeleteBalancePaymentMethodIsRejected(t *testing.T) {
	repo := &updatePaymentRepo{stored: &paymentModel.Payment{
		Id:       -1,
		Name:     "Balance",
		Platform: "balance",
		Enable:   new(bool),
	}}
	orders := &updatePaymentOrders{}
	svc := NewService(repo, orders, nil, "")

	err := svc.Delete(context.Background(), &dto.DeletePaymentMethodRequest{Id: -1})
	if err == nil || !strings.Contains(err.Error(), "cannot be deleted") {
		t.Fatalf("Delete error = %v, want internal-method rejection", err)
	}
}

func TestUpdateGatewayPaymentMethodStillValidatesConfig(t *testing.T) {
	enable := true
	repo := &updatePaymentRepo{stored: &paymentModel.Payment{
		Id:       1,
		Name:     "EPay",
		Platform: "EPay",
		Enable:   new(bool),
	}}
	svc := NewService(repo, &updatePaymentOrders{}, nil, "")

	_, err := svc.Update(context.Background(), &dto.UpdatePaymentMethodRequest{
		Id:       1,
		Name:     "EPay",
		Platform: "EPay",
		Config:   "not-a-config",
		Enable:   &enable,
	})
	if err == nil {
		t.Fatal("Update error = nil, want INVALID_PAYMENT_CONFIG")
	}
}
