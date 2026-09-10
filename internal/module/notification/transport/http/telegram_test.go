package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/internal/module/notification"
)

type fakeNotificationService struct {
	notification.Service
	payloads [][]byte
}

func (f *fakeNotificationService) HandleTelegramWebhook(_ context.Context, payload []byte) error {
	f.payloads = append(f.payloads, append([]byte(nil), payload...))
	return nil
}

func telegramWebhookContext(t *testing.T, service notification.Service, botToken string) (*server.Hertz, func(secret string, body []byte) uint32) {
	t.Helper()
	engine := server.Default()
	engine.POST("/v1/telegram/webhook", TelegramHandler(service, func() string { return botToken }))
	return engine, func(secret string, body []byte) uint32 {
		ctx := engine.NewContext()
		ctx.Request.SetRequestURI("/v1/telegram/webhook")
		ctx.Request.Header.SetMethod(http.MethodPost)
		if secret != "" {
			ctx.Request.Header.Set("X-Telegram-Bot-Api-Secret-Token", secret)
		}
		ctx.Request.SetBody(body)
		engine.ServeHTTP(context.Background(), ctx)
		if got := ctx.Response.StatusCode(); got != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, got)
		}
		var response struct {
			Code uint32 `json:"code"`
		}
		if err := json.Unmarshal(ctx.Response.Body(), &response); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		return response.Code
	}
}

func TestTelegramHandler_abortsAndWritesSuccessEnvelope_whenSecretIsInvalid(t *testing.T) {
	fake := &fakeNotificationService{}
	_, post := telegramWebhookContext(t, fake, "bot-token")

	if code := post("invalid", []byte(`{"update_id":1}`)); code != 200 {
		t.Fatalf("expected success envelope code 200, got %d", code)
	}
	if len(fake.payloads) != 0 {
		t.Fatalf("update was dispatched despite an invalid secret: %d payloads", len(fake.payloads))
	}
}

// An unconfigured bot must reject every request, including the secret an
// attacker could derive from the empty token.
func TestTelegramHandler_rejectsAll_whenBotTokenIsEmpty(t *testing.T) {
	fake := &fakeNotificationService{}
	_, post := telegramWebhookContext(t, fake, "")

	post(notification.WebhookSecret(""), []byte(`{"update_id":1}`))
	if len(fake.payloads) != 0 {
		t.Fatal("update was dispatched despite the bot being unconfigured")
	}
}

func TestTelegramHandler_dispatchesPayload_whenSecretMatches(t *testing.T) {
	fake := &fakeNotificationService{}
	_, post := telegramWebhookContext(t, fake, "bot-token")

	body := []byte(`{"update_id":7}`)
	if code := post(notification.WebhookSecret("bot-token"), body); code != 200 {
		t.Fatalf("expected success envelope code 200, got %d", code)
	}
	if len(fake.payloads) != 1 || string(fake.payloads[0]) != string(body) {
		t.Fatalf("payloads = %q, want the request body dispatched once", fake.payloads)
	}
}
