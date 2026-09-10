package auth

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type TelephoneResetPasswordLogic struct {
	logger.Logger
	ctx  context.Context
	deps TelephoneResetPasswordDependencies
}

// Reset password
func NewTelephoneResetPasswordLogic(ctx context.Context, deps TelephoneResetPasswordDependencies) *TelephoneResetPasswordLogic {
	return &TelephoneResetPasswordLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TelephoneResetPasswordLogic) TelephoneResetPassword(req *dto.TelephoneResetPasswordRequest) (resp *dto.LoginResponse, err error) {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Mobile); err != nil {
		return nil, err
	}
	code := req.Code

	phoneNumber, err := identifier.FormatToE164(req.TelephoneAreaCode, req.Telephone)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}

	// if the email verification is enabled, the verification code is required
	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeTelephoneCacheKey, auth.Security, phoneNumber)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, code, false); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}

	authMethods, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Mobile, phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("FindOneByTelephone Error", logger.Field("error", err))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if authMethods.UserId == 0 {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user telephone exist: %v", phoneNumber)
	}

	// Check if the user exists
	userInfo, err := l.deps.Store.User().FindOne(l.ctx, authMethods.UserId)
	if err != nil {
		l.Errorw("FindOneByTelephone Error", logger.Field("error", err))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, code, true); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}

	// Generate password
	pwd := password.EncodePassWord(req.Password)
	userInfo.Password = pwd
	userInfo.Algo = password.PasswordAlgoArgon2id
	userInfo.Salt = ""
	err = l.deps.Store.User().Update(l.ctx, userInfo)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "update user password failed: %v", err.Error())
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
