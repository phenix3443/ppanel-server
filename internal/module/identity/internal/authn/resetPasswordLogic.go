package auth

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type ResetPasswordLogic struct {
	logger.Logger
	ctx  context.Context
	deps ResetPasswordDependencies
}

// NewResetPasswordLogic Reset password
func NewResetPasswordLogic(ctx context.Context, deps ResetPasswordDependencies) *ResetPasswordLogic {
	return &ResetPasswordLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ResetPasswordLogic) ResetPassword(req *dto.ResetPasswordRequest) (resp *dto.LoginResponse, err error) {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Email); err != nil {
		return nil, err
	}
	var userInfo *user.User
	loginStatus := false
	email := identifier.CanonicalEmail(req.Email)

	defer func() {
		if userInfo != nil && userInfo.Id != 0 && loginStatus {
			loginLog := log.Login{
				Method:    "email",
				LoginIP:   req.IP,
				UserAgent: req.UserAgent,
				Success:   loginStatus,
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

	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeCacheKey, auth.Security, email)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "Verification code error")
	}

	// Check user
	authMethod, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Email, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user email not exist: %v", req.Email)
		}
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find user by email error: %v", err.Error())
	}

	userInfo, err = l.deps.Store.User().FindOne(l.ctx, authMethod.UserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user email not exist: %v", req.Email)
		}
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "Verification code error")
	}

	// Update password
	userInfo.Password = password.EncodePassWord(req.Password)
	userInfo.Algo = password.PasswordAlgoArgon2id
	userInfo.Salt = ""
	if err = l.deps.Store.User().Update(l.ctx, userInfo); err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update user info failed: %v", err.Error())
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
	loginStatus = true
	return &dto.LoginResponse{
		Token: token,
	}, nil
}
