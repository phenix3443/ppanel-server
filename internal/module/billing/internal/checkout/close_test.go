package checkout

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	orderEntity "github.com/perfect-panel/server/internal/module/billing/entity/order"
	paymentEntity "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	inboxEntity "github.com/perfect-panel/server/internal/module/platform/entity/inbox"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription"
	subscribeEntity "github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type closeOrderStore struct {
	repository.Store
	orders     *closeOrderRepo
	subscribes *closeSubscribeRepo
	users      *closeUserRepo
	logs       *closeLogRepo
	inbox      *closeInboxRepo
}

func (s *closeOrderStore) InTx(_ context.Context, fn func(repository.Store) error) error {
	return fn(s)
}

func (s *closeOrderStore) InBillingTx(_ context.Context, fn func(repository.BillingStore) error) error {
	return fn(s)
}

func (s *closeOrderStore) InSubscriptionTx(_ context.Context, fn func(repository.SubscriptionStore) error) error {
	return fn(s)
}

func (s *closeOrderStore) Wallet() repository.WalletRepo { return s.users }
func (s *closeOrderStore) Order() repository.OrderRepo   { return s.orders }
func (s *closeOrderStore) Subscribe() repository.SubscribeRepo {
	return s.subscribes
}
func (s *closeOrderStore) Log() repository.LogRepo { return s.logs }
func (s *closeOrderStore) Inbox() repository.InboxRepo {
	if s.inbox == nil {
		s.inbox = &closeInboxRepo{records: map[string]string{}}
	}
	return s.inbox
}

// newCloseService wires the checkout service against the fake store; only the
// dependencies the close flow touches are provided.
func newCloseService(store *closeOrderStore) *Service {
	return NewService(Deps{
		Orders:    store.orders,
		Payments:  nil, // gateway settlement is not exercised: fake orders carry no gateway method
		Store:     store,
		Inventory: subscription.NewInventory(store),
	})
}

type closeInboxRepo struct {
	repository.InboxRepo
	records map[string]string
}

func (r *closeInboxRepo) Find(_ context.Context, consumer, key string) (*inboxEntity.Record, error) {
	result, ok := r.records[consumer+"|"+key]
	if !ok {
		return nil, nil
	}
	return &inboxEntity.Record{Consumer: consumer, EventKey: key, Result: result}, nil
}

func (r *closeInboxRepo) Insert(_ context.Context, consumer, key, result string) error {
	k := consumer + "|" + key
	if _, ok := r.records[k]; ok {
		return fmt.Errorf("duplicate inbox record %s", k)
	}
	r.records[k] = result
	return nil
}

// markReserved seeds the inbox as if the purchase flow had reserved inventory
// for the order (the new-flow invariant for pending subscribe orders).
func (s *closeOrderStore) markReserved(t *testing.T, orderNo string) {
	t.Helper()
	if err := s.Inbox().Insert(context.Background(), subscription.InventoryReserveConsumer, orderNo, ""); err != nil {
		t.Fatalf("seed reserve marker: %v", err)
	}
}

type closeOrderRepo struct {
	repository.OrderRepo
	order       *orderEntity.Order
	transition  bool
	from        uint8
	to          uint8
	deleteCalls int
}

