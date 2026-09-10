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
	"github.com/perfect-panel/server/internal/infra/mail"
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

type SendEmailCodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps SendEmailCodeDependencies
}

const (
	IntervalTime = 60
)

// NewSendEmailCodeLogic Get verification code
func NewSendEmailCodeLogic(ctx context.Context, deps SendEmailCodeDependencies) *SendEmailCodeLogic {
	return &SendEmailCodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *SendEmailCodeLogic) SendEmailCode(req *dto.SendCodeRequest) (resp *dto.SendCodeResponse, err error) {
	verifyType := auth.ParseVerifyType(req.Type)
	email, err := identifier.ValidateEmail(
		req.Email,
		l.deps.Config.DomainSuffixList,
		verifyType == auth.Register && l.deps.Config.EnableDomainSuffix,
	)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid email: %v", err)
	}
	if verifyType == auth.Register {
		if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, identifier.Email); err != nil {
			return nil, err
		}
	} else if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Email); err != nil {
		return nil, err
	}
	// Check if there is Redis in the code
	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeCacheKey, verifyType, email)
	// Check if the limit is exceeded of current request
	interval := l.deps.Config.VerifyCodeInterval
	if interval <= 0 {
		interval = IntervalTime
	}
	limiter := ratelimit.NewPeriodLimit(int(interval), 1, l.deps.Redis, fmt.Sprintf("%semail:%s:", config.SendIntervalKeyPrefix, verifyType))
	permit, err := limiter.Take(email)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Failed to take limit")
	}
	if !limiter.ParsePermitState(permit) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "send email too many requests")
	}
	// Check if the limit is exceeded of today
	dailyLimit := l.deps.Config.VerifyCodeLimit
	if dailyLimit <= 0 {
		dailyLimit = 15
	}
	dailyLimiter := ratelimit.NewPeriodLimit(86400, int(dailyLimit), l.deps.Redis, config.SendCountLimitKeyPrefix, ratelimit.Align())
	permit, err = dailyLimiter.Take(fmt.Sprintf("%s:%s:%s", "email", verifyType, email))
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Failed to take limit")
	}
	if !dailyLimiter.ParsePermitState(permit) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TodaySendCountExceedsLimit), "send email too many requests")
	}
	m, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Email, email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindUserAuthMethodByOpenID error")
	}
	if verifyType == auth.Register && m.Id > 0 {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "mobile already bind")
	} else if verifyType == auth.Security && m.Id == 0 {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "mobile not bind")
	}

	var taskPayload taskqueue.SendEmailPayload
	taskPayload.Metadata, _ = requestmeta.From(l.ctx)
	// Generate verification code
	code := random.Key(6, 0)
	expireSeconds := l.deps.Config.VerifyCodeExpire
	if expireSeconds <= 0 {
		expireSeconds = IntervalTime * 5
	}
	taskPayload.Type = taskqueue.EmailTypeVerify
	taskPayload.Email = email
	taskPayload.Subject = mail.DefaultEmailVerifySubject
	taskPayload.Content = map[string]interface{}{
		"Type":     req.Type,
		"SiteLogo": l.deps.Config.SiteLogo,
		"SiteName": l.deps.Config.SiteName,
		"Expire":   (expireSeconds + 59) / 60,
		"Code":     code,
	}
	expiration := time.Duration(expireSeconds) * time.Second
	if err = verification.SaveVerificationCode(l.ctx, l.deps.Redis, cacheKey, code, expiration); err != nil {
		l.Errorw("[SendEmailCode]: Redis Error", logger.Field("error", err.Error()), logger.Field("cacheKey", cacheKey))
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), "Failed to set verification code")
	}

	// Marshal the task payload
	payloadBuy, err := json.Marshal(taskPayload)
	if err != nil {
		l.Errorw("[SendEmailCode]: Marshal Error", logger.Field("error", err.Error()))
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), "Failed to marshal task payload")
	}
	// Create a queue task
	task := asynq.NewTask(taskqueue.ForthwithSendEmail, payloadBuy)
	// Enqueue the task
	taskInfo, err := l.deps.Queue.EnqueueContext(l.ctx, task, asynq.MaxRetry(3))
	if err != nil {
		_ = verification.DeleteVerificationCode(l.ctx, l.deps.Redis, cacheKey)
		l.Errorw("[SendEmailCode]: Enqueue Error", logger.Field("error", err.Error()), logger.Field("type", taskPayload.Type))
		return nil, errors.Wrap(xerr.NewErrCode(xerr.ERROR), "Failed to enqueue task")
	}
	l.Infow("[SendEmailCode]: Enqueue Success", logger.Field("taskID", taskInfo.ID), logger.Field("type", taskPayload.Type))
	return &dto.SendCodeResponse{Status: true}, nil
}
