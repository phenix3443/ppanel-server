package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type CheckUserTelephoneLogic struct {
	logger.Logger
	ctx  context.Context
	deps CheckUserDependencies
}

// Check user telephone is exist
func NewCheckUserTelephoneLogic(ctx context.Context, deps CheckUserDependencies) *CheckUserTelephoneLogic {
	return &CheckUserTelephoneLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CheckUserTelephoneLogic) CheckUserTelephone(req *dto.TelephoneCheckUserRequest) (resp *dto.TelephoneCheckUserResponse, err error) {
	phoneNumber, err := identifier.FormatToE164(req.TelephoneAreaCode, req.Telephone)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.TelephoneError), "Invalid phone number")
	}
	authMethods, err := l.deps.Store.UserAuth().FindUserAuthMethodByOpenID(l.ctx, identifier.Mobile, phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find user by email error: %v", err.Error())
	}

	return &dto.TelephoneCheckUserResponse{
		Exist: authMethods.UserId != 0,
	}, nil
}