func (r *closeOrderRepo) FindOneByOrderNo(_ context.Context, orderNo string) (*orderEntity.Order, error) {
	if orderNo != r.order.OrderNo {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.order
	return &copy, nil
}

func (r *closeOrderRepo) FindOneByOrderNoForUpdate(ctx context.Context, orderNo string) (*orderEntity.Order, error) {
	return r.FindOneByOrderNo(ctx, orderNo)
}

func (r *closeOrderRepo) UpdateOrderStatusFrom(_ context.Context, orderNo string, from, to uint8, _ ...*gorm.DB) (bool, error) {
	r.from, r.to = from, to
	if orderNo != r.order.OrderNo || !r.transition {
		return false, nil
	}
	r.order.Status = to
	return true, nil
}

func (r *closeOrderRepo) Delete(_ context.Context, _ int64, _ ...*gorm.DB) error {
	r.deleteCalls++
	return nil
}

type closeSubscribeRepo struct {
	repository.SubscribeRepo
	sub         *subscribeEntity.Subscribe
	updateCalls int
}

func (r *closeSubscribeRepo) FindOne(_ context.Context, id int64) (*subscribeEntity.Subscribe, error) {
	if r.sub == nil || id != r.sub.Id {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.sub
	return &copy, nil
}

func (r *closeSubscribeRepo) RestoreInventory(_ context.Context, id int64, _ ...*gorm.DB) error {
	if r.sub == nil || r.sub.Id != id {
		return gorm.ErrRecordNotFound
	}
	if r.sub.Inventory != -1 {
		r.sub.Inventory++
	}
	r.updateCalls++
	return nil
}

type closeUserRepo struct {
	repository.WalletRepo
	wallet      *walletEntity.Wallet
	updateCalls int
}

func (r *closeUserRepo) FindOneForUpdate(_ context.Context, id int64) (*walletEntity.Wallet, error) {
	if r.wallet == nil || id != r.wallet.UserId {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *r.wallet
	return &copy, nil
}

func (r *closeUserRepo) UpdateBalanceFields(_ context.Context, value *walletEntity.Wallet, _ ...*gorm.DB) error {
	r.updateCalls++
	r.wallet.Balance = value.Balance
	r.wallet.GiftAmount = value.GiftAmount
	return nil
}

type closeLogRepo struct {
	repository.LogRepo
	insertCalls int
}

func (r *closeLogRepo) Insert(_ context.Context, _ *logEntity.SystemLog) error {
	r.insertCalls++
	return nil
}

type closePaymentRepo struct {
	repository.PaymentRepo
	method *paymentEntity.Payment
}

func (r *closePaymentRepo) FindOne(_ context.Context, _ int64) (*paymentEntity.Payment, error) {
	return r.method, nil
}

func epayCloseFixture(gatewayURL string) (*closeOrderStore, *Service) {
	orders := &closeOrderRepo{
		order: &orderEntity.Order{
			Id: 1, OrderNo: "epay-order", Status: 1, UserId: 7,
			Method: "EPay", PaymentId: 2, PaymentCurrency: "CNY", PaymentAmount: 1000,
		},
		transition: true,
	}
	store := &closeOrderStore{orders: orders}
	svc := NewService(Deps{
		Orders: orders,
		Payments: &closePaymentRepo{method: &paymentEntity.Payment{
			Id: 2, Platform: "EPay",
			Config: fmt.Sprintf(`{"pid":"1001","url":%q,"key":"secret","type":"alipay"}`, gatewayURL),
		}},
		Store:     store,
		Inventory: subscription.NewInventory(store),
	})
	return store, svc
}

func (r *closeOrderRepo) MarkOrderPaid(_ context.Context, orderNo, tradeNo string, _ ...*gorm.DB) (bool, error) {
	if orderNo != r.order.OrderNo || r.order.Status != 1 {
		return false, nil
	}
	r.order.Status = 2
	r.order.TradeNo = tradeNo
	return true, nil
}

type closeQueue struct {
	activations []string
}

func (q *closeQueue) EnqueueActivation(_ context.Context, orderNo string) error {
	q.activations = append(q.activations, orderNo)
	return nil
}

func (q *closeQueue) EnqueueDeferredClose(_ context.Context, _ string) error { return nil }

// alipayTestKey is generated once: every fixture shares one merchant/gateway
// keypair, which only exists so the SDK's signing and verification succeed.
var (
	alipayTestKeyOnce sync.Once
	alipayTestKey     *rsa.PrivateKey
)

func alipayKeys(t *testing.T) (*rsa.PrivateKey, string, string) {
	t.Helper()
	alipayTestKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate RSA key: %v", err)
		}
		alipayTestKey = key
	})
	private := base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(alipayTestKey))
	publicDER, err := x509.MarshalPKIXPublicKey(&alipayTestKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	return alipayTestKey, private, base64.StdEncoding.EncodeToString(publicDER)
}

