package auth

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

func upgradePasswordAfterLogin(ctx context.Context, users repository.UserRepo, log logger.Logger, userInfo *user.User, plainPassword string) {
	if userInfo == nil || userInfo.Id == 0 || plainPassword == "" {
		return
	}
	if !password.PasswordNeedsRehash(userInfo.Algo, userInfo.Password) {
		return
	}

	nextHash := password.EncodePassWord(plainPassword)
	updated, err := users.UpgradePasswordHash(ctx, userInfo.Id, userInfo.Password, nextHash, password.PasswordAlgoArgon2id, "")
	if err != nil {
		log.Errorw("failed to upgrade password hash",
			logger.Field("user_id", userInfo.Id),
			logger.Field("error", err.Error()),
		)
		return
	}
	if !updated {
		return
	}
	userInfo.Password = nextHash
	userInfo.Algo = password.PasswordAlgoArgon2id
	userInfo.Salt = ""
}
