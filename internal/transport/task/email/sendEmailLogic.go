package email

import (
	"bytes"
	"context"
	"encoding/json"
	"text/template"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/mail"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type SendEmailLogic struct {
	deps Dependencies
}

func emailLogContent(emailType string, _ map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"redacted": true, "email_type": emailType}
}

func renderEmailTemplate(name, text string, data map[string]interface{}) (string, error) {
	tpl, err := template.New(name).Parse(text)
	if err != nil {
		return "", err
	}
	var result bytes.Buffer
	if err := tpl.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
}

// resolveSubject prefers the operator-configured subject over the fallback
// literal the producer queued. The configured subject renders with the same
// data as the body; if it fails to render it is still sent as raw text,
// because a localized subject with a template typo beats silently reverting
// to English.
func resolveSubject(ctx context.Context, configured, fallback string, data map[string]interface{}) string {
	if configured == "" {
		return fallback
	}
	rendered, err := renderEmailTemplate("subject", configured, data)
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Execute subject template failed",
			logger.Field("error", err.Error()),
			logger.Field("subject", configured),
		)
		return configured
	}
	return rendered
}

func NewSendEmailLogic(deps Dependencies) *SendEmailLogic {
	return &SendEmailLogic{
		deps: deps,
	}
}
func (l *SendEmailLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload taskqueue.SendEmailPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Unmarshal payload failed",
			logger.Field("error", err.Error()),
		)
		return nil
	}
	ctx = requestmeta.With(ctx, payload.Metadata)
	ctx = logger.ContextWithRequestMetadata(ctx, payload.Metadata)
	sender, err := mail.NewSender(l.deps.Email().Platform, l.deps.Email().PlatformConfig, l.deps.SiteName())
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] NewSender failed", logger.Field("error", err.Error()))
		return nil
	}
	// The operator-configured subject of a typed notification wins over the
	// literal queued by the producer; it renders with the same data as the
	// body so subjects can interpolate {{.SiteName}} and friends.
	var content, bodyTemplate, subjectTemplate string
	switch payload.Type {
	case taskqueue.EmailTypeVerify:
		payload.Content["Type"] = uint8(payload.Content["Type"].(float64))
		bodyTemplate = l.deps.Email().VerifyEmailTemplate
		subjectTemplate = l.deps.Email().VerifyEmailSubject
	case taskqueue.EmailTypeMaintenance:
		bodyTemplate = l.deps.Email().MaintenanceEmailTemplate
		subjectTemplate = l.deps.Email().MaintenanceEmailSubject
	case taskqueue.EmailTypeExpiration:
		bodyTemplate = l.deps.Email().ExpirationEmailTemplate
		subjectTemplate = l.deps.Email().ExpirationEmailSubject
	case taskqueue.EmailTypeTrafficExceed:
		bodyTemplate = l.deps.Email().TrafficExceedEmailTemplate
		subjectTemplate = l.deps.Email().TrafficExceedEmailSubject
	case taskqueue.EmailTypeCustom:
		if payload.Content == nil {
			logger.WithContext(ctx).Error("[SendEmailLogic] Custom email content is empty")
			return nil
		}
		if tpl, ok := payload.Content["content"].(string); !ok {
			logger.WithContext(ctx).Error("[SendEmailLogic] Custom email content is not a string")
			return nil
		} else {
			content = tpl
		}
	default:
		logger.WithContext(ctx).Error("[SendEmailLogic] Unsupported email type",
			logger.Field("type", payload.Type),
		)
		return nil
	}
	if bodyTemplate != "" {
		content, err = renderEmailTemplate(payload.Type, bodyTemplate, payload.Content)
		if err != nil {
			logger.WithContext(ctx).Error("[SendEmailLogic] Execute template failed",
				logger.Field("error", err.Error()),
				logger.Field("template", bodyTemplate),
			)
			return nil
		}
	}
	subject := resolveSubject(ctx, subjectTemplate, payload.Subject, payload.Content)
	messageLog := log.Message{
		Metadata: payload.Metadata,
		Platform: l.deps.Email().Platform,
		To:       logger.RedactedValue,
		// Subjects are operator-controlled templates and may interpolate names,
		// addresses or one-time credentials. Keep only the notification type in
		// the audit record; the actual subject is used solely for delivery.
		Subject: payload.Type,
		Content: emailLogContent(payload.Type, payload.Content),
		Status:  0, // attempted; finalized after the provider call
	}
	emailLog, err := messageLog.Marshal()
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Marshal message log failed", logger.Field("error", err.Error()))
		return err
	}
	audit := &log.SystemLog{
		Type:     log.TypeEmailMessage.Uint8(),
		Date:     timeutil.Now().Format("2006-01-02"),
		ObjectID: 0,
		Content:  string(emailLog),
	}
	// Persist the attempt before contacting the provider. A database failure is
	// safe to retry here because no email has been sent yet.
	if err = l.deps.Store.Log().Insert(ctx, audit); err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Insert email log failed", logger.Field("error", err.Error()))
		return err
	}

	err = sender.Send([]string{payload.Email}, subject, content)
	if err != nil {
		messageLog.Status = 2
		logger.WithContext(ctx).Error("[SendEmailLogic] Send email failed", logger.Field("error", err.Error()))
	} else {
		messageLog.Status = 1
	}
	emailLog, marshalErr := messageLog.Marshal()
	if marshalErr != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Marshal finalized message log failed", logger.Field("error", marshalErr.Error()))
		return nil
	}
	audit.Content = string(emailLog)
	if updateErr := l.deps.Store.Log().Update(ctx, audit); updateErr != nil {
		// The pre-created attempt row remains, so the delivery is never missing
		// from the audit trail. Retrying after a provider call could duplicate the
		// email, therefore record the update failure without failing the task.
		logger.WithContext(ctx).Error("[SendEmailLogic] Finalize email log failed", logger.Field("error", updateErr.Error()), logger.Field("log_id", audit.Id))
	}
	return nil
}
