package email

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/infra/taskqueue"
)

// The queued literal is only the fallback: an operator-configured subject
// wins and renders against the same data as the body.
func TestResolveSubjectPrefersConfiguredTemplate(t *testing.T) {
	data := map[string]interface{}{"SiteName": "示例站"}

	got := resolveSubject(context.Background(), "【{{.SiteName}}】订阅已到期", "Subscription Expired", data)
	if got != "【示例站】订阅已到期" {
		t.Fatalf("subject = %q, want the rendered configured template", got)
	}
}

func TestResolveSubjectFallsBackWhenUnconfigured(t *testing.T) {
	got := resolveSubject(context.Background(), "", "Subscription Expired", nil)
	if got != "Subscription Expired" {
		t.Fatalf("subject = %q, want the queued fallback", got)
	}
}

// A subject with a template typo is still sent as the operator wrote it; the
// localized text beats silently reverting to English.
func TestResolveSubjectKeepsRawTextOnBadTemplate(t *testing.T) {
	got := resolveSubject(context.Background(), "订阅已到期 {{.SiteName", "Subscription Expired", nil)
	if got != "订阅已到期 {{.SiteName" {
		t.Fatalf("subject = %q, want the raw configured text", got)
	}
}

func TestRenderEmailTemplateReportsParseErrors(t *testing.T) {
	if _, err := renderEmailTemplate("body", "{{.Broken", nil); err == nil {
		t.Fatal("parse error was swallowed")
	}
	rendered, err := renderEmailTemplate("body", "Hello {{.Name}}", map[string]interface{}{"Name": "User"})
	if err != nil || rendered != "Hello User" {
		t.Fatalf("rendered = %q, err = %v", rendered, err)
	}
}

func TestEmailLogContentRedactsVerificationCode(t *testing.T) {
	content := map[string]interface{}{"Code": "123456", "SiteName": "Example"}

	redacted := emailLogContent(taskqueue.EmailTypeVerify, content)
	if redacted["redacted"] != true {
		t.Fatalf("verification log content = %#v", redacted)
	}
	if _, ok := redacted["Code"]; ok {
		t.Fatalf("verification log contains code: %#v", redacted)
	}
	if content["Code"] != "123456" {
		t.Fatalf("rendering content was mutated: %#v", content)
	}
}

func TestEmailLogContentRedactsNonVerificationContent(t *testing.T) {
	content := map[string]interface{}{"message": "maintenance"}

	if got := emailLogContent(taskqueue.EmailTypeMaintenance, content); got["redacted"] != true || got["email_type"] != taskqueue.EmailTypeMaintenance {
		t.Fatalf("non-verification log content = %#v", got)
	}
}
