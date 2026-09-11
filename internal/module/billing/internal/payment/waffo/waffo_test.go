package waffo

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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Gateway short ids are validated client-side as PREFIX_ plus 22 base62
// characters, so tests cannot use placeholder ids.
const (
	testMerchantID = "MER_2aUyqjCzEIiEcYMKj7TZtw"
	testStoreID    = "STO_2aUyqjCzEIiEcYMKj7TZtw"
	testProductID  = "PROD_2aUyqjCzEIiEcYMKj7TZtw"
)

// testKeys generates a merchant key pair: the private half signs API
// requests, the public half stands in for the gateway's webhook key.
func testKeys(t *testing.T) (privatePEM, publicPEM string, key *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	privatePEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}))
	publicPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	return privatePEM, publicPEM, key
}

// signedWebhook builds a payload the way the gateway does: an RSA-SHA256
// signature over "<timestamp>.<raw body>".
func signedWebhook(t *testing.T, key *rsa.PrivateKey, event map[string]interface{}) (payload []byte, header string) {
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

// mustWrite fails the test instead of silently truncating a stub response.
func mustWrite(t *testing.T, w io.Writer, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write stub response: %v", err)
	}
}

func orderCompletedEvent(orderNo string) map[string]interface{} {
	return map[string]interface{}{
		"id": "delivery-1", "timestamp": time.Now().UTC().Format(time.RFC3339),
		"eventType": EventOrderCompleted, "eventId": "PAY_1",
		"storeId": testStoreID, "storeName": "PPanel", "mode": "test",
		"data": map[string]interface{}{
			"orderId": "ORD_1", "orderStatus": "completed", "buyerEmail": "buyer@example.com",
			"orderMerchantExternalId": orderNo, "currency": "USD",
			"amount": "10.00", "taxAmount": "1.00", "total": "11.00",
			"productName": "PPanel Order", "paymentId": "PAY_1",
		},
	}
}

func TestFormatMoney(t *testing.T) {
	cases := map[int64]string{0: "0.00", 5: "0.05", 199: "1.99", 100000: "1000.00", -1: "0.00"}
	for amount, want := range cases {
		if got := FormatMoney(amount); got != want {
			t.Fatalf("FormatMoney(%d)=%s, want %s", amount, got, want)
		}
	}
}

func TestParseMoneyRejectsUnrepresentableAmounts(t *testing.T) {
	if _, err := ParseMoney("10.001"); err == nil {
		t.Fatal("an amount below minor-unit precision must be rejected")
	}
	amount, err := ParseMoney("10.00")
	if err != nil || amount != 1000 {
		t.Fatalf("ParseMoney(10.00)=%d, %v", amount, err)
	}
}

// The webhook and the GraphQL API spell the same amount differently, so the
// two parsers must not be interchangeable.
func TestParseMinorUnitsRejectsDisplayAmounts(t *testing.T) {
	amount, err := ParseMinorUnits("1234")
	if err != nil || amount != 1234 {
		t.Fatalf("ParseMinorUnits(1234)=%d, %v", amount, err)
	}
	if _, err := ParseMinorUnits("12.34"); err == nil {
		t.Fatal("a decimal amount is a webhook amount, not a GraphQL one, and must be rejected")
	}
	if _, err := ParseMinorUnits(""); err == nil {
		t.Fatal("an empty amount must be rejected")
	}
}

func TestKnownTaxCategory(t *testing.T) {
	if !KnownTaxCategory(DefaultTaxCategory) {
		t.Fatalf("%s must be accepted", DefaultTaxCategory)
	}
	if KnownTaxCategory("physical_goods") {
		t.Fatal("an undocumented tax category must be rejected")
	}
}

func TestNewClientRequiresCredentials(t *testing.T) {
	if _, err := NewClient(Config{MerchantID: testMerchantID}); err == nil {
		t.Fatal("a missing private key must be rejected")
	}
}

