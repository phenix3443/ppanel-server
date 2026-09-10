package callbacks

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
)

func signCryptomusTestPayload(t *testing.T, apiKey string, fields map[string]interface{}) []byte {
	t.Helper()
	unsigned, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	digest := md5.Sum([]byte(base64.StdEncoding.EncodeToString(unsigned) + apiKey))
	sign := hex.EncodeToString(digest[:])
	return []byte(strings.TrimSuffix(string(unsigned), "}") + `,"sign":"` + sign + `"}`)
}

func cryptomusPaidNotification(t *testing.T, apiKey string) []byte {
	t.Helper()
	return signCryptomusTestPayload(t, apiKey, map[string]interface{}{
		"type": "payment", "uuid": "uuid-1", "order_id": "order-1",
		"amount": "10.00", "currency": "USD", "payer_currency": "USDT",
		"status": "paid", "is_final": true,
	})
}

func cryptomusInfoServer(t *testing.T, invoice string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/payment/info" {
			t.Errorf("unexpected gateway path %s", r.URL.Path)
		}
		fmt.Fprint(w, invoice)
	}))
}

func cryptomusPaymentConfig(id int64, apiKey string) *payment.Payment {
	return &payment.Payment{
		Id:       id,
		Platform: "Cryptomus",
		Config:   `{"merchant_id":"merchant-1","api_key":"` + apiKey + `"}`,
	}
}

// withCryptomusGateway redirects the confirmation query to a test stub. The
// production redirect knob was removed on purpose; the base URL is not part
// of the database payment configuration.
func withCryptomusGateway(t *testing.T, url string) {
	t.Helper()
	previous := cryptomusBaseURL
	cryptomusBaseURL = url
	t.Cleanup(func() { cryptomusBaseURL = previous })
}

func TestCryptomusNotifySettlesOnlyAfterSignedAndQueriedInvoiceMatch(t *testing.T) {
	queryServer := cryptomusInfoServer(t,
		`{"state":0,"result":{"uuid":"uuid-1","order_id":"order-1","amount":"10.00","currency":"USD","payment_status":"paid","is_final":true}}`)
	defer queryServer.Close()

	queue := &fakeActivationQueue{}
	orders := &callbackOrderRepo{order: &order.Order{
		OrderNo: "order-1", PaymentId: 11, Method: "Cryptomus", Status: settle.StatusPending,
		PaymentAmount: 1000, PaymentCurrency: "USD",
	}}
	withCryptomusGateway(t, queryServer.URL)
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
	svc := NewService(orders, queue)
	payload := cryptomusPaidNotification(t, "api-key")

	if err := svc.CryptomusNotify(ctx, payload); err != nil {
		t.Fatalf("CryptomusNotify: %v", err)
	}
	if err := svc.CryptomusNotify(ctx, payload); err != nil {
		t.Fatalf("duplicate CryptomusNotify must be idempotent: %v", err)
	}
	if orders.markCount != 1 || orders.order.Status != settle.StatusPaid || orders.order.TradeNo != "uuid-1" {
		t.Fatalf("order was not settled exactly once: %+v, marks=%d", orders.order, orders.markCount)
	}
	if len(queue.enqueued) == 0 {
		t.Fatal("settlement must enqueue activation")
	}
}

func TestCryptomusNotifyRejectsInvalidSignature(t *testing.T) {
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
	svc := NewService(nil, nil)

	payload := cryptomusPaidNotification(t, "wrong-key")
	if err := svc.CryptomusNotify(ctx, payload); err == nil || !strings.Contains(err.Error(), "verify sign failed") {
		t.Fatalf("payload signed with another key must be rejected, got %v", err)
	}

	tampered := []byte(strings.Replace(string(cryptomusPaidNotification(t, "api-key")), `"10.00"`, `"1.00"`, 1))
	if err := svc.CryptomusNotify(ctx, tampered); err == nil || !strings.Contains(err.Error(), "verify sign failed") {
		t.Fatalf("tampered payload must be rejected, got %v", err)
	}
}

func TestCryptomusNotifyRejectsWrongTypeAndAmountMismatch(t *testing.T) {
	orders := &callbackOrderRepo{order: &order.Order{
		OrderNo: "order-1", PaymentId: 11, Method: "Cryptomus", Status: settle.StatusPending,
		PaymentAmount: 1000, PaymentCurrency: "USD",
	}}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
	svc := NewService(orders, nil)

	walletTopup := signCryptomusTestPayload(t, "api-key", map[string]interface{}{
		"type": "wallet", "uuid": "uuid-1", "order_id": "order-1",
		"amount": "10.00", "currency": "USD", "status": "paid", "is_final": true,
	})
	if err := svc.CryptomusNotify(ctx, walletTopup); err == nil || !strings.Contains(err.Error(), "notification type") {
		t.Fatalf("wallet webhooks must not settle orders, got %v", err)
	}

	underpaid := signCryptomusTestPayload(t, "api-key", map[string]interface{}{
		"type": "payment", "uuid": "uuid-1", "order_id": "order-1",
		"amount": "9.00", "currency": "USD", "status": "paid", "is_final": true,
	})
	if err := svc.CryptomusNotify(ctx, underpaid); err == nil || !strings.Contains(err.Error(), "amount mismatch") {
		t.Fatalf("amount below the payment expectation must be rejected, got %v", err)
	}

	wrongCurrency := signCryptomusTestPayload(t, "api-key", map[string]interface{}{
		"type": "payment", "uuid": "uuid-1", "order_id": "order-1",
		"amount": "10.00", "currency": "EUR", "status": "paid", "is_final": true,
	})
	if err := svc.CryptomusNotify(ctx, wrongCurrency); err == nil || !strings.Contains(err.Error(), "currency mismatch") {
		t.Fatalf("currency mismatch must be rejected, got %v", err)
	}
}

