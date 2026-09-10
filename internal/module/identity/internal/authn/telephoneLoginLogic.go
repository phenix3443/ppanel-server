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

type TelephoneLoginLogic struct {
	logger.Logger
	ctx  context.Context
	deps TelephoneLoginDependencies
}

// User Telephone login
func NewTelephoneLoginLogic(ctx context.Context, deps TelephoneLoginDependencies) *TelephoneLoginLogic {
	return &TelephoneLoginLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TelephoneLoginLogic) TelephoneLogin(req *dto.TelephoneLoginRequest, ip, userAgent string) (resp *dto.LoginResponse, err error) {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Mobile); err != nil {
		return nil, err
	}
	phoneNumber, err := identifier.FormatToE164(req.TelephoneAreaCode, req.Telephone)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}
	loginStatus := false

	authMethodInfo, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Mobile, phoneNumber)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user telephone not exist: %v", req.Telephone)
		}
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}

	userInfo, err := l.deps.Store.User().FindOne(l.ctx, authMethodInfo.UserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user telephone not exist: %v", req.Telephone)
		}
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if userInfo.DeletedAt.Valid {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user telephone deleted: %v", req.Telephone)
	}
	if !*userInfo.Enable {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserDisabled), "user account is disabled")
	}

	// Record login status
	defer func() {
		if userInfo.Id != 0 {
			loginLog := log.Login{
				Method:    "mobile",
				LoginIP:   ip,
				UserAgent: userAgent,
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

	if req.Password == "" && req.TelephoneCode == "" {
		return nil, xerr.NewErrCodeMsg(xerr.InvalidParams, "password and telephone code is empty")
	}

	if req.TelephoneCode == "" {
		// Verify password
		if !password.MultiPasswordVerify(userInfo.Algo, userInfo.Salt, req.Password, userInfo.Password) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserPasswordError), "user password")
		}
		upgradePasswordAfterLogin(l.ctx, l.deps.Store.User(), l.Logger, userInfo, req.Password)
	} else {
		cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeTelephoneCacheKey, auth.ParseVerifyType(uint8(auth.Security)), phoneNumber)
		if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.TelephoneCode, true); err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
		}
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
