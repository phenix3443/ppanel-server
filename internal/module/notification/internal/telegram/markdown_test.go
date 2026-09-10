package telegram

import (
	"strings"
	"testing"
)

func TestEscapeMarkdownV2EscapesEveryReservedCharacter(t *testing.T) {
	const reserved = "_*[]()~`>#+-=|{}.!"
	escaped := EscapeMarkdownV2(reserved)
	for i, r := range escaped {
		expectBackslash := i%2 == 0
		if expectBackslash && r != '\\' {
			t.Fatalf("escaped = %q, want a backslash before every reserved character", escaped)
		}
	}
	if EscapeMarkdownV2(`\`) != `\\` {
		t.Fatalf("backslash itself must be escaped, got %q", EscapeMarkdownV2(`\`))
	}
	if got := EscapeMarkdownV2(`\.`); got != `\\\.` {
		t.Fatalf("pre-escaped input must not collapse, got %q", got)
	}
}

func TestRenderMarkdownV2EscapesValuesButKeepsTemplateMarkup(t *testing.T) {
	out, err := RenderMarkdownV2("*Order {{.OrderNo}}*", map[string]string{"OrderNo": "A-1.2 (test)"})
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	if out != `*Order A\-1\.2 \(test\)*` {
		t.Fatalf("rendered = %q, want escaped value inside intact bold markers", out)
	}
}

// A key the caller forgot must render as nothing — the default "<no value>"
// contains '>' and would make Telegram reject the whole message.
func TestRenderMarkdownV2RendersMissingKeysAsEmpty(t *testing.T) {
	out, err := RenderMarkdownV2("账号: {{.Missing}}，完毕", map[string]string{})
	if err != nil {
		t.Fatalf("render error = %v", err)
	}
	if out != "账号: ，完毕" {
		t.Fatalf("rendered = %q, want the missing key to vanish", out)
	}
}

// assertMarkdownV2Clean is a miniature MarkdownV2 linter: after removing
// escape sequences, the only reserved characters allowed to remain are the
// paired '*' (bold) and '_' (italic) the templates use deliberately. One
// unescaped '.' or unbalanced marker makes Telegram reject the entire
// message, which is a silent notification outage.
func assertMarkdownV2Clean(t *testing.T, rendered string) {
	t.Helper()
	var bold, italic int
	runes := []rune(rendered)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			if i+1 >= len(runes) {
				t.Fatalf("dangling backslash at end of %q", rendered)
			}
			i++ // escaped character, skip it
		case '*':
			bold++
		case '_':
			italic++
		case '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!':
			t.Fatalf("unescaped MarkdownV2 character %q in %q", runes[i], rendered)
		}
	}
	if bold%2 != 0 {
		t.Fatalf("unbalanced bold markers (%d) in %q", bold, rendered)
	}
	if italic%2 != 0 {
		t.Fatalf("unbalanced italic markers (%d) in %q", italic, rendered)
	}
}

// Every template must survive rendering with adversarial data: amounts carry
// '.', timestamps carry '-' and ':', order numbers and plan names carry
// whatever the operator typed.
func TestTemplatesRenderValidMarkdownV2(t *testing.T) {
	adversarial := map[string]string{
		"Id": "7", "Time": "2026-08-02 15:04:05",
		"OrderNo": "NO-2026.08.02-001", "TradeNo": "T_1+2=3",
		"SubscribeName": "套餐 (v2.0) [特惠]", "OrderAmount": "12.34",
		"ExpireTime": "2026-09-01 00:00:00", "ResetTime": "2026-08-02 00:10:00",
		"PaymentMethod": "e_pay!", "Balance": "99.90",
		"OrderStatus": "已支付", "UserEmail": "a.b_c@example.com",
		"OrderTime": "2026-08-02 15:04:05",
		"Date":      "2026-08-01", "Orders": "3", "Amount": "45.67",
		"Subscribe": "· 套餐A：2 单，30.00\n· 无", "Payment": "· e-pay：3 单，45.67",
		"ExpiredAt": "2026-08-05 12:00:00", "RenewalAmount": "15.00",
		"Email": "user@example.com",
	}
	templates := map[string]string{
		"BindNotify":            BindNotify,
		"PurchaseNotify":        PurchaseNotify,
		"RenewalNotify":         RenewalNotify,
		"RechargeNotify":        RechargeNotify,
		"AdminOrderNotify":      AdminOrderNotify,
		"AdminOrderDaily":       AdminOrderDaily,
		"SubscribeExpireNotify": SubscribeExpireNotify,
		"UnbindNotify":          UnbindNotify,
		"ResetTrafficNotify":    ResetTrafficNotify,
	}
	for name, tpl := range templates {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(tpl, "**") {
				t.Fatalf("template still contains legacy ** markup")
			}
			rendered, err := RenderMarkdownV2(tpl, adversarial)
			if err != nil {
				t.Fatalf("render error = %v", err)
			}
			assertMarkdownV2Clean(t, rendered)
		})
	}
}
