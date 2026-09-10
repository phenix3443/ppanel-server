package logger

import (
	"bytes"
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFieldRedactsSensitiveKeys(t *testing.T) {
	for _, key := range []string{
		"token", "access_token", "password", "client_secret", "authorization",
		"cookie", "signature", "session_id", "uuid", "request_body", "response",
		"payload", "sql", "user", "order_info", "subscribe", "template", "value",
		"email", "recovery_email", "phone", "ip", "user_agent", "device_id",
		"group_chat_id", "redirect_url",
	} {
		t.Run(key, func(t *testing.T) {
			field := Field(key, "sensitive-value")
			if field.Value != RedactedValue {
				t.Fatalf("field %q was not redacted: %#v", key, field.Value)
			}
		})
	}
}

func TestGormTraceDoesNotLogExpandedSQL(t *testing.T) {
	const secret = "person@example.com"

	setTestLogState(t, jsonEncodingType)
	w := new(mockWriter)
	oldWriter := writer.Swap(w)
	t.Cleanup(func() {
		writer.Store(oldWriter)
	})

	(&GormLogger{SlowThreshold: time.Nanosecond}).Trace(context.Background(), time.Now().Add(-time.Millisecond), func() (string, int64) {
		return `SELECT * FROM users WHERE email = '` + secret + `'`, 1
	}, nil)

	got := w.String()
	if strings.Contains(got, secret) || strings.Contains(got, "FROM users") {
		t.Fatalf("gorm trace contains expanded SQL: %s", got)
	}
	if !strings.Contains(got, `"operation":"SELECT"`) || !strings.Contains(got, `"rows":1`) {
		t.Fatalf("gorm trace is missing safe diagnostics: %s", got)
	}
}

func TestGormTraceSkipsFastSuccessfulQueries(t *testing.T) {
	setTestLogState(t, jsonEncodingType)
	w := new(mockWriter)
	oldWriter := writer.Swap(w)
	t.Cleanup(func() {
		writer.Store(oldWriter)
	})

	called := false
	(&GormLogger{SlowThreshold: time.Second}).Trace(context.Background(), time.Now(), func() (string, int64) {
		called = true
		return "SELECT 1", 1
	}, nil)

	if called {
		t.Fatal("fast query expanded SQL callback was invoked")
	}
	if got := w.String(); got != "" {
		t.Fatalf("fast query produced a log entry: %s", got)
	}
}

func TestFieldPreservesSafeOperationalKeys(t *testing.T) {
	for _, key := range []string{"status", "method", "route", "duration", "request_id", "user_id", "order_no"} {
		t.Run(key, func(t *testing.T) {
			field := Field(key, "diagnostic-value")
			if field.Value != "diagnostic-value" {
				t.Fatalf("safe field %q was unexpectedly redacted: %#v", key, field.Value)
			}
		})
	}
}

func TestRiskFieldPreservesOnlyExplicitRequestMetadata(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value string
		want  string
	}{
		{key: "client_ip", value: "203.0.113.1", want: "203.0.113.1"},
		{key: "user_agent", value: "RiskClient/1.0", want: "RiskClient/1.0"},
		{key: "email", value: "person@example.com", want: RedactedValue},
		{key: "authorization", value: "Bearer secret", want: RedactedValue},
	} {
		field := redactField(RiskField(tc.key, tc.value))
		if field.Value != tc.want {
			t.Fatalf("RiskField(%q) = %#v, want %q", tc.key, field.Value, tc.want)
		}
	}
}

func TestRedactTextRemovesCommonCredentialsAndEmail(t *testing.T) {
	secrets := []string{
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.VeryLongSignatureValue",
		"123456789:AAabcdefghijklmnopqrstuvwxyz1234",
		"alice@example.com",
		"Bearer top-secret-token",
		"query-secret",
	}
	input := strings.Join(secrets[:4], " ") + " https://example.test/callback?token=" + secrets[4]
	got := redactText(input)

	for _, secret := range secrets {
		if strings.Contains(got, secret) {
			t.Fatalf("redacted text contains %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, RedactedValue) {
		t.Fatalf("redacted text does not contain replacement marker: %s", got)
	}
}

func TestRedactValueRecursesThroughStructuredPayloads(t *testing.T) {
	value := map[string]any{
		"safe": "ok",
		"nested": map[string]any{
			"token": "subscription-secret",
			"note":  "contact person@example.com",
		},
		"items": []any{map[string]string{"password": "secret", "status": "ready"}},
	}

	redacted, ok := redactValue(value).(map[string]any)
	if !ok {
		t.Fatalf("redacted value has type %T", redactValue(value))
	}
	nested := redacted["nested"].(map[string]any)
	if nested["token"] != RedactedValue || nested["note"] != "contact "+RedactedValue {
		t.Fatalf("nested payload was not redacted: %#v", nested)
	}
	items := redacted["items"].([]any)
	item := items[0].(map[string]string)
	if item["password"] != RedactedValue || item["status"] != "ready" {
		t.Fatalf("slice payload was not redacted: %#v", items)
	}
}

func TestSplitLogArgsRedactsLegacyStructuredFields(t *testing.T) {
	msg, fields := splitLogArgs([]any{
		"lookup failed",
		Field("token", "subscription-secret"),
		Field("status", 401),
	})

	if msg != "lookup failed" {
		t.Fatalf("unexpected message: %q", msg)
	}
	if len(fields) != 2 {
		t.Fatalf("unexpected field count: %d", len(fields))
	}
	if fields[0].Value != RedactedValue || fields[1].Value != 401 {
		t.Fatalf("unexpected fields: %#v", fields)
	}
}

func TestWriterRedactsDirectFieldsAndMessagePatterns(t *testing.T) {
	const (
		token = "direct-token-secret"
		email = "person@example.com"
		jwt   = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.VeryLongSignatureValue"
	)

	var output bytes.Buffer
	w := NewWriter(&output)
	w.Info("authentication failed for "+email+" using "+jwt,
		LogField{Key: "token", Value: token},
		LogField{Key: "status", Value: 401},
	)

	got := output.String()
	for _, secret := range []string{token, email, jwt} {
		if strings.Contains(got, secret) {
			t.Fatalf("writer output contains %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, `"token":"[REDACTED]"`) || !strings.Contains(got, `"status":401`) {
		t.Fatalf("writer output is missing expected redaction or safe field: %s", got)
	}
}

func TestWriterLimitsStructuredAndFieldValues(t *testing.T) {
	old := atomic.SwapUint32(&maxContentLength, 4)
	t.Cleanup(func() { atomic.StoreUint32(&maxContentLength, old) })

	var output bytes.Buffer
	w := NewWriter(&output)
	w.Info(map[string]any{"status": "abcdefgh"}, LogField{Key: "detail", Value: "abcdefgh"})

	got := output.String()
	if strings.Contains(got, "abcdefgh") || !strings.Contains(got, `"status":"abcd"`) || !strings.Contains(got, `"detail":"abcd"`) || !strings.Contains(got, `"truncated":true`) {
		t.Fatalf("structured values were not limited: %s", got)
	}
}
