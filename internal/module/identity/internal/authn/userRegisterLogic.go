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

type UserRegisterLogic struct {
	logger.Logger
	ctx  context.Context
	deps UserRegisterDependencies
}

// NewUserRegisterLogic User register
func NewUserRegisterLogic(ctx context.Context, deps UserRegisterDependencies) *UserRegisterLogic {
	return &UserRegisterLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UserRegisterLogic) UserRegister(req *dto.UserRegisterRequest) (resp *dto.LoginResponse, err error) {

	canonicalEmail, err := identifier.ValidateEmail(req.Email, l.deps.Config.EmailDomainSuffixList, l.deps.Config.EmailEnableDomainSuffix)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid email: %v", err)
	}
	var referer *user.User
	if err := l.deps.Policy.EnsureRegistrationOpen(l.ctx, identifier.Email); err != nil {
		return nil, err
	}
	if err := l.deps.Policy.VerifyHuman(l.ctx, req.CfToken, req.IP); err != nil {
		return nil, err
	}

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

	// if the email verification is enabled, the verification code is required
	if l.deps.Config.EmailVerifyEnabled {
		cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeCacheKey, auth.Register, canonicalEmail)
		if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
		}
	}
	// Check if the user exists
	u, err := l.deps.Store.User().FindOneByEmail(l.ctx, canonicalEmail)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("FindOneByEmail Error", logger.Field("error", err))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query user info failed: %v", err.Error())
	} else if err == nil && !u.DeletedAt.Valid {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "user email exist: %v", req.Email)
	} else if err == nil && u.DeletedAt.Valid {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserDisabled), "user email deleted: %v", req.Email)
	}
	if err := l.deps.Policy.TakeIPPermit(l.ctx, req.IP); err != nil {
		return nil, err
	}
	if l.deps.Config.EmailVerifyEnabled {
		cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeCacheKey, auth.Register, canonicalEmail)
		if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
		}
	}

	// Generate password
	pwd := password.EncodePassWord(req.Password)
	userInfo := &user.User{
		Password:          pwd,
		Algo:              password.PasswordAlgoArgon2id,
		OnlyFirstPurchase: &l.deps.Config.OnlyFirstPurchase,
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
		// create user auth info
		authInfo := &user.AuthMethods{
			UserId:         userInfo.Id,
			AuthType:       identifier.Email,
			AuthIdentifier: canonicalEmail,
			Verified:       l.deps.Config.EmailVerifyEnabled,
		}
		if err = store.UserAuth().InsertUserAuthMethods(l.ctx, authInfo); err != nil {
			return err
		}

		// Registration emits the domain event; the subscription module
		// grants the trial when it consumes it (idempotent, retried by
		// the dispatcher).
		if err := store.Outbox().Append(l.ctx, "identity.user_registered", strconv.FormatInt(userInfo.Id, 10), "{}"); err != nil {
			return err
		}
		registerLog := log.Register{
			AuthMethod: "email",
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
	loginStatus := true
	defer func() {
		if token != "" && userInfo.Id != 0 {
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
	return &dto.LoginResponse{
		Token: token,
	}, nil
}
