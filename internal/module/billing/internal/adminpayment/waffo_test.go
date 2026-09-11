package adminpayment

import (
	"context"
	"encoding/json"
	"testing"

	paymentModel "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
)

func waffoConfigFields() map[string]interface{} {
	return map[string]interface{}{
		"merchant_id":  "MER_2aUyqjCzEIiEcYMKj7TZtw",
		"private_key":  "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----",
		"store_id":     "STO_2aUyqjCzEIiEcYMKj7TZtw",
		"product_id":   "PROD_2aUyqjCzEIiEcYMKj7TZtw",
		"tax_category": "saas",
		"test_mode":    "true",
	}
}

func TestWaffoConfigRequiresEveryCredential(t *testing.T) {
	for _, field := range []string{"merchant_id", "private_key", "store_id", "product_id"} {
		t.Run("missing "+field, func(t *testing.T) {
			fields := waffoConfigFields()
			delete(fields, field)
			if parsePaymentPlatformConfig(context.Background(), payment.Waffo, fields) != "" {
				t.Fatalf("a config without %s must be rejected", field)
			}
		})
	}
}

func TestWaffoConfigRejectsUnknownTaxCategory(t *testing.T) {
	fields := waffoConfigFields()
	fields["tax_category"] = "physical_goods"
	if parsePaymentPlatformConfig(context.Background(), payment.Waffo, fields) != "" {
		t.Fatal("a tax category the gateway does not accept must be rejected while it is still visible to the administrator")
	}
}

func TestWaffoConfigDefaultsTaxCategoryAndParsesTestMode(t *testing.T) {
	fields := waffoConfigFields()
	delete(fields, "tax_category")
	encoded := parsePaymentPlatformConfig(context.Background(), payment.Waffo, fields)
	if encoded == "" {
		t.Fatal("a config without a tax category must be accepted")
	}
	var config paymentModel.WaffoConfig
	if err := json.Unmarshal([]byte(encoded), &config); err != nil {
		t.Fatal(err)
	}
	if config.TaxCategory != "saas" {
		t.Fatalf("tax category = %s, want the saas default", config.TaxCategory)
	}
	// The admin form posts booleans as strings.
	if !config.TestMode {
		t.Fatal(`test_mode "true" must be stored as a boolean true`)
	}
}

func TestNotifyURLFallsBackToTheConfiguredHost(t *testing.T) {
	svc := NewService(nil, nil, nil, "panel.example")
	method := &paymentModel.Payment{Platform: "Waffo", Token: "tok"}
	if got, want := svc.notifyURL(method), "https://panel.example/v1/notify/Waffo/tok"; got != want {
		t.Fatalf("notifyURL = %s, want %s", got, want)
	}
	method.Domain = "https://pay.example/"
	if got, want := svc.notifyURL(method), "https://pay.example/v1/notify/Waffo/tok"; got != want {
		t.Fatalf("notifyURL = %s, want %s", got, want)
	}
}