// fakeAlipayGateway impersonates the Alipay OpenAPI endpoint. respond returns
// the biz-content JSON for the given method and per-method call count; signed
// responses carry an RSA signature over the biz bytes exactly like the real
// gateway, unsigned ones mimic gateway business failures.
type fakeAlipayGateway struct {
	t       *testing.T
	key     *rsa.PrivateKey
	respond func(method string, call int) (biz string, signed bool)

	mu         sync.Mutex
	queryCalls int
	closeCalls int
}

func (g *fakeAlipayGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		g.t.Errorf("gateway: parse form: %v", err)
		return
	}
	method := r.Form.Get("method")
	g.mu.Lock()
	var call int
	switch method {
	case "alipay.trade.query":
		g.queryCalls++
		call = g.queryCalls
	case "alipay.trade.close":
		g.closeCalls++
		call = g.closeCalls
	default:
		g.t.Errorf("gateway: unexpected method %q", method)
	}
	g.mu.Unlock()

	biz, signed := g.respond(method, call)
	field := strings.ReplaceAll(method, ".", "_") + "_response"
	if !signed {
		_, _ = fmt.Fprintf(w, `{%q:%s}`, field, biz)
		return
	}
	hashed := sha256.Sum256([]byte(biz))
	signature, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, hashed[:])
	if err != nil {
		g.t.Errorf("gateway: sign response: %v", err)
		return
	}
	_, _ = fmt.Fprintf(w, `{%q:%s,"sign":%q}`, field, biz, base64.StdEncoding.EncodeToString(signature))
}

func (g *fakeAlipayGateway) closed() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closeCalls
}

// alipayCloseFixture wires a pending ¥10.00 AlipayF2F order against a fake
// signed gateway; gatewayURL overrides the fake gateway for unreachable-host
// scenarios.
func alipayCloseFixture(t *testing.T, respond func(method string, call int) (string, bool), gatewayURL string) (*closeOrderStore, *Service, *fakeAlipayGateway, *closeQueue) {
	t.Helper()
	key, private, public := alipayKeys(t)
	gateway := &fakeAlipayGateway{t: t, key: key, respond: respond}
	if gatewayURL == "" {
		server := httptest.NewServer(gateway)
		t.Cleanup(server.Close)
		gatewayURL = server.URL
	}
	orders := &closeOrderRepo{
		order: &orderEntity.Order{
			Id: 1, OrderNo: "alipay-order", Status: 1, UserId: 7,
			Method: "AlipayF2F", PaymentId: 3, PaymentCurrency: "CNY", PaymentAmount: 1000,
		},
		transition: true,
	}
	store := &closeOrderStore{orders: orders}
	queue := &closeQueue{}
	svc := NewService(Deps{
		Orders: orders,
		Payments: &closePaymentRepo{method: &paymentEntity.Payment{
			Id: 3, Platform: "AlipayF2F",
			Config: fmt.Sprintf(`{"app_id":"2021000000000000","private_key":%q,"public_key":%q,"sandbox":true,"gateway":%q}`, private, public, gatewayURL),
		}},
		Store:     store,
		Inventory: subscription.NewInventory(store),
		Queue:     queue,
	})
	return store, svc, gateway, queue
}

func alipayPaidBiz(amount string) string {
	return `{"code":"10000","msg":"Success","trade_no":"2026080222001430000000000001","out_trade_no":"alipay-order","trade_status":"TRADE_SUCCESS","total_amount":"` + amount + `"}`
}

const (
	alipayWaitBiz     = `{"code":"10000","msg":"Success","trade_no":"2026080222001430000000000001","out_trade_no":"alipay-order","trade_status":"WAIT_BUYER_PAY","total_amount":"10.00"}`
	alipayNotExistBiz = `{"code":"40004","msg":"Business Failed","sub_code":"ACQ.TRADE_NOT_EXIST","sub_msg":"trade not exist"}`
	alipayClosedOkBiz = `{"code":"10000","msg":"Success","out_trade_no":"alipay-order","trade_no":"2026080222001430000000000001"}`
	alipayCloseErrBiz = `{"code":"40004","msg":"Business Failed","sub_code":"ACQ.TRADE_STATUS_ERROR","sub_msg":"trade status error"}`
)

