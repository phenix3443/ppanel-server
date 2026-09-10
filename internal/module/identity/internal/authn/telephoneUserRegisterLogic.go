package auth

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type TelephoneUserRegisterLogic struct {
	logger.Logger
	ctx  context.Context
	deps TelephoneUserRegisterDependencies
}

// NewTelephoneUserRegisterLogic User Telephone register
func NewTelephoneUserRegisterLogic(ctx context.Context, deps TelephoneUserRegisterDependencies) *TelephoneUserRegisterLogic {
	return &TelephoneUserRegisterLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TelephoneUserRegisterLogic) TelephoneUserRegister(req *dto.TelephoneRegisterRequest) (resp *dto.LoginResponse, err error) {
	if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, identifier.Mobile); err != nil {
		return nil, err
	}
	if err := l.deps.Policy.VerifyHuman(l.ctx, req.CfToken, req.IP); err != nil {
		return nil, err
	}
	if !identifier.Check(req.TelephoneAreaCode, req.Telephone) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "telephone number error")
	}

	phoneNumber, err := identifier.FormatToE164(req.TelephoneAreaCode, req.Telephone)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}

	// if the email verification is enabled, the verification code is required
	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeTelephoneCacheKey, auth.ParseVerifyType(uint8(auth.Register)), phoneNumber)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}
	// Check if the user exists
	_, err = l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Mobile, phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("FindOneByTelephone Error", logger.Field("error", err))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "telephone already exists")
	}
	var referer *user.User
	if req.Invite == "" {
		if l.deps.Config.InviteForced {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.InviteCodeError), "invite code is required")
		}
	} else {
		// Check if the invite code is valid
		referer, err = l.deps.Store.User().FindOneByReferCode(l.ctx, req.Invite)
		if err != nil {
			l.Errorw("FindOneByReferCode Error", logger.Field("error", err))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.InviteCodeError), "invite code is invalid")
		}
	}
	if err := l.deps.Policy.TakeIPPermit(l.ctx, req.IP); err != nil {
		return nil, err
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}

	// Generate password
	pwd := password.EncodePassWord(req.Password)
	userInfo := &user.User{
		Password:          pwd,
		Algo:              password.PasswordAlgoArgon2id,
		OnlyFirstPurchase: &l.deps.Config.OnlyFirstPurchase,
		AuthMethods: []user.AuthMethods{
			{
				AuthType:       identifier.Mobile,
				AuthIdentifier: phoneNumber,
				Verified:       true,
			},
		},
	}
	if referer != nil {
		userInfo.RefererId = referer.Id
	}
	err = l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		// Save user information
		if err := store.User().Insert(l.ctx, userInfo); err != nil {
			return err
		}
		// Generate ReferCode
		userInfo.ReferCode = user.GenerateInviteCode(userInfo.Id)
		// Update ReferCode
		if err := store.User().Update(l.ctx, userInfo); err != nil {
			return err
		}
		// Registration emits the domain event; the subscription module
		// grants the trial when it consumes it (idempotent, retried by
		// the dispatcher).
		if err := store.Outbox().Append(l.ctx, "identity.user_registered", strconv.FormatInt(userInfo.Id, 10), "{}"); err != nil {
			return err
		}
		registerLog := log.Register{
			AuthMethod: "mobile",
			Identifier: logger.RedactedValue,
			RegisterIP: req.IP,
			UserAgent:  req.UserAgent,
			Timestamp:  timeutil.Now().UnixMilli(),
		}
		content, err := registerLog.Marshal()
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "marshal registration audit: %v", err)
		}
		if err := store.Log().Insert(l.ctx, &log.SystemLog{
			Type:     log.TypeRegister.Uint8(),
			ObjectID: userInfo.Id,
			Date:     timeutil.Now().Format(time.DateOnly),
			Content:  string(content),
		}); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "record registration audit: %v", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	device, err := bindLoginDevice(l.deps.DeviceBinder, req.Identifier, req.IP, req.UserAgent, userInfo.Id)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "bind device: %v", err)
	}
	session, err := issueLoginSession(l.ctx, l.deps.Redis, l.deps.Config.JWTAccessSecret, l.deps.Config.JWTAccessExpire, userInfo.Id, req.LoginType, device)
	if err != nil {
		return nil, err
	}
	token := session.Token

	defer func() {
		if token != "" && userInfo.Id != 0 {
			loginLog := log.Login{
				Method:    "mobile",
				LoginIP:   req.IP,
				UserAgent: req.UserAgent,
				Success:   token != "",
				Timestamp: timeutil.Now().UnixMilli(),
			}
			content, _ := loginLog.Marshal()
			if auditErr := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
				Id:       0,
				Type:     log.TypeLogin.Uint8(),
				Date:     timeutil.Now().Format("2006-01-02"),
				ObjectID: userInfo.Id,
				Content:  string(content),
			}); auditErr != nil {
				l.Errorw("failed to insert login log",
					logger.Field("user_id", userInfo.Id),
					logger.Field("ip", req.IP),
					logger.Field("error", auditErr.Error()),
				)
				if err == nil {
					resp = nil
					err = errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "record login audit: %v", auditErr)
				}
			}
		}
	}()
	return &dto.LoginResponse{
		Token: token,
	}, nil
}
