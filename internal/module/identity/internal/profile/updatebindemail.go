package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type UpdateBindEmailLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateBindEmailLogic Update Bind Email
func newUpdateBindEmailLogic(ctx context.Context, deps Deps) *UpdateBindEmailLogic {
	return &UpdateBindEmailLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateBindEmailLogic) UpdateBindEmail(req *dto.UpdateBindEmailRequest) error {
	if err := l.deps.Policy.EnsureMethodEnabled(l.ctx, identifier.Email); err != nil {
		return err
	}
	domainList, restrict := l.deps.EmailDomains()
	email, err := identifier.ValidateEmail(req.Email, domainList, restrict)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "invalid email: %v", err)
	}
	req.Email = email
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "email", u.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindUserAuthMethodByOpenID error")
	}
	m, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "email", req.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindUserAuthMethodByOpenID error")
	}
	// email already bind
	if m.Id > 0 {
		return errors.Wrapf(xerr.NewErrCode(xerr.UserExist), "email already bind")
	}
	if method.Id == 0 {
		method = &user.AuthMethods{
			UserId:         u.Id,
			AuthType:       "email",
			AuthIdentifier: req.Email,
			Verified:       false,
		}
		if err := l.deps.UserAuth.InsertUserAuthMethods(l.ctx, method); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "InsertUserAuthMethods error")
		}
	} else {
		method.Verified = false
		method.AuthIdentifier = req.Email
		if err := l.deps.UserAuth.UpdateUserAuthMethods(l.ctx, method); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "UpdateUserAuthMethods error")
		}
	}
	return nil
}
