package sms

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/sms"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type SmsSendCount struct {
	Count    int   `json:"count"`
	CreateAt int64 `json:"create_at"`
}

type SendSmsLogic struct {
	deps Dependencies
}

func newSMSMessageLog(platform string, messageType uint8, metadata ...requestmeta.Metadata) *log.Message {
	var requestMetadata requestmeta.Metadata
	if len(metadata) > 0 {
		requestMetadata = metadata[0]
	}
	return &log.Message{
		Metadata: requestMetadata,
		Platform: platform,
		To:       logger.RedactedValue,
		Subject:  auth.ParseVerifyType(messageType).String(),
		Content:  map[string]interface{}{"redacted": true},
	}
}

func NewSendSmsLogic(deps Dependencies) *SendSmsLogic {
	return &SendSmsLogic{
		deps: deps,
	}
}
func (l *SendSmsLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload taskqueue.SendSmsPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.WithContext(ctx).Error("[SendSmsLogic] Unmarshal payload failed",
			logger.Field("error", err.Error()),
			logger.Field("payload", task.Payload()),
		)
		return nil
	}
	ctx = requestmeta.With(ctx, payload.Metadata)
	ctx = logger.ContextWithRequestMetadata(ctx, payload.Metadata)
	client, err := sms.NewSender(l.deps.Mobile().Platform, l.deps.Mobile().PlatformConfig)
	if err != nil {
		logger.WithContext(ctx).Error("[SendSmsLogic] New send sms client failed", logger.Field("error", err.Error()), logger.Field("payload", payload))
		return err
	}
	createSms := newSMSMessageLog(l.deps.Mobile().Platform, payload.Type, payload.Metadata)
	content, marshalErr := createSms.Marshal()
	if marshalErr != nil {
		return marshalErr
	}
	audit := &log.SystemLog{
		Type:     log.TypeMobileMessage.Uint8(),
		Date:     timeutil.Now().Format("2006-01-02"),
		ObjectID: 0,
		Content:  string(content),
	}
	// Record the attempt before contacting the provider so a storage failure
	// cannot produce a successful but unaudited SMS delivery.
	if err = l.deps.Store.Log().Insert(ctx, audit); err != nil {
		logger.WithContext(ctx).Error("[SendSmsLogic] Insert sms log failed", logger.Field("error", err.Error()))
		return err
	}
	err = client.SendCode(payload.TelephoneArea, payload.Telephone, payload.Content)

	if err != nil {
		logger.WithContext(ctx).Error("[SendSmsLogic] Send sms failed", logger.Field("error", err.Error()), logger.Field("payload", payload))
		createSms.Status = 2
	} else {
		createSms.Status = 1
	}
	logger.WithContext(ctx).Info("[SendSmsLogic] Send sms", logger.Field("telephone", payload.Telephone), logger.Field("content", createSms.Content))

	content, marshalErr = createSms.Marshal()
	if marshalErr != nil {
		return nil
	}
	audit.Content = string(content)
	if updateErr := l.deps.Store.Log().Update(ctx, audit); updateErr != nil {
		logger.WithContext(ctx).Error("[SendSmsLogic] Finalize sms log failed", logger.Field("error", updateErr.Error()), logger.Field("log_id", audit.Id))
	}
	return nil
}
