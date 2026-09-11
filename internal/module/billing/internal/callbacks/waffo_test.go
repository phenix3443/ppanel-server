package callbacks

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
)

const (
	waffoTestMerchantID = "MER_2aUyqjCzEIiEcYMKj7TZtw"
	waffoTestStoreID    = "STO_2aUyqjCzEIiEcYMKj7TZtw"
	waffoTestProductID  = "PROD_2aUyqjCzEIiEcYMKj7TZtw"
)

// waffoWebhookKey installs a test signing key as the environment's webhook
// key for the duration of the test. The SDK resolves it ahead of its built-in
// gateway keys, which is also how a key rotation is rolled out in production.
func waffoWebhookKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	t.Setenv("WAFFO_WEBHOOK_TEST_PUBLIC_KEY", string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})))
	return key
}

func waffoMerchantKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

func signWaffoEvent(t *testing.T, key *rsa.PrivateKey, event map[string]interface{}) (payload []byte, header string) {
	t.Helper()
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	timestamp := time.Now().UnixMilli()
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign event: %v", err)
	}
	return payload, fmt.Sprintf("t=%d,v1=%s", timestamp, base64.StdEncoding.EncodeToString(signature))
}

func waffoEvent(eventType string, mutate func(data map[string]interface{})) map[string]interface{} {
	data := map[string]interface{}{
		"orderId": "ORD_1", "orderStatus": "completed", "buyerEmail": "buyer@example.com",
		"orderMerchantExternalId": "order-1", "currency": "USD",
		"amount": "10.00", "taxAmount": "1.00", "total": "11.00",
		"productName": "PPanel Order", "paymentId": "PAY_1",
	}
	if mutate != nil {
		mutate(data)
	}
	return map[string]interface{}{
		"id": "delivery-1", "timestamp": time.Now().UTC().Format(time.RFC3339),
		"eventType": eventType, "eventId": "PAY_1",
		"storeId": waffoTestStoreID, "storeName": "PPanel", "mode": "test",
		"data": data,
	}
}

func waffoPaymentConfig(id int64, privateKey string) *payment.Payment {
	config, _ := (&payment.WaffoConfig{
		MerchantID:  waffoTestMerchantID,
		PrivateKey:  privateKey,
		StoreID:     waffoTestStoreID,
		ProductID:   waffoTestProductID,
		TaxCategory: "saas",
		TestMode:    true,
	}).Marshal()
	return &payment.Payment{Id: id, Platform: "Waffo", Config: string(config)}
}

