package verifycode

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/ratelimit"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type SmsSendCount struct {
	Count    int64 `json:"count"`
	CreateAt int64 `json:"create_at"`
}

type SendSmsCodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps SendSmsCodeDependencies
}

// NewSendSmsCodeLogic Get sms verification code
func NewSendSmsCodeLogic(ctx context.Context, deps SendSmsCodeDependencies) *SendSmsCodeLogic {
	return &SendSmsCodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *SendSmsCodeLogic) SendSmsCode(req *dto.SendSmsCodeRequest) (resp *dto.SendCodeResponse, err error) {
	verifyType := auth.ParseVerifyType(req.Type)
	if verifyType == auth.Register {
		if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, identifier.Mobile); err != nil {
			return nil, err
		}
	} else if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Mobile); err != nil {
		return nil, err
	}
	phoneNumber, err := identifier.FormatToE164(req.TelephoneAreaCode, req.Telephone)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}

	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeTelephoneCacheKey, verifyType, phoneNumber)
	// Check if the limit is exceeded of current request
	interval := l.deps.Config.VerifyCodeInterval
	if interval <= 0 {
		interval = 60
	}
	limiter := ratelimit.NewPeriodLimit(int(interval), 1, l.deps.Redis, fmt.Sprintf("%smobile:%s:", config.SendIntervalKeyPrefix, verifyType))
	permit, err := limiter.Take(phoneNumber)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Failed to take limit")
	}
	if !limiter.ParsePermitState(permit) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "send sms too many requests")
	}
	// Check if the limit is exceeded of the today
	dailyLimit := l.deps.Config.VerifyCodeLimit
	if dailyLimit <= 0 {
		dailyLimit = 15
	}
	dailyLimiter := ratelimit.NewPeriodLimit(86400, int(dailyLimit), l.deps.Redis, config.SendCountLimitKeyPrefix, ratelimit.Align())
	permit, err = dailyLimiter.Take(fmt.Sprintf("%s:%s:%s", "mobile", verifyType, phoneNumber))
	if err != nil {
		return nil, err
	}
	if !dailyLimiter.ParsePermitState(permit) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TodaySendCountExceedsLimit), "This account has reached the limit of sending times today")
	}
	m, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Mobile, phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindUserAuthMethodByOpenID error")
	}
	if verifyType == auth.Register && m.Id > 0 {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "mobile already bind")
	} else if verifyType == auth.Security && m.Id == 0 {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "mobile not bind")
	}

	metadata, _ := requestmeta.From(l.ctx)
	taskPayload := taskqueue.SendSmsPayload{
		Metadata:      metadata,
		Type:          req.Type,
		Telephone:     req.Telephone,
		TelephoneArea: req.TelephoneAreaCode,
	}
	// Generate verification code
	code := random.Key(6, 0)
	taskPayload.Telephone = req.Telephone
	taskPayload.Content = code
	if err = verification.SaveVerificationCode(l.ctx, l.deps.Redis, cacheKey, code, time.Second*time.Duration(l.deps.Config.VerifyCodeExpire)); err != nil {
		l.Errorw("[SendSmsCode]: Redis Error", logger.Field("error", err.Error()), logger.Field("cacheKey", cacheKey))
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), "Failed to set verification code")
	}

	// Marshal the task payload
	payloadValue, err := json.Marshal(taskPayload)
	if err != nil {
		l.Errorw("[SendSmsCode]: Marshal Error", logger.Field("error", err.Error()))
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), "Failed to marshal task payload")
	}
	// Create a queue task
	task := asynq.NewTask(taskqueue.ForthwithSendSms, payloadValue)
	// Enqueue the task
	taskInfo, err := l.deps.Queue.EnqueueContext(l.ctx, task)
	if err != nil {
		_ = verification.DeleteVerificationCode(l.ctx, l.deps.Redis, cacheKey)
		l.Errorw("[SendSmsCode]: Enqueue Error", logger.Field("error", err.Error()), logger.Field("type", taskPayload.Type))
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), "Failed to enqueue task")
	}
	l.Infow("[SendSmsCode]: Enqueue Success", logger.Field("taskID", taskInfo.ID), logger.Field("type", taskPayload.Type))
	return &dto.SendCodeResponse{
		Status: true,
	}, nil
}
