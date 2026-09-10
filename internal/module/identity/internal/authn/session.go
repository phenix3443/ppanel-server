package auth

import (
	"context"
	"fmt"
	"strconv"
	"time"
	"uuid"

	"github.com/perfect-panel/server/internal/auth/devicesession"
	token2 "github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

func bindLoginDevice(binder DeviceBinder, identifier, ip, ua string, userID int64) (*user.Device, error) {
	if identifier == "" {
		return nil, nil
	}
	if binder == nil {
		return nil, errors.New("device binder is unavailable")
	}
	device, err := binder.BindDeviceToUser(identifier, ip, ua, userID)
	if err != nil {
		return nil, err
	}
	if device == nil || device.Id <= 0 || device.UserId != userID || !device.Enabled {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "invalid device binding")
	}
	return device, nil
}

func issueLoginSession(ctx context.Context, client *redis.Client, secret string, lifetime, userID int64, loginType string, device *user.Device) (*dto.LoginResponse, error) {
	if value, ok := ctx.Value(requestctx.LoginType).(string); ok {
		loginType = value
	}
	if loginType == "device" && device == nil {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device session requires a binding")
	}
	if client == nil || lifetime <= 0 {
		return nil, errors.New("session store unavailable")
	}
	sessionID := uuid.NewV7().String()
	options := []token2.Option{token2.WithOption("UserId", userID), token2.WithOption("SessionId", sessionID), token2.WithOption("LoginType", loginType)}
	if device != nil {
		if device.Id <= 0 || device.UserId != userID || !device.Enabled {
			return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device session binding invalid")
		}
		epoch, err := devicesession.AcquireEpoch(ctx, client, device.Id)
		if err != nil {
			return nil, err
		}
		options = append(options, token2.WithOption(devicesession.IDClaim, strconv.FormatInt(device.Id, 10)), token2.WithOption(devicesession.EpochClaim, epoch))
	}
	token, err := token2.NewJwtToken(secret, timeutil.Now().Unix(), lifetime, options...)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionID)
	if err := client.Set(ctx, key, userID, time.Duration(lifetime)*time.Second).Err(); err != nil {
		return nil, err
	}
	return &dto.LoginResponse{Token: token}, nil
}
