package auth

import (
	"testing"

	"github.com/perfect-panel/server/internal/infra/mail"
	"github.com/stretchr/testify/assert"
)

// A fresh config marshals with every template and subject defaulted; the
// maintenance template used to stay empty because its default was guarded by
// a copy-pasted check of the expiration template.
func TestEmailAuthConfigMarshalFillsAllDefaults(t *testing.T) {
	cfg := new(EmailAuthConfig)
	roundTripped := new(EmailAuthConfig)
	roundTripped.Unmarshal(cfg.Marshal())

	assert.Equal(t, mail.DefaultEmailVerifyTemplate, roundTripped.VerifyEmailTemplate)
	assert.Equal(t, mail.DefaultExpirationEmailTemplate, roundTripped.ExpirationEmailTemplate)
	assert.Equal(t, mail.DefaultMaintenanceEmailTemplate, roundTripped.MaintenanceEmailTemplate)
	assert.Equal(t, mail.DefaultTrafficExceedEmailTemplate, roundTripped.TrafficExceedEmailTemplate)
	assert.Equal(t, mail.DefaultEmailVerifySubject, roundTripped.VerifyEmailSubject)
	assert.Equal(t, mail.DefaultExpirationEmailSubject, roundTripped.ExpirationEmailSubject)
	assert.Equal(t, mail.DefaultMaintenanceEmailSubject, roundTripped.MaintenanceEmailSubject)
	assert.Equal(t, mail.DefaultTrafficExceedEmailSubject, roundTripped.TrafficExceedEmailSubject)
}

func TestEmailAuthConfigMarshalKeepsCustomizedValues(t *testing.T) {
	cfg := &EmailAuthConfig{
		MaintenanceEmailTemplate: "<p>自定义维护正文</p>",
		ExpirationEmailSubject:   "【{{.SiteName}}】订阅已到期",
	}
	roundTripped := new(EmailAuthConfig)
	roundTripped.Unmarshal(cfg.Marshal())

	assert.Equal(t, "<p>自定义维护正文</p>", roundTripped.MaintenanceEmailTemplate)
	assert.Equal(t, "【{{.SiteName}}】订阅已到期", roundTripped.ExpirationEmailSubject)
}

func TestAlibabaCloudConfig_Marshal(t *testing.T) {
	v := new(AlibabaCloudConfig)
	t.Log(v.Marshal())
}

func TestAlibabaCloudConfig_Unmarshal(t *testing.T) {

	cfg := AlibabaCloudConfig{
		Access:       "AccessKeyId",
		Secret:       "AccessKeySecret",
		SignName:     "SignName",
		Endpoint:     "Endpoint",
		TemplateCode: "VerifyTemplateCode",
	}
	data := cfg.Marshal()
	v := new(AlibabaCloudConfig)
	err := v.Unmarshal(data)
	if err != nil {
		t.Fatal(err.Error())
	}
	assert.Equal(t, "AccessKeyId", v.Access)
}
