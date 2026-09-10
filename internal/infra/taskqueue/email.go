package taskqueue

import "github.com/perfect-panel/server/pkg/requestmeta"

const (
	// ForthwithSendEmail forthwith send email
	ForthwithSendEmail = "forthwith:email:send"
)

const (
	EmailTypeVerify        = "verify"
	EmailTypeMaintenance   = "maintenance"
	EmailTypeExpiration    = "expiration"
	EmailTypeTrafficExceed = "traffic_exceed"
	EmailTypeCustom        = "custom"
)

type (
	SendEmailPayload struct {
		requestmeta.Metadata
		Type    string                 `json:"type"`
		Email   string                 `json:"to"`
		Subject string                 `json:"subject"`
		Content map[string]interface{} `json:"content"`
	}
)
