package schema

import (
	"errors"
	"time"

	"github.com/perfect-panel/server/internal/auth/identifier"
	password2 "github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"gorm.io/gorm"
)

var errInvalidAdminEmail = errors.New("invalid admin email")

func canonicalAdminEmail(email string) (string, error) {
	canonicalEmail := identifier.CanonicalEmail(email)
	if canonicalEmail == "" {
		return "", errInvalidAdminEmail
	}
	return canonicalEmail, nil
}

// CreateAdminUser create admin user
func CreateAdminUser(email, password string, tx *gorm.DB) error {
	enable := true
	return tx.Transaction(func(tx *gorm.DB) error {
		// Prevent duplicate creation
		if tx.Model(&user.User{}).Find(&user.User{}).RowsAffected != 0 {
			logger.Info("User already exists, skip creating administrator account")
			return nil
		}
		canonicalEmail, err := canonicalAdminEmail(email)
		if err != nil {
			return err
		}

		u := user.User{
			Password:  password2.EncodePassWord(password),
			Algo:      password2.PasswordAlgoArgon2id,
			IsAdmin:   &enable,
			ReferCode: user.GenerateInviteCode(time.Now().Unix()),
		}
		if err := tx.Model(&user.User{}).Save(&u).Error; err != nil {
			return err
		}
		method := user.AuthMethods{
			UserId:         u.Id,
			AuthType:       identifier.Email,
			AuthIdentifier: canonicalEmail,
			Verified:       true,
		}
		if err := tx.Model(&user.AuthMethods{}).Save(&method).Error; err != nil {
			return err
		}
		return nil
	})
}