// The regression behind this test lost real money: a paid trade whose async
// notification never arrived was silently cancelled by the expiry reconciler.
// Closing must first ask the gateway and settle a trade it reports as paid.
func TestCloseAlipayOrderSettlesPaidTradeWhenCallbackWasLost(t *testing.T) {
	store, svc, gateway, queue := alipayCloseFixture(t, func(method string, _ int) (string, bool) {
		if method != "alipay.trade.query" {
			t.Errorf("unexpected gateway call %q", method)
		}
		return alipayPaidBiz("10.00"), true
	}, "")

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "alipay-order"}); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.orders.order.Status != 2 {
		t.Fatalf("status = %d, want paid", store.orders.order.Status)
	}
	if store.orders.order.TradeNo != "2026080222001430000000000001" {
		t.Fatalf("tradeNo = %q, want the gateway trade number", store.orders.order.TradeNo)
	}
	if len(queue.activations) != 1 || queue.activations[0] != "alipay-order" {
		t.Fatalf("activations = %v, want the settled order enqueued once", queue.activations)
	}
	if gateway.closed() != 0 {
		t.Fatal("a paid trade must never be closed at the gateway")
	}
}

// A paid trade whose signed query response does not match the persisted
// payment expectation must neither settle nor close; it stays pending for
// manual resolution.
func TestCloseAlipayOrderRejectsMismatchedPaidTrade(t *testing.T) {
	store, svc, _, queue := alipayCloseFixture(t, func(string, int) (string, bool) {
		return alipayPaidBiz("9.00"), true
	}, "")

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "alipay-order"}); err == nil {
		t.Fatal("Close accepted a paid trade with a mismatched amount")
	}
	if store.orders.order.Status != 1 {
		t.Fatalf("status = %d, want still pending", store.orders.order.Status)
	}
	if len(queue.activations) != 0 {
		t.Fatal("mismatched trade must not activate the order")
	}
}

// A face-to-face trade only exists once the buyer scans the QR code, so the
// gateway reporting no trade proves no money was collected and the order may
// close without a gateway-side cancellation.
func TestCloseAlipayOrderClosesWhenQRWasNeverScanned(t *testing.T) {
	store, svc, gateway, _ := alipayCloseFixture(t, func(string, int) (string, bool) {
		return alipayNotExistBiz, false
	}, "")

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "alipay-order"}); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.orders.order.Status != 3 {
		t.Fatalf("status = %d, want closed", store.orders.order.Status)
	}
	if gateway.closed() != 0 {
		t.Fatal("no gateway close expected for a trade that never existed")
	}
}

// A scanned-but-unpaid trade keeps a payable QR code alive, so the local
// close must first void the trade at the gateway.
func TestCloseAlipayOrderVoidsScannedUnpaidTradeAtGateway(t *testing.T) {
	store, svc, gateway, _ := alipayCloseFixture(t, func(method string, _ int) (string, bool) {
		if method == "alipay.trade.close" {
			return alipayClosedOkBiz, true
		}
		return alipayWaitBiz, true
	}, "")

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "alipay-order"}); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.orders.order.Status != 3 {
		t.Fatalf("status = %d, want closed", store.orders.order.Status)
	}
	if gateway.closed() != 1 {
		t.Fatalf("gateway close calls = %d, want 1", gateway.closed())
	}
}

// A payment can land between the status query and the gateway close; the
// rejected close triggers one requery, and the now-paid trade settles instead
// of being cancelled.
func TestCloseAlipayOrderSettlesPaymentThatRacesGatewayClose(t *testing.T) {
	store, svc, _, queue := alipayCloseFixture(t, func(method string, call int) (string, bool) {
		if method == "alipay.trade.close" {
			return alipayCloseErrBiz, false
		}
		if call == 1 {
			return alipayWaitBiz, true
		}
		return alipayPaidBiz("10.00"), true
	}, "")

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "alipay-order"}); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.orders.order.Status != 2 {
		t.Fatalf("status = %d, want paid", store.orders.order.Status)
	}
	if len(queue.activations) != 1 {
		t.Fatalf("activations = %v, want the settled order enqueued once", queue.activations)
	}
}

