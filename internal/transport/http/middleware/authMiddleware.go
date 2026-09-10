package middleware

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/auth/devicesession"
	token2 "github.com/perfect-panel/server/internal/auth/token"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type AuthDeps struct {
	JWT   config.JwtAuth
	Redis *redis.Client
	Store repository.Store
}

func AuthMiddleware(deps AuthDeps) app.HandlerFunc {
	return func(ctx context.Context, requestCtx *app.RequestContext) {
		ctx, err := AuthenticateRequest(ctx, deps, string(requestCtx.GetHeader("Authorization")), string(requestCtx.Path()))
		if err != nil {
			httpx.HttpResult(requestCtx, nil, err)
			requestCtx.Abort()
			return
		}
		if metadata, ok := requestmeta.From(ctx); ok && metadata.ActorID > 0 {
			requestCtx.Set(requestActorIDKey, metadata.ActorID)
		}
		requestCtx.Next(ctx)
	}
}

// OptionalAuthMiddleware authenticates a request when it supplies an
// Authorization header, while preserving anonymous access for routes that
// intentionally support guest checkout.  Handlers on those routes must still
// explicitly require an authenticated user before operating on a user-owned
// resource.
func OptionalAuthMiddleware(deps AuthDeps) app.HandlerFunc {
	return func(ctx context.Context, requestCtx *app.RequestContext) {
		token := string(requestCtx.GetHeader("Authorization"))
		if token == "" {
			requestCtx.Next(ctx)
			return
		}

		authenticatedCtx, err := AuthenticateRequest(ctx, deps, token, string(requestCtx.Path()))
		if err != nil {
			httpx.HttpResult(requestCtx, nil, err)
			requestCtx.Abort()
			return
		}
		if metadata, ok := requestmeta.From(authenticatedCtx); ok && metadata.ActorID > 0 {
			requestCtx.Set(requestActorIDKey, metadata.ActorID)
		}
		requestCtx.Next(authenticatedCtx)
	}
}

func AuthenticateRequest(ctx context.Context, deps AuthDeps, token string, path string) (context.Context, error) {
	jwtConfig := deps.JWT
	if token == "" {
		logger.WithContext(ctx).Debug("[AuthMiddleware] Token Empty")
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.ErrorTokenEmpty), "Token Empty")
	}

	claims, err := token2.ParseJwtToken(token, jwtConfig.AccessSecret)
	if err != nil {
		logger.WithContext(ctx).Debug("[AuthMiddleware] ParseJwtToken", logger.Field("error", err.Error()))
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.ErrorTokenExpire), "Token Invalid")
	}

	loginType := ""
	if claims["LoginType"] != nil {
		loginType = claims["LoginType"].(string)
	}

	userId := int64(claims["UserId"].(float64))
	sessionId := claims["SessionId"].(string)
	sessionIdCacheKey := fmt.Sprintf("%v:%v", config.SessionIdKey, sessionId)
	value, err := deps.Redis.Get(ctx, sessionIdCacheKey).Result()
	if err != nil {
		logger.WithContext(ctx).Debug("[AuthMiddleware] Redis Get", logger.Field("error", err.Error()), logger.Field("sessionId", sessionId))
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}

	if value != fmt.Sprintf("%v", userId) {
		logger.WithContext(ctx).Debug("[AuthMiddleware] Invalid Access", logger.Field("userId", userId), logger.Field("sessionId", sessionId))
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	deviceID, epoch, err := devicesession.Binding(claims)
	if err != nil {
		return ctx, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device session must be renewed")
	}
	if deviceID != 0 {
		device, err := deps.Store.UserDevice().FindDeviceForAuth(ctx, deviceID)
		if err != nil || device == nil || !device.Enabled || device.UserId != userId {
			return ctx, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device is unavailable")
		}
		currentEpoch, err := devicesession.Epoch(ctx, deps.Redis, deviceID)
		if err != nil || epoch != currentEpoch {
			return ctx, errors.Wrap(xerr.NewErrCode(xerr.InvalidAccess), "device session revoked")
		}
	}

	userInfo, err := deps.Store.User().FindOne(ctx, userId)
	if err != nil {
		logger.WithContext(ctx).Debug("[AuthMiddleware] UserModel FindOne", logger.Field("error", err.Error()), logger.Field("userId", userId))
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Database Query Error")
	}
	if userInfo.DeletedAt.Valid {
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "User Deleted")
	}

	// Check if user is enabled
	if userInfo.Enable == nil || !*userInfo.Enable {
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.UserDisabled), "User Disabled")
	}

	paths := strings.Split(path, "/")
	if slices.Contains(paths, "admin") && (userInfo.IsAdmin == nil || !*userInfo.IsAdmin) {
		logger.WithContext(ctx).Debug("[AuthMiddleware] Not Admin User", logger.Field("userId", userId), logger.Field("sessionId", sessionId))
		return ctx, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}

	ctx = context.WithValue(ctx, requestctx.LoginType, loginType)
	ctx = context.WithValue(ctx, requestctx.CtxKeyUser, userInfo)
	ctx = context.WithValue(ctx, requestctx.CtxKeySessionID, sessionId)
	ctx = requestmeta.WithActor(ctx, userId)
	ctx = logger.ContextWithFields(ctx, logger.Field("actor_id", userId))
	return ctx, nil
}