// waffoOrderServer stubs the GraphQL order query used to confirm a webhook.
func waffoOrderServer(t *testing.T, response string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/graphql" {
			t.Errorf("unexpected gateway path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
}

func withWaffoGateway(t *testing.T, url string) {
	t.Helper()
	previous := waffoBaseURL
	waffoBaseURL = url
	t.Cleanup(func() { waffoBaseURL = previous })
}

const waffoCompletedOrder = `{"data":{"onetimeOrder":{"id":"ORD_1","status":"completed","currency":"USD","testMode":true,"orderMerchantExternalId":"order-1","subtotal":{"amount":"1000","currency":"USD"},"taxAmount":{"amount":"100","currency":"USD"},"total":{"amount":"1100","currency":"USD"}}}}`

func waffoPendingOrder(t *testing.T, status uint8) *order.Order {
	t.Helper()
	return &order.Order{
		OrderNo: "order-1", PaymentId: 12, Method: "Waffo", Status: status,
		PaymentAmount: 1000, PaymentCurrency: "USD",
	}
}

func TestWaffoNotifySettlesOnlyAfterSignedAndQueriedOrderMatch(t *testing.T) {
	key := waffoWebhookKey(t)
	queryServer := waffoOrderServer(t, waffoCompletedOrder)
	defer queryServer.Close()
	withWaffoGateway(t, queryServer.URL)

	queue := &fakeActivationQueue{}
	orders := &callbackOrderRepo{order: waffoPendingOrder(t, settle.StatusPending)}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
	svc := NewService(orders, queue)
	payload, header := signWaffoEvent(t, key, waffoEvent("order.completed", nil))

	if err := svc.WaffoNotify(ctx, payload, header); err != nil {
		t.Fatalf("WaffoNotify: %v", err)
	}
	if err := svc.WaffoNotify(ctx, payload, header); err != nil {
		t.Fatalf("duplicate WaffoNotify must be idempotent: %v", err)
	}
	if orders.markCount != 1 || orders.order.Status != settle.StatusPaid || orders.order.TradeNo != "ORD_1" {
		t.Fatalf("order was not settled exactly once: %+v, marks=%d", orders.order, orders.markCount)
	}
	if len(queue.enqueued) == 0 {
		t.Fatal("settlement must enqueue activation")
	}
}

func TestWaffoNotifyRejectsForgedSignature(t *testing.T) {
	waffoWebhookKey(t)
	foreignKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
	svc := NewService(nil, nil)

	payload, header := signWaffoEvent(t, foreignKey, waffoEvent("order.completed", nil))
	if err := svc.WaffoNotify(ctx, payload, header); err == nil || !strings.Contains(err.Error(), "verify sign failed") {
		t.Fatalf("a payload signed with another key must be rejected, got %v", err)
	}
}

func TestWaffoNotifyRejectsTamperedPayload(t *testing.T) {
	key := waffoWebhookKey(t)
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
	svc := NewService(nil, nil)

	payload, header := signWaffoEvent(t, key, waffoEvent("order.completed", nil))
	tampered := []byte(strings.Replace(string(payload), `"10.00"`, `"1.00"`, 1))
	if err := svc.WaffoNotify(ctx, tampered, header); err == nil || !strings.Contains(err.Error(), "verify sign failed") {
		t.Fatalf("a tampered payload must be rejected, got %v", err)
	}
}

func TestWaffoNotifyRejectsEventFromTheOtherEnvironment(t *testing.T) {
	key := waffoWebhookKey(t)
	orders := &callbackOrderRepo{order: waffoPendingOrder(t, settle.StatusPending)}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
	svc := NewService(orders, &fakeActivationQueue{})

	event := waffoEvent("order.completed", nil)
	event["mode"] = "prod"
	payload, header := signWaffoEvent(t, key, event)
	if err := svc.WaffoNotify(ctx, payload, header); err == nil {
		t.Fatal("a prod-mode event must not settle a test-mode payment method")
	}
	if orders.markCount != 0 {
		t.Fatal("the order must not be settled")
	}
}

func TestWaffoNotifyAcknowledgesOtherEventsWithoutSettling(t *testing.T) {
	key := waffoWebhookKey(t)
	orders := &callbackOrderRepo{order: waffoPendingOrder(t, settle.StatusPending)}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
	svc := NewService(orders, &fakeActivationQueue{})

	payload, header := signWaffoEvent(t, key, waffoEvent("refund.succeeded", nil))
	if err := svc.WaffoNotify(ctx, payload, header); err != nil {
		t.Fatalf("an unrelated event must be acknowledged: %v", err)
	}
	if orders.markCount != 0 {
		t.Fatal("an unrelated event must not settle the order")
	}
}

func TestWaffoNotifyRejectsAmountMismatch(t *testing.T) {
	key := waffoWebhookKey(t)
	queryServer := waffoOrderServer(t, waffoCompletedOrder)
	defer queryServer.Close()
	withWaffoGateway(t, queryServer.URL)

	orders := &callbackOrderRepo{order: waffoPendingOrder(t, settle.StatusPending)}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
	svc := NewService(orders, &fakeActivationQueue{})

	// The buyer's tax-inclusive total is not what the order is priced at:
	// settling against it would accept an underpaid order elsewhere.
	payload, header := signWaffoEvent(t, key, waffoEvent("order.completed", func(data map[string]interface{}) {
		data["amount"] = "9.00"
	}))
	if err := svc.WaffoNotify(ctx, payload, header); err == nil || !strings.Contains(err.Error(), "payment amount mismatch") {
		t.Fatalf("an underpaid callback must be rejected, got %v", err)
	}
	if orders.markCount != 0 {
		t.Fatal("the order must not be settled")
	}
}

func TestWaffoNotifyRejectsUnconfirmedGatewayOrder(t *testing.T) {
	key := waffoWebhookKey(t)
	cases := map[string]string{
		"order still pending at the gateway": `{"data":{"onetimeOrder":{"id":"ORD_1","status":"pending","currency":"USD","testMode":true,"orderMerchantExternalId":"order-1","subtotal":{"amount":"1000","currency":"USD"},"taxAmount":{"amount":"100","currency":"USD"},"total":{"amount":"1100","currency":"USD"}}}}`,
		"gateway order is another order":     `{"data":{"onetimeOrder":{"id":"ORD_1","status":"completed","currency":"USD","testMode":true,"orderMerchantExternalId":"order-2","subtotal":{"amount":"1000","currency":"USD"},"taxAmount":{"amount":"100","currency":"USD"},"total":{"amount":"1100","currency":"USD"}}}}`,
		"gateway subtotal disagrees":         `{"data":{"onetimeOrder":{"id":"ORD_1","status":"completed","currency":"USD","testMode":true,"orderMerchantExternalId":"order-1","subtotal":{"amount":"100","currency":"USD"},"taxAmount":{"amount":"10","currency":"USD"},"total":{"amount":"110","currency":"USD"}}}}`,
		"gateway order is the other mode":    `{"data":{"onetimeOrder":{"id":"ORD_1","status":"completed","currency":"USD","testMode":false,"orderMerchantExternalId":"order-1","subtotal":{"amount":"1000","currency":"USD"},"taxAmount":{"amount":"100","currency":"USD"},"total":{"amount":"1100","currency":"USD"}}}}`,
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			queryServer := waffoOrderServer(t, response)
			defer queryServer.Close()
			withWaffoGateway(t, queryServer.URL)

			orders := &callbackOrderRepo{order: waffoPendingOrder(t, settle.StatusPending)}
			ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(12, waffoMerchantKey(t)))
			svc := NewService(orders, &fakeActivationQueue{})

			payload, header := signWaffoEvent(t, key, waffoEvent("order.completed", nil))
			if err := svc.WaffoNotify(ctx, payload, header); err == nil {
				t.Fatal("a signed callback the gateway does not confirm must be rejected")
			}
			if orders.markCount != 0 {
				t.Fatal("the order must not be settled")
			}
		})
	}
}

func TestWaffoNotifyRejectsCallbackForAnotherPaymentMethod(t *testing.T) {
	key := waffoWebhookKey(t)
	orders := &callbackOrderRepo{order: waffoPendingOrder(t, settle.StatusPending)}
	// The order belongs to payment method 12; the callback arrives on 13.
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, waffoPaymentConfig(13, waffoMerchantKey(t)))
	svc := NewService(orders, &fakeActivationQueue{})

	payload, header := signWaffoEvent(t, key, waffoEvent("order.completed", nil))
	if err := svc.WaffoNotify(ctx, payload, header); err == nil || !strings.Contains(err.Error(), "payment method mismatch") {
		t.Fatalf("a callback for another payment method must be rejected, got %v", err)
	}
}
