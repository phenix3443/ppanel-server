package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/cryptomus"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/subscription"
	subscribeEntity "github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
)

type cryptomusCloseTransport func(*http.Request) (*http.Response, error)

func (f cryptomusCloseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// Replace the test process's transport, never the production gateway URL or
// payment configuration. These tests must not run in parallel.
func cryptomusCloseFixture(t *testing.T, gateway func() (int, string, error)) (*closeOrderStore, *Service, *closeQueue) {
	t.Helper()
	orders := &closeOrderRepo{
		order: &orderEntity.Order{
			Id: 1, OrderNo: "cryptomus-order", Status: 1, UserId: 7,
			Method: "Cryptomus", PaymentId: 2, PaymentCurrency: "USD", PaymentAmount: 1000, TradeNo: "invoice-1",
			Type: orderTypeSubscribe, SubscribeId: 99, GiftAmount: 40,
		},
		transition: true,
	}
	store := &closeOrderStore{
		orders:     orders,
		subscribes: &closeSubscribeRepo{sub: &subscribeEntity.Subscribe{Id: 99, Inventory: 2}},
		users:      &closeUserRepo{wallet: &walletEntity.Wallet{UserId: 7, GiftAmount: 10}},
		logs:       &closeLogRepo{},
	}
	store.markReserved(t, orders.order.OrderNo)
	queue := &closeQueue{}
	svc := NewService(Deps{
		Orders: orders, Store: store, Queue: queue,
		Inventory: subscription.NewInventory(store),
		Payments: &closePaymentRepo{method: &paymentEntity.Payment{
			Id: 2, Platform: "Cryptomus", Config: `{"merchant_id":"merchant-1","api_key":"test-key"}`,
		}},
	})
	previous := http.DefaultTransport
	http.DefaultTransport = cryptomusCloseTransport(func(req *http.Request) (*http.Response, error) {
		defer req.Body.Close()
		if req.URL.String() != cryptomus.DefaultBaseURL+"/v1/payment/info" || req.Method != http.MethodPost {
			t.Fatalf("unexpected gateway request: %s %s", req.Method, req.URL)
		}
		var query map[string]string
		if err := json.NewDecoder(req.Body).Decode(&query); err != nil {
			t.Fatal(err)
		}
		if tradeNo := orders.order.TradeNo; tradeNo != "" {
			if query["uuid"] != tradeNo {
				t.Fatalf("expected lookup by claimed invoice, got %v", query)
			}
		} else if query["order_id"] != orders.order.OrderNo {
			t.Fatalf("expected recovery lookup by order number, got %v", query)
		}
		status, body, err := gateway()
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	return store, svc, queue
}

func cryptomusInvoiceBody(status string, final bool, paymentAmount string) string {
	return fmt.Sprintf(`{"state":0,"result":{"uuid":"invoice-1","order_id":"cryptomus-order","amount":"10.00","currency":"USD","status":%q,"is_final":%t,"payment_amount":%q}}`, status, final, paymentAmount)
}

func assertCryptomusReservationUnchanged(t *testing.T, store *closeOrderStore) {
	t.Helper()
	if store.users.updateCalls != 0 || store.users.wallet.GiftAmount != 10 || store.logs.insertCalls != 0 ||
		store.subscribes.updateCalls != 0 || store.subscribes.sub.Inventory != 2 {
		t.Fatal("unconfirmed or paid invoice released the order reservation")
	}
}

func TestCloseCryptomusKeepsUnconfirmedInvoicesPending(t *testing.T) {
	tests := []struct {
		name, status, paid string
		final              bool
	}{
		{"waiting", "check", "0.00", false},
		{"processing", "process", "10.00", false},
		{"confirming", "confirm_check", "10.00", false},
		{"partial payment", "wrong_amount_waiting", "5.00", false},
		{"underpaid final", "wrong_amount", "5.00", true},
		{"AML locked", "locked", "10.00", true},
		{"refund pending", "refund_process", "10.00", true},
		{"refund failed", "refund_fail", "10.00", true},
		{"refund paid", "refund_paid", "10.00", true},
		{"gateway failure", "fail", "0.00", true},
		{"system failure", "system_fail", "0.00", true},
		{"unknown final", "new_status", "0.00", true},
		{"not finally cancelled", "cancel", "0.00", false},
		{"cancelled with money", "cancel", "0.001", true},
		{"cancelled missing amount", "cancel", "", true},
		{"cancelled invalid amount", "cancel", "invalid", true},
	}
	for _, test := range tests {
		for _, userInitiated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/user=%t", test.name, userInitiated), func(t *testing.T) {
				store, svc, queue := cryptomusCloseFixture(t, func() (int, string, error) {
					return 200, cryptomusInvoiceBody(test.status, test.final, test.paid), nil
				})
				ctx := context.Background()
				if userInitiated {
					ctx = context.WithValue(ctx, requestctx.CtxKeyUser, &userEntity.User{Id: 7})
				}
				err := svc.Close(ctx, &dto.CloseOrderRequest{OrderNo: "cryptomus-order"})
				if !errors.Is(err, ErrGatewayUnconfirmed) || store.orders.order.Status != 1 || len(queue.activations) != 0 {
					t.Fatalf("must stay pending: err=%v order=%+v", err, store.orders.order)
				}
				assertCryptomusReservationUnchanged(t, store)
			})
		}
	}
}

