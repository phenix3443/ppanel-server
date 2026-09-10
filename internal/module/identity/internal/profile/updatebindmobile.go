package profile

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type UpdateBindMobileLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update Bind Mobile
func newUpdateBindMobileLogic(ctx context.Context, deps Deps) *UpdateBindMobileLogic {
	return &UpdateBindMobileLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateBindMobileLogic) UpdateBindMobile(req *dto.UpdateBindMobileRequest) error {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Mobile); err != nil {
		return err
	}
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	// verify mobile
	phoneNumber, err := identifier.FormatToE164(req.AreaCode, req.Mobile)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}
	cacheKey := fmt.Sprintf("%s:%s:%s", config.AuthCodeTelephoneCacheKey, auth.Register, phoneNumber)
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, false); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}

	m, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "mobile", phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindUserAuthMethodByOpenID error")
	}
	if m.Id > 0 {
		return errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "mobile already bind")
	}

	method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "mobile", u.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindUserAuthMethodByOpenID error")
	}
	if err := verification.ValidateVerificationCode(l.ctx, l.deps.Redis, cacheKey, req.Code, true); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.VerifyCodeError), "code error")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		method = &user.AuthMethods{
			UserId:         u.Id,
			AuthType:       "mobile",
			AuthIdentifier: phoneNumber,
			Verified:       true,
		}
		if err := l.deps.UserAuth.InsertUserAuthMethods(l.ctx, method); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "InsertUserAuthMethods error")
		}
	} else {
		method.Verified = true
		method.AuthIdentifier = phoneNumber
		if err := l.deps.UserAuth.UpdateUserAuthMethods(l.ctx, method); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "UpdateUserAuthMethods error")
		}
	}
	return nil
}