func TestCreateCheckoutSendsPriceSnapshotAndOrderNumber(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	var sessionBody map[string]interface{}
	var tokenBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("X-Merchant-Id") == "" || r.Header.Get("X-Signature") == "" {
			t.Errorf("request to %s is not signed", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/actions/auth/issue-session-token":
			_ = json.Unmarshal(body, &tokenBody)
			mustWrite(t, w, `{"data":{"token":"jwt-1","expiresAt":"2026-01-01T00:00:00Z"}}`)
		case "/v1/actions/checkout/create-session":
			_ = json.Unmarshal(body, &sessionBody)
			mustWrite(t, w, `{"data":{"sessionId":"SES_1","checkoutUrl":"https://pay.example/checkout/SES_1","expiresAt":"2026-01-01T00:00:00Z"}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		MerchantID: testMerchantID, PrivateKey: privatePEM, StoreID: testStoreID,
		ProductID: testProductID, TestMode: true, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	session, err := client.CreateCheckout(context.Background(), Order{
		OrderNo: "PP20260101", Amount: 1999, Currency: "usd",
		BuyerIdentity: "user:7", ReturnURL: "https://panel.example/orders",
		ExpiresInSeconds: 900,
	})
	if err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	if !strings.HasPrefix(session.CheckoutURL, "https://pay.example/checkout/SES_1") {
		t.Fatalf("checkout url = %s", session.CheckoutURL)
	}
	if got := sessionBody["orderMerchantExternalId"]; got != "PP20260101" {
		t.Fatalf("orderMerchantExternalId = %v", got)
	}
	if got := sessionBody["currency"]; got != "USD" {
		t.Fatalf("currency = %v, want the ISO 4217 upper-case form", got)
	}
	snapshot, _ := sessionBody["priceSnapshot"].(map[string]interface{})
	if snapshot["amount"] != "19.99" {
		t.Fatalf("priceSnapshot amount = %v", snapshot["amount"])
	}
	if snapshot["taxCategory"] != DefaultTaxCategory {
		t.Fatalf("taxCategory = %v", snapshot["taxCategory"])
	}
	if tokenBody["buyerIdentity"] != "user:7" {
		t.Fatalf("buyerIdentity = %v", tokenBody["buyerIdentity"])
	}
}

func TestCreateCheckoutRejectsIncompleteOrders(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	client, err := NewClient(Config{MerchantID: testMerchantID, PrivateKey: privatePEM, ProductID: testProductID})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	cases := map[string]Order{
		"no order number":   {Amount: 100, Currency: "USD", BuyerIdentity: "user:1"},
		"no amount":         {OrderNo: "PP1", Currency: "USD", BuyerIdentity: "user:1"},
		"no currency":       {OrderNo: "PP1", Amount: 100, BuyerIdentity: "user:1"},
		"no buyer identity": {OrderNo: "PP1", Amount: 100, Currency: "USD"},
	}
	for name, order := range cases {
		if _, err := client.CreateCheckout(context.Background(), order); err == nil {
			t.Fatalf("%s must be rejected", name)
		}
	}
}

func TestVerifyNotification(t *testing.T) {
	privatePEM, publicPEM, key := testKeys(t)
	client, err := NewClient(Config{
		MerchantID: testMerchantID, PrivateKey: privatePEM,
		TestMode: true, WebhookPublicKey: publicPEM,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	payload, header := signedWebhook(t, key, orderCompletedEvent("PP20260101"))
	notification, err := client.VerifyNotification(payload, header)
	if err != nil {
		t.Fatalf("verify notification: %v", err)
	}
	if notification.OrderNo != "PP20260101" || notification.OrderID != "ORD_1" {
		t.Fatalf("notification = %+v", notification)
	}
	// The MoR charges tax on top of the price, so the comparable amount is
	// the pre-tax one, not the total the buyer was charged.
	if notification.Amount != "10.00" || notification.TaxAmount != "1.00" {
		t.Fatalf("amount=%s tax=%s", notification.Amount, notification.TaxAmount)
	}
	if !notification.TestMode {
		t.Fatal("a test-mode event must be reported as test mode")
	}

	tampered := []byte(strings.Replace(string(payload), `"10.00"`, `"1.00"`, 1))
	if _, err := client.VerifyNotification(tampered, header); err == nil {
		t.Fatal("changing a signed field must invalidate the signature")
	}
	if _, err := client.VerifyNotification(payload, ""); err == nil {
		t.Fatal("a missing signature header must be rejected")
	}
}

func TestVerifyNotificationFallsBackToMetadataOrderNumber(t *testing.T) {
	privatePEM, publicPEM, key := testKeys(t)
	client, err := NewClient(Config{
		MerchantID: testMerchantID, PrivateKey: privatePEM,
		TestMode: true, WebhookPublicKey: publicPEM,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	event := orderCompletedEvent("")
	data := event["data"].(map[string]interface{})
	delete(data, "orderMerchantExternalId")
	data["orderMetadata"] = map[string]string{"order_no": "PP20260102"}

	payload, header := signedWebhook(t, key, event)
	notification, err := client.VerifyNotification(payload, header)
	if err != nil {
		t.Fatalf("verify notification: %v", err)
	}
	if notification.OrderNo != "PP20260102" {
		t.Fatalf("orderNo = %s", notification.OrderNo)
	}
}

func TestGetOrder(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/graphql" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, `{"data":{"onetimeOrder":{"id":"ORD_1","status":"completed","currency":"USD","testMode":true,"orderMerchantExternalId":"PP1","subtotal":{"amount":"1000","currency":"USD"},"taxAmount":{"amount":"100","currency":"USD"},"total":{"amount":"1100","currency":"USD"}}}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{MerchantID: testMerchantID, PrivateKey: privatePEM, BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	gatewayOrder, err := client.GetOrder(context.Background(), "ORD_1")
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if !gatewayOrder.Completed() || gatewayOrder.MerchantExternalID != "PP1" {
		t.Fatalf("gateway order = %+v", gatewayOrder)
	}
	if gatewayOrder.Subtotal.Amount != "1000" {
		t.Fatalf("subtotal = %s", gatewayOrder.Subtotal.Amount)
	}
}

func TestGetOrderReportsMissingOrder(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		mustWrite(t, w, `{"data":{"onetimeOrder":null}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{MerchantID: testMerchantID, PrivateKey: privatePEM, BaseURL: server.URL})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.GetOrder(context.Background(), "ORD_missing"); err == nil {
		t.Fatal("a missing order must be an error, not an empty order")
	}
}

func TestRegisterWebhookAddsAndUpdates(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	var paths []string
	var updateBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/actions/store/add-webhook":
			mustWrite(t, w, `{"data":{"webhook":{"id":"WHK_1","storeId":"STO_2aUyqjCzEIiEcYMKj7TZtw","channel":"http","url":"https://panel.example/v1/notify/Waffo/tok","events":["order.completed"],"testMode":true}}}`)
		case "/v1/actions/store/update-webhook":
			_ = json.Unmarshal(body, &updateBody)
			mustWrite(t, w, `{"data":{"webhook":{"id":"WHK_1","storeId":"STO_2aUyqjCzEIiEcYMKj7TZtw","channel":"http","url":"https://panel.example/v1/notify/Waffo/tok","events":["order.completed"],"testMode":true}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{
		MerchantID: testMerchantID, PrivateKey: privatePEM, StoreID: testStoreID,
		TestMode: true, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	callbackURL := "https://panel.example/v1/notify/Waffo/tok"
	id, err := client.RegisterWebhook(context.Background(), "", callbackURL)
	if err != nil || id != "WHK_1" {
		t.Fatalf("register webhook = %s, %v", id, err)
	}
	// A second save must keep the same endpoint instead of adding another.
	id, err = client.RegisterWebhook(context.Background(), id, callbackURL)
	if err != nil || id != "WHK_1" {
		t.Fatalf("re-register webhook = %s, %v", id, err)
	}
	if updateBody["url"] != callbackURL {
		t.Fatalf("update url = %v", updateBody["url"])
	}
	if len(paths) != 2 || paths[0] != "/v1/actions/store/add-webhook" || paths[1] != "/v1/actions/store/update-webhook" {
		t.Fatalf("request paths = %v", paths)
	}
}

// A transient failure must not be mistaken for a deleted endpoint: re-adding
// on any error leaves the store with a second endpoint delivering every event
// twice, and the config only remembers the newest id.
func TestRegisterWebhookKeepsEndpointOnTransientFailure(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/actions/store/update-webhook" {
			w.WriteHeader(http.StatusBadGateway)
			mustWrite(t, w, `{"data":null,"errors":[{"message":"upstream unavailable","layer":"service"}]}`)
			return
		}
		mustWrite(t, w, `{"data":{"webhook":{"id":"WHK_dup","storeId":"STO_2aUyqjCzEIiEcYMKj7TZtw","channel":"http","url":"https://panel.example/v1/notify/Waffo/tok","events":["order.completed"],"testMode":false}}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		MerchantID: testMerchantID, PrivateKey: privatePEM, StoreID: testStoreID, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	if _, err := client.RegisterWebhook(context.Background(), "WHK_live", "https://panel.example/v1/notify/Waffo/tok"); err == nil {
		t.Fatal("a transient update failure must surface, not silently register a second endpoint")
	}
	for _, path := range paths {
		if path == "/v1/actions/store/add-webhook" {
			t.Fatal("a transient update failure must not add a duplicate endpoint")
		}
	}
}

func TestRegisterWebhookReAddsWhenStoredEndpointIsGone(t *testing.T) {
	privatePEM, _, _ := testKeys(t)
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/actions/store/update-webhook" {
			w.WriteHeader(http.StatusNotFound)
			mustWrite(t, w, `{"data":null,"errors":[{"message":"Webhook not found","layer":"service"}]}`)
			return
		}
		mustWrite(t, w, `{"data":{"webhook":{"id":"WHK_2","storeId":"STO_2aUyqjCzEIiEcYMKj7TZtw","channel":"http","url":"https://panel.example/v1/notify/Waffo/tok","events":["order.completed"],"testMode":false}}}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{
		MerchantID: testMerchantID, PrivateKey: privatePEM, StoreID: testStoreID, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	id, err := client.RegisterWebhook(context.Background(), "WHK_gone", "https://panel.example/v1/notify/Waffo/tok")
	if err != nil || id != "WHK_2" {
		t.Fatalf("register webhook = %s, %v", id, err)
	}
	if len(paths) != 2 || paths[1] != "/v1/actions/store/add-webhook" {
		t.Fatalf("request paths = %v", paths)
	}
}
