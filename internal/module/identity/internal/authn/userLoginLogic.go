package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type UserLoginLogic struct {
	logger.Logger
	ctx  context.Context
	deps UserLoginDependencies
}

// NewUserLoginLogic User login
func NewUserLoginLogic(ctx context.Context, deps UserLoginDependencies) *UserLoginLogic {
	return &UserLoginLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UserLoginLogic) UserLogin(req *dto.UserLoginRequest) (resp *dto.LoginResponse, err error) {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Email); err != nil {
		return nil, err
	}
	email, err := identifier.ValidateEmail(req.Email, "", false)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid email: %v", err)
	}
	loginStatus := false
	var userInfo *user.User
	// Record login status
	defer func() {
		if userInfo != nil && userInfo.Id != 0 {
			loginLog := log.Login{
				Method:    "email",
				LoginIP:   req.IP,
				UserAgent: req.UserAgent,
				Success:   loginStatus,
				Timestamp: timeutil.Now().UnixMilli(),
			}
			content, _ := loginLog.Marshal()
			if auditErr := l.deps.Store.Log().Insert(l.ctx, &log.SystemLog{
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

	userInfo, err = l.deps.Store.User().FindOneByEmail(l.ctx, email)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user email not exist: %v", req.Email)
		}
		logger.WithContext(l.ctx).Error(err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	}
	if userInfo.DeletedAt.Valid {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user email deleted: %v", req.Email)
	}

	// Verify password
	if !password.MultiPasswordVerify(userInfo.Algo, userInfo.Salt, req.Password, userInfo.Password) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserPasswordError), "user password")
	}

	// Check if user is enabled
	if !*userInfo.Enable {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserDisabled), "user account is disabled")
	}
	upgradePasswordAfterLogin(l.ctx, l.deps.Store.User(), l.Logger, userInfo, req.Password)

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
