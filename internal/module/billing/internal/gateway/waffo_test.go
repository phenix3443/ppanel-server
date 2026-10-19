package gateway

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentModel "github.com/perfect-panel/server/internal/module/billing/entity/payment"
)

func waffoConfigFields() map[string]any {
	return map[string]any{
		"merchant_id":  "MER_2aUyqjCzEIiEcYMKj7TZtw",
		"private_key":  "-----BEGIN PRIVATE KEY-----\nkey\n-----END PRIVATE KEY-----",
		"store_id":     "STO_2aUyqjCzEIiEcYMKj7TZtw",
		"product_id":   "PROD_2aUyqjCzEIiEcYMKj7TZtw",
		"tax_category": "saas",
		"test_mode":    "true",
	}
}

func normalizeWaffoFields(t *testing.T, fields map[string]any) (string, error) {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return normalizeWaffo(raw)
}

func TestWaffoConfigRequiresEveryCredential(t *testing.T) {
	for _, field := range []string{"merchant_id", "private_key", "store_id", "product_id"} {
		t.Run("missing "+field, func(t *testing.T) {
			fields := waffoConfigFields()
			delete(fields, field)
			if _, err := normalizeWaffoFields(t, fields); err == nil {
				t.Fatalf("a config without %s must be rejected", field)
			}
		})
	}
}

func TestWaffoConfigRejectsUnknownTaxCategory(t *testing.T) {
	fields := waffoConfigFields()
	fields["tax_category"] = "physical_goods"
	if _, err := normalizeWaffoFields(t, fields); err == nil {
		t.Fatal("a tax category the gateway does not accept must be rejected while it is still visible to the administrator")
	}
}

func TestWaffoConfigDefaultsTaxCategoryAndParsesTestMode(t *testing.T) {
	fields := waffoConfigFields()
	delete(fields, "tax_category")
	encoded, err := normalizeWaffoFields(t, fields)
	if err != nil {
		t.Fatalf("a config without a tax category must be accepted: %v", err)
	}
	var config paymentModel.WaffoConfig
	if err := config.Unmarshal([]byte(encoded)); err != nil {
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

// The webhook id is the gateway's: a request must not be able to point the
// method at another endpoint, and a save must keep the registered one.
func TestWaffoWebhookIDComesFromTheStoredConfig(t *testing.T) {
	fields := waffoConfigFields()
	fields["webhook_id"] = "WH_forged"
	normalized, err := normalizeWaffoFields(t, fields)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(normalized, "WH_forged") {
		t.Fatal("the webhook id must not be taken from the request")
	}
	stored := `{"merchant_id":"m","private_key":"k","store_id":"s","product_id":"p","webhook_id":"WH_1"}`
	carried, err := WithWaffoWebhookID(normalized, stored)
	if err != nil {
		t.Fatal(err)
	}
	if WaffoWebhookID(carried) != "WH_1" {
		t.Fatalf("carried config %s lost the stored webhook id", carried)
	}
	again, _ := WithWaffoWebhookID(normalized, stored)
	if again != carried {
		t.Fatal("carrying the id over must be deterministic, or every save compares as changed")
	}
}

// An unpaid checkout cannot be confirmed at the gateway, so the order waits
// until every session it could have opened has expired.
func TestWaffoReconcileWaitsForTheSessionsToExpire(t *testing.T) {
	g := &waffoGateway{}
	fresh := &order.Order{OrderNo: "o", PaymentCurrency: "USD", CreatedAt: time.Now()}
	if _, err := g.Reconcile(t.Context(), CloseRequest{Order: fresh}); err == nil {
		t.Fatal("a fresh started checkout must stay pending")
	}
	if _, err := g.Reconcile(t.Context(), CloseRequest{Order: fresh, Explicit: true}); err != nil {
		t.Fatalf("an explicit close must go through: %v", err)
	}
	old := &order.Order{OrderNo: "o", PaymentCurrency: "USD", CreatedAt: time.Now().Add(-order.UnpaidCloseAge - time.Minute)}
	if _, err := g.Reconcile(t.Context(), CloseRequest{Order: old}); err != nil {
		t.Fatalf("an abandoned checkout must close: %v", err)
	}
	never := &order.Order{OrderNo: "o", CreatedAt: time.Now()}
	if r, err := g.Reconcile(t.Context(), CloseRequest{Order: never}); err != nil || !r.RequireStableCheckout {
		t.Fatalf("a never-started checkout closes if it stays unstarted: %+v, %v", r, err)
	}
}

func TestWaffoStartPaymentRefusesAnOrderAboutToClose(t *testing.T) {
	g := &waffoGateway{}
	o := &order.Order{OrderNo: "o", CreatedAt: time.Now().Add(-order.UnpaidCloseAge + time.Minute)}
	if _, err := g.StartPayment(t.Context(), Checkout{Order: o}); err == nil {
		t.Fatal("a session shorter than the minimum must not be opened")
	}
}
