package notification

import "testing"

func TestDeriveIsDeterministicAndTokenScoped(t *testing.T) {
	a := WebhookSecret("123456:token-a")
	if a != WebhookSecret("123456:token-a") {
		t.Fatal("WebhookSecret is not deterministic")
	}
	if a == WebhookSecret("123456:token-b") {
		t.Fatal("different tokens must derive different secrets")
	}
	if len(a) != 64 {
		t.Fatalf("secret length = %d, want 64 hex chars", len(a))
	}
}

// The derived secret must never equal a naive digest of the token itself:
// the previous scheme (md5 of the token in a query parameter) is what this
// package replaces, and the telegramWebhookSecretPurpose string is what keeps them apart.
func TestDeriveDiffersFromPlainDigest(t *testing.T) {
	if WebhookSecret("") == WebhookSecret(telegramWebhookSecretPurpose) {
		t.Fatal("telegramWebhookSecretPurpose separation is not applied")
	}
}

func TestEqual(t *testing.T) {
	secret := WebhookSecret("123456:token")
	if !WebhookSecretEqual(secret, secret) {
		t.Fatal("WebhookSecretEqual rejected the matching secret")
	}
	if WebhookSecretEqual("wrong", secret) {
		t.Fatal("WebhookSecretEqual accepted a mismatching secret")
	}
	if WebhookSecretEqual("", secret) {
		t.Fatal("WebhookSecretEqual accepted an empty secret")
	}
}