// Without gateway confirmation the reconciler must keep the order pending;
// the sentinel lets schedulers treat the refusal as an expected outcome.
func TestCloseAlipayOrderReconcilerStaysStrict(t *testing.T) {
	store, svc, _, _ := alipayCloseFixture(t, nil, unreachableGatewayURL())

	err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "alipay-order"})
	if !stderrors.Is(err, ErrGatewayUnconfirmed) {
		t.Fatalf("Close error = %v, want ErrGatewayUnconfirmed", err)
	}
	if store.orders.order.Status != 1 {
		t.Fatalf("status = %d, want still pending", store.orders.order.Status)
	}
}

// The order's owner explicitly gives up the order, which consents to
// forfeiting an unconfirmed payment; an unreachable gateway must not trap
// the user's own cancellation.
func TestCloseAlipayOrderUserCancelBypassesUnconfirmedGateway(t *testing.T) {
	store, svc, _, _ := alipayCloseFixture(t, nil, unreachableGatewayURL())
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &userEntity.User{Id: 7})

	if err := svc.Close(ctx, &dto.CloseOrderRequest{OrderNo: "alipay-order"}); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if store.orders.order.Status != 3 {
		t.Fatalf("status = %d, want closed", store.orders.order.Status)
	}
}

// unreachableGatewayURL returns a URL on a port that refuses connections
// immediately, so query failures do not wait out the client timeout.
func unreachableGatewayURL() string {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	return server.URL
}

// The order's owner explicitly gives up the order, so a gateway that reports
// unpaid — or cannot be confirmed at all — must not block the cancellation.
func TestCloseEPayOrderUserCancelBypassesUnconfirmedGateway(t *testing.T) {
	unpaidGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":1,"msg":"ok","trade_no":"","out_trade_no":"epay-order","type":"alipay","money":"10.00","pid":"1001","status":0}`))
	}))
	defer unpaidGateway.Close()

	tests := []struct {
		name       string
		gatewayURL string
	}{
		{name: "gateway reports unpaid", gatewayURL: unpaidGateway.URL},
		{name: "gateway unreachable", gatewayURL: unreachableGatewayURL()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, svc := epayCloseFixture(tt.gatewayURL)
			ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &userEntity.User{Id: 7})

			if err := svc.Close(ctx, &dto.CloseOrderRequest{OrderNo: "epay-order"}); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if store.orders.order.Status != 3 {
				t.Fatalf("status = %d, want closed", store.orders.order.Status)
			}
		})
	}
}

// Without an explicit owner request (queue reconciler context), an EPay order
// that cannot be confirmed as paid keeps its pending reservation, and the
// error carries the sentinel so schedulers can treat it as expected.
func TestCloseEPayOrderReconcilerStaysStrict(t *testing.T) {
	store, svc := epayCloseFixture(unreachableGatewayURL())

	err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "epay-order"})
	if !stderrors.Is(err, ErrGatewayUnconfirmed) {
		t.Fatalf("Close error = %v, want ErrGatewayUnconfirmed", err)
	}
	if store.orders.order.Status != 1 {
		t.Fatalf("status = %d, want still pending", store.orders.order.Status)
	}
}

func TestCloseOrderDoesNotOverwriteConcurrentPayment(t *testing.T) {
	orders := &closeOrderRepo{
		order:      &orderEntity.Order{Id: 1, OrderNo: "order-1", Status: 1},
		transition: false, // callback already transitioned Pending -> Paid
	}
	svc := newCloseService(&closeOrderStore{orders: orders})

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "order-1"}); err != nil {
		t.Fatalf("CloseOrder: %v", err)
	}
	if orders.from != 1 || orders.to != 3 {
		t.Fatalf("expected conditional Pending -> Closed transition, got %d -> %d", orders.from, orders.to)
	}
	if orders.deleteCalls != 0 {
		t.Fatal("guest order was deleted after conditional close lost the race")
	}
}