func TestCryptomusNotifyAcknowledgesNonPaidStatusesWithoutSettlement(t *testing.T) {
	statuses := []string{"check", "process", "confirm_check", "wrong_amount_waiting", "wrong_amount",
		"cancel", "fail", "system_fail", "locked", "refund_process", "refund_fail", "refund_paid"}
	for _, status := range statuses {
		for _, localStatus := range []uint8{settle.StatusPending, settle.StatusPaid, settle.StatusFinished, 3} {
			t.Run(fmt.Sprintf("%s/local=%d", status, localStatus), func(t *testing.T) {
				orders := &callbackOrderRepo{order: &order.Order{
					OrderNo: "order-1", TradeNo: "uuid-1", PaymentId: 11, Method: "Cryptomus", Status: localStatus,
					PaymentAmount: 1000, PaymentCurrency: "USD",
				}}
				queue := &fakeActivationQueue{}
				svc := NewService(orders, queue)
				ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
				payload := signCryptomusTestPayload(t, "api-key", map[string]interface{}{
					"type": "payment", "uuid": "uuid-1", "order_id": "order-1",
					"amount": "10.00000000", "currency": "USD", "status": status,
				})
				// Non-settling events must not query the gateway or enqueue work.
				withCryptomusGateway(t, ":invalid-gateway")
				for range 2 {
					if err := svc.CryptomusNotify(ctx, payload); err != nil {
						t.Fatalf("valid lifecycle event must be acknowledged: %v", err)
					}
				}
				if orders.order.Status != localStatus || orders.markCount != 0 || len(queue.enqueued) != 0 {
					t.Fatalf("non-paid event mutated the order: %+v", orders.order)
				}
			})
		}
	}
}

func TestCryptomusNonPaidNotificationStillRequiresAuthenticationAndBinding(t *testing.T) {
	tests := []struct {
		name, field, value, want string
	}{
		{"bad signature", "sign", strings.Repeat("0", 32), "verify sign failed"},
		{"another invoice", "uuid", "uuid-2", "trade number mismatch"},
		{"another order", "order_id", "other-order", "order not exist"},
		{"wrong amount", "amount", "9.00", "amount mismatch"},
		{"wrong currency", "currency", "EUR", "currency mismatch"},
		{"wrong type", "type", "wallet", "notification type"},
		{"unknown status", "status", "invented_status", "unknown payment status"},
		{"empty status", "status", "", "unknown payment status"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			orders := &callbackOrderRepo{order: &order.Order{
				OrderNo: "order-1", TradeNo: "uuid-1", PaymentId: 11, Method: "Cryptomus", Status: settle.StatusPending,
				PaymentAmount: 1000, PaymentCurrency: "USD",
			}}
			fields := map[string]interface{}{
				"type": "payment", "uuid": "uuid-1", "order_id": "order-1",
				"amount": "10.00", "currency": "USD", "status": "confirm_check",
			}
			fields[test.field] = test.value
			ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
			err := NewService(orders, nil).CryptomusNotify(ctx, signCryptomusTestPayload(t, "api-key", fields))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
			if orders.markCount != 0 || orders.order.Status != settle.StatusPending {
				t.Fatal("rejected notification changed the order")
			}
		})
	}
}

func TestCryptomusNotifyRejectsWhenGatewayDisagrees(t *testing.T) {
	tests := []struct {
		name    string
		invoice string
		want    string
	}{
		{
			name:    "unpaid at gateway",
			invoice: `{"state":0,"result":{"uuid":"uuid-1","order_id":"order-1","amount":"10.00","currency":"USD","status":"process","is_final":false}}`,
			want:    "not paid",
		},
		{
			name:    "identity mismatch",
			invoice: `{"state":0,"result":{"uuid":"uuid-2","order_id":"order-1","amount":"10.00","currency":"USD","status":"paid","is_final":true}}`,
			want:    "identity mismatch",
		},
		{
			name:    "amount mismatch",
			invoice: `{"state":0,"result":{"uuid":"uuid-1","order_id":"order-1","amount":"9.00","currency":"USD","status":"paid","is_final":true}}`,
			want:    "amount mismatch",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			queryServer := cryptomusInfoServer(t, test.invoice)
			defer queryServer.Close()

			orders := &callbackOrderRepo{order: &order.Order{
				OrderNo: "order-1", PaymentId: 11, Method: "Cryptomus", Status: settle.StatusPending,
				PaymentAmount: 1000, PaymentCurrency: "USD",
			}}
			withCryptomusGateway(t, queryServer.URL)
			ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
			svc := NewService(orders, &fakeActivationQueue{})

			err := svc.CryptomusNotify(ctx, cryptomusPaidNotification(t, "api-key"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q error, got %v", test.want, err)
			}
			if orders.markCount != 0 {
				t.Fatal("order must not settle when the gateway disagrees")
			}
		})
	}
}

func TestCryptomusNotifyRequiresExactPaymentBinding(t *testing.T) {
	orders := &callbackOrderRepo{order: &order.Order{
		OrderNo: "order-1", PaymentId: 12, Method: "EPay", Status: settle.StatusPending,
		PaymentAmount: 1000, PaymentCurrency: "USD",
	}}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyPayment, cryptomusPaymentConfig(11, "api-key"))
	svc := NewService(orders, nil)

	err := svc.CryptomusNotify(ctx, cryptomusPaidNotification(t, "api-key"))
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("order bound to another payment method must be rejected, got %v", err)
	}
}
