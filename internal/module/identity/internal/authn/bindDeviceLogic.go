package auth

import (
	"context"
	"strings"
	"unicode/utf8"

	identifier2 "github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

type BindDeviceLogic struct {
	logger.Logger
	ctx  context.Context
	deps BindDeviceDependencies
}

func NewBindDeviceLogic(ctx context.Context, deps BindDeviceDependencies) *BindDeviceLogic {
	return &BindDeviceLogic{Logger: logger.WithContext(ctx), ctx: ctx, deps: deps}
}

// maxUserAgentLength matches the user_device.user_agent column (02149).
const maxUserAgentLength = 512

func truncateUserAgent(ua string) string {
	if len(ua) <= maxUserAgentLength {
		return ua
	}
	cut := ua[:maxUserAgentLength]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// BindDeviceToUser creates a binding or refreshes the current owner's device.
// An identifier is not proof of ownership: never move another user's binding
// or disable that account. Anonymous users can add email/mobile credentials
// through authenticated profile binding without abandoning their account.
func (l *BindDeviceLogic) BindDeviceToUser(identifier, ip, userAgent string, userID int64) (*user.Device, error) {
	if identifier == "" {
		return nil, nil
	}
	if userID <= 0 || len(identifier) > 255 || strings.TrimSpace(identifier) != identifier {
		return nil, xerr.NewErrCode(xerr.InvalidParams)
	}
	userAgent = truncateUserAgent(userAgent)
	device, err := l.deps.Store.UserDevice().FindOneDeviceByIdentifier(l.ctx, identifier)
	if err == nil {
		return l.existingDevice(device, userID, ip, userAgent)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	device = &user.Device{UserId: userID, Identifier: identifier, Ip: ip, UserAgent: userAgent, Enabled: true}
	err = l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		if err := store.UserAuth().InsertUserAuthMethods(l.ctx, &user.AuthMethods{
			UserId: userID, AuthType: identifier2.Device, AuthIdentifier: identifier, Verified: true,
		}); err != nil {
			return err
		}
		return store.UserDevice().InsertDevice(l.ctx, device)
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		// A concurrent creator may only be reused if it belongs to this user.
		existing, queryErr := l.deps.Store.UserDevice().FindOneDeviceByIdentifier(l.ctx, identifier)
		if queryErr != nil {
			return nil, queryErr
		}
		return l.existingDevice(existing, userID, ip, userAgent)
	}
	if err != nil {
		return nil, err
	}
	return device, nil
}

func (l *BindDeviceLogic) existingDevice(device *user.Device, userID int64, ip, userAgent string) (*user.Device, error) {
	if device == nil || device.UserId != userID {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device belongs to another account")
	}
	if !device.Enabled {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device is disabled")
	}
	updated, err := l.deps.Store.UserDevice().TouchDevice(l.ctx, device.Id, userID, ip, userAgent)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device binding changed")
	}
	device.Ip, device.UserAgent = ip, userAgent
	return device, nil
}
