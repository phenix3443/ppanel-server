package mail

import "testing"

func TestSMTPSenderAssemblyPreservesConfiguration(t *testing.T) {
	sender, err := NewSender("smtp", `{"host":"smtp.example.test","port":587,"user":"test","pass":"test-only","from":"mail@example.test","reply_to":"reply@example.test"}`, "Example")
	if err != nil {
		t.Fatal(err)
	}
	client, ok := sender.(*SMTPClient)
	if !ok || client.conf.Host != "smtp.example.test" || client.conf.Port != 587 || client.conf.SiteName != "Example" || client.conf.ReplyTo != "reply@example.test" {
		t.Fatalf("unexpected SMTP sender: %+v", sender)
	}
	if NewSMTPClient(nil) != nil {
		t.Fatal("nil configuration no longer returns nil")
	}
	if _, err := NewSender("unsupported", `{}`, "Example"); err == nil {
		t.Fatal("unsupported provider accepted")
	}
}
