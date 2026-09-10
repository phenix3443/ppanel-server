package taskqueue

import "github.com/perfect-panel/server/pkg/requestmeta"

const (
	// ForthwithSendSms forthwith send email
	ForthwithSendSms = "forthwith:sms:send"
)

type (
	SendSmsPayload struct {
		requestmeta.Metadata
		Type          uint8  `json:"type"`
		Telephone     string `json:"telephone"`
		TelephoneArea string `json:"area"`
		Content       string `json:"content"`
	}
)