func TestCloseCryptomusQueryErrorsNeverDiscardKnownInvoice(t *testing.T) {
	tests := []struct {
		name string
		code int
		body string
		err  error
	}{
		{"network failure", 0, "", errors.New("connection refused")},
		{"proxy 404", 404, "<html>Not found</html>", nil},
		{"merchant missing", 404, `{"state":1,"message":"Merchant not found"}`, nil},
		{"payment missing", 422, `{"state":1,"message":"Payment not found"}`, nil},
		{"server error", 500, `{"state":1,"message":"Server error"}`, nil},
	}
	for _, test := range tests {
		for _, userInitiated := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/user=%t", test.name, userInitiated), func(t *testing.T) {
				store, svc, _ := cryptomusCloseFixture(t, func() (int, string, error) { return test.code, test.body, test.err })
				ctx := context.Background()
				if userInitiated {
					ctx = context.WithValue(ctx, requestctx.CtxKeyUser, &userEntity.User{Id: 7})
				}
				if err := svc.Close(ctx, &dto.CloseOrderRequest{OrderNo: "cryptomus-order"}); !errors.Is(err, ErrGatewayUnconfirmed) {
					t.Fatalf("must reject unconfirmed close: %v", err)
				}
				if store.orders.order.Status != 1 {
					t.Fatal("query error closed the order")
				}
				assertCryptomusReservationUnchanged(t, store)
			})
		}
	}
}

