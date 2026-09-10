package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserPasswordLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update User Password
func newUpdateUserPasswordLogic(ctx context.Context, deps Deps) *UpdateUserPasswordLogic {
	return &UpdateUserPasswordLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserPasswordLogic) UpdateUserPassword(req *dto.UpdateUserPasswordRequest) error {
	userInfo := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	//update the password
	userInfo.Password = password.EncodePassWord(req.Password)
	// Reset algo to the current password algorithm, otherwise a migrated user
	// would keep verifying the new hash with the old legacy algorithm.
	userInfo.Algo = password.PasswordAlgoArgon2id
	userInfo.Salt = ""
	if err := l.deps.Users.Update(l.ctx, userInfo); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Update user password error")
	}
	return nil
}