func TestCloseOrderRetainsGuestOrderAndRestoresInventory(t *testing.T) {
	orders := &closeOrderRepo{
		order:      &orderEntity.Order{Id: 1, OrderNo: "guest-order", Type: 1, SubscribeId: 99, Status: 1},
		transition: true,
	}
	subscribes := &closeSubscribeRepo{sub: &subscribeEntity.Subscribe{Id: 99, Inventory: 2}}
	store := &closeOrderStore{orders: orders, subscribes: subscribes}
	store.markReserved(t, "guest-order")
	svc := newCloseService(store)

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "guest-order"}); err != nil {
		t.Fatalf("CloseOrder: %v", err)
	}
	if orders.order.Status != 3 {
		t.Fatalf("expected closed status, got %d", orders.order.Status)
	}
	if orders.deleteCalls != 0 {
		t.Fatal("closed guest order must be retained for audit")
	}
	if subscribes.updateCalls != 1 || subscribes.sub.Inventory != 3 {
		t.Fatalf("expected guest close to restore inventory once, calls=%d inventory=%d", subscribes.updateCalls, subscribes.sub.Inventory)
	}
}

func TestCloseOrderRefundsGiftAndRestoresInventory(t *testing.T) {
	orders := &closeOrderRepo{
		order:      &orderEntity.Order{Id: 1, OrderNo: "gift-order", Type: 1, UserId: 7, GiftAmount: 40, SubscribeId: 99, Status: 1},
		transition: true,
	}
	subscribes := &closeSubscribeRepo{sub: &subscribeEntity.Subscribe{Id: 99, Inventory: 2}}
	users := &closeUserRepo{wallet: &walletEntity.Wallet{UserId: 7, GiftAmount: 10}}
	logs := &closeLogRepo{}
	store := &closeOrderStore{orders: orders, subscribes: subscribes, users: users, logs: logs}
	store.markReserved(t, "gift-order")
	svc := newCloseService(store)

	if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "gift-order"}); err != nil {
		t.Fatalf("CloseOrder: %v", err)
	}
	if users.updateCalls != 1 || users.wallet.GiftAmount != 50 || logs.insertCalls != 1 {
		t.Fatalf("expected gift refund and log, updates=%d balance=%d logs=%d", users.updateCalls, users.wallet.GiftAmount, logs.insertCalls)
	}
	if subscribes.updateCalls != 1 || subscribes.sub.Inventory != 3 {
		t.Fatalf("expected inventory restoration after gift refund, calls=%d inventory=%d", subscribes.updateCalls, subscribes.sub.Inventory)
	}
}

func TestCloseOrderDoesNotRestoreInventoryForRenewalOrTrafficReset(t *testing.T) {
	for _, orderType := range []uint8{2, 3} {
		t.Run(fmt.Sprintf("type=%d", orderType), func(t *testing.T) {
			orders := &closeOrderRepo{
				order:      &orderEntity.Order{Id: 1, OrderNo: "existing-subscription-order", Type: orderType, SubscribeId: 99, Status: 1},
				transition: true,
			}
			subscribes := &closeSubscribeRepo{sub: &subscribeEntity.Subscribe{Id: 99, Inventory: 2}}
			svc := newCloseService(&closeOrderStore{orders: orders, subscribes: subscribes})

			if err := svc.Close(context.Background(), &dto.CloseOrderRequest{OrderNo: "existing-subscription-order"}); err != nil {
				t.Fatalf("CloseOrder: %v", err)
			}
			if orders.order.Status != 3 {
				t.Fatalf("status = %d, want closed", orders.order.Status)
			}
			if subscribes.updateCalls != 0 || subscribes.sub.Inventory != 2 {
				t.Fatalf("renewal/reset close must not restore inventory, calls=%d inventory=%d", subscribes.updateCalls, subscribes.sub.Inventory)
			}
		})
	}
}
