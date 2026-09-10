package middleware

import (
	"context"
	"errors"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/repository"
)

type PaymentParams struct {
	Platform string `uri:"platform"`
	Token    string `uri:"token"`
}

type PaymentStore interface {
	Payment() repository.PaymentRepo
}

func NotifyMiddleware(store PaymentStore) app.HandlerFunc {
	return func(ctx context.Context, requestCtx *app.RequestContext) {
		params := PaymentParams{
			Platform: requestCtx.Param("platform"),
			Token:    requestCtx.Param("token"),
		}
		ctx, err := PaymentNotifyContext(ctx, store, params.Platform, params.Token)
		if err != nil {
			requestCtx.JSON(400, map[string]string{"error": err.Error()})
			requestCtx.Abort()
			return
		}
		requestCtx.Next(ctx)
	}
}

func PaymentNotifyContext(ctx context.Context, store PaymentStore, platform, token string) (context.Context, error) {
	config, err := store.Payment().FindOneByPaymentToken(ctx, token)
	if err != nil {
		return ctx, err
	}
	if config.Platform != platform {
		return ctx, errors.New("payment callback platform mismatch")
	}
	ctx = context.WithValue(ctx, requestctx.CtxKeyPlatform, config.Platform)
	ctx = context.WithValue(ctx, requestctx.CtxKeyPayment, config)
	return ctx, nil
}
