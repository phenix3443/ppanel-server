package mail

import "testing"

func TestEmailSend(t *testing.T) {
	t.Skipf("Skip TestEmailSend")
	config := &SMTPConfig{
		Host:     "smtp.mail.me.com",
		Port:     587,
		User:     "support@ppanel.dev",
		Pass:     "password",
		From:     "support@ppanel.dev",
		SSL:      true,
		SiteName: "",
	}
	address := []string{"tension@sparkdance.dev"}
	subject := "test"
	body := "test"
	email := NewSMTPClient(config)
	err := email.Send(address, subject, body)
	if err != nil {
		t.Errorf("send email error: %v", err)
	}
}