func TestCloseCryptomusUnclaimedInvoiceMayStillBeCreating(t *testing.T) {
	for _, test := range []struct {
		name string
		code int
		body string
	}{
		{"currently missing payment", 422, `{"state":1,"message":"Payment not found"}`},
		{"proxy error", 404, "<html>Not found</html>"},
		{"missing merchant", 404, `{"state":1,"message":"Merchant not found"}`},
		{"generic not found", 404, `{"state":1,"message":"Not found"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, svc, _ := cryptomusCloseFixture(t, func() (int, string, error) { return test.code, test.body, nil })
			store.orders.order.TradeNo = ""
			err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "cryptomus-order"})
			if !errors.Is(err, ErrGatewayUnconfirmed) || store.orders.order.Status != 1 {
				t.Fatalf("unconfirmed invoice should stay pending: %v", err)
			}
			assertCryptomusReservationUnchanged(t, store)
		})
	}
}

func TestCloseCryptomusBeforeCheckoutCanCancel(t *testing.T) {
	store, svc, _ := cryptomusCloseFixture(t, func() (int, string, error) {
		t.Fatal("an order before checkout needs no gateway request")
		return 0, "", nil
	})
	store.orders.order.TradeNo, store.orders.order.PaymentCurrency = "", ""
	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "cryptomus-order"}); err != nil {
		t.Fatal(err)
	}
	if store.orders.order.Status != 3 {
		t.Fatal("an order before checkout should close normally")
	}
}

type cryptomusConcurrentCheckoutStore struct{ *closeOrderStore }

func (s cryptomusConcurrentCheckoutStore) InBillingTx(ctx context.Context, fn func(repository.BillingStore) error) error {
	// Simulate checkout persisting its expectation after the close request's
	// initial read but before the transaction acquires the order row lock.
	s.orders.order.PaymentCurrency = "USD"
	return s.closeOrderStore.InBillingTx(ctx, fn)
}

func TestCloseCryptomusRechecksConcurrentCheckoutInsideTransaction(t *testing.T) {
	store, svc, _ := cryptomusCloseFixture(t, func() (int, string, error) {
		t.Fatal("the initial snapshot is from before checkout")
		return 0, "", nil
	})
	store.orders.order.TradeNo, store.orders.order.PaymentCurrency = "", ""
	svc.deps.Store = cryptomusConcurrentCheckoutStore{store}
	err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "cryptomus-order"})
	if !errors.Is(err, ErrGatewayUnconfirmed) || store.orders.order.Status != 1 {
		t.Fatalf("must not close an order whose checkout just started: %v", err)
	}
	assertCryptomusReservationUnchanged(t, store)
}

func TestCloseCryptomusRequiresMatchingInvoiceBeforeCancellation(t *testing.T) {
	for _, test := range []struct{ from, to string }{
		{"invoice-1", "other-invoice"}, {"cryptomus-order", "other-order"}, {"10.00", "9.00"}, {"USD", "EUR"},
	} {
		t.Run(test.from, func(t *testing.T) {
			store, svc, _ := cryptomusCloseFixture(t, func() (int, string, error) {
				return 200, strings.Replace(cryptomusInvoiceBody("cancel", true, "0.00"), test.from, test.to, 1), nil
			})
			if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "cryptomus-order"}); !errors.Is(err, ErrGatewayUnconfirmed) {
				t.Fatalf("must reject mismatched invoice: %v", err)
			}
			assertCryptomusReservationUnchanged(t, store)
		})
	}
}

func TestCloseCryptomusCancelledUnpaidInvoiceReleasesReservation(t *testing.T) {
	store, svc, _ := cryptomusCloseFixture(t, func() (int, string, error) {
		return 200, cryptomusInvoiceBody("cancel", true, "0.00000000"), nil
	})
	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "cryptomus-order"}); err != nil {
		t.Fatal(err)
	}
	if store.orders.order.Status != 3 || store.users.wallet.GiftAmount != 50 || store.subscribes.sub.Inventory != 3 {
		t.Fatal("verified cancellation must close and release reservation")
	}
}

func TestCloseCryptomusSettlesAfterRefusingEarlyUserCancellation(t *testing.T) {
	for _, paidStatus := range []string{"paid", "paid_over"} {
		t.Run(paidStatus, func(t *testing.T) {
			status := "confirm_check"
			store, svc, queue := cryptomusCloseFixture(t, func() (int, string, error) {
				return 200, cryptomusInvoiceBody(status, status != "confirm_check", "10.00"), nil
			})
			ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &userEntity.User{Id: 7})
			req := &dto.CloseOrderRequest{OrderNo: "cryptomus-order"}
			if err := svc.Close(ctx, req); !errors.Is(err, ErrGatewayUnconfirmed) {
				t.Fatalf("must refuse cancellation while confirming: %v", err)
			}
			status = paidStatus
			if err := svc.Close(context.Background(), req); err != nil {
				t.Fatalf("reconciler must settle the late payment: %v", err)
			}
			if store.orders.order.Status != 2 || len(queue.activations) != 1 {
				t.Fatal("paid invoice must activate exactly once instead of closing")
			}
			assertCryptomusReservationUnchanged(t, store)
		})
	}
}
