package notify

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/logger"
)

const (
	maxStripePayloadSize    = 65_536
	maxCryptomusPayloadSize = 65_536
)

var errNotifyPayloadTooLarge = errors.New("http: request body too large")

// PaymentNotifyHandler documents Payment Notify.
//
// @Summary Payment Notify
// @Tags common
// @Accept json,x-www-form-urlencoded
// @Produce json
// @Param platform path string true "platform"
// @Param token path string true "token"
// @Success 200 {object} httpx.ResponseSuccessBean
// @Router /v1/notify/{platform}/{token} [delete]
// @Router /v1/notify/{platform}/{token} [get]
// @Router /v1/notify/{platform}/{token} [head]
// @Router /v1/notify/{platform}/{token} [options]
// @Router /v1/notify/{platform}/{token} [patch]
// @Router /v1/notify/{platform}/{token} [post]
// @Router /v1/notify/{platform}/{token} [put]
func PaymentNotifyHandler(service billing.Service) app.HandlerFunc {
	return func(c context.Context, ctx *app.RequestContext) {
		platform, ok := c.Value(requestctx.CtxKeyPlatform).(string)
		if !ok {
			logger.WithContext(c).Errorf("platform not found")
			httpx.HttpResult(ctx, nil, fmt.Errorf("platform not found"))
			return
		}

		switch payment.ParsePlatform(platform) {
		case payment.EPay:
			params, err := uniqueFormValues(nativeFormValues(ctx))
			if err != nil {
				logger.WithContext(c).Errorw("[PaymentNotifyHandler] ShouldBind failed", logger.Field("error", err.Error()))
				ctx.String(consts.StatusBadRequest, "invalid request")
				return
			}
			req := epayNotifyRequest(params)
			if err := service.EPayNotify(c, billing.EPayNotifyMeta{
				Method: string(ctx.Method()),
				Params: params,
			}, req); err != nil {
				logger.WithContext(c).Errorf("EPayNotify failed: %v", err.Error())
				ctx.String(consts.StatusBadRequest, err.Error())
				return
			}
			ctx.String(consts.StatusOK, "success")
		case payment.Stripe:
			payload, err := stripePayload(ctx.Request.Body())
			if err != nil {
				httpx.HttpResult(ctx, nil, err)
				return
			}
			if err := service.StripeNotify(c, payload, string(ctx.GetHeader("Stripe-Signature"))); err != nil {
				httpx.HttpResult(ctx, nil, err)
				return
			}
			httpx.HttpResult(ctx, nil, nil)

		case payment.AlipayF2F:
			if err := service.AlipayNotify(c, nativeFormValues(ctx)); err != nil {
				httpx.HttpResult(ctx, nil, err)
				return
			}
			// Return success to alipay
			ctx.String(consts.StatusOK, "success")

		case payment.Cryptomus:
			payload, err := cryptomusPayload(ctx.Request.Body())
			if err != nil {
				httpx.HttpResult(ctx, nil, err)
				return
			}
			if err := service.CryptomusNotify(c, payload); err != nil {
				logger.WithContext(c).Errorf("CryptomusNotify failed: %v", err.Error())
				ctx.String(consts.StatusBadRequest, err.Error())
				return
			}
			ctx.String(consts.StatusOK, "success")

		default:
			logger.WithContext(c).Errorf("platform %s not support", platform)
			ctx.String(consts.StatusBadRequest, "unsupported payment platform")
		}
	}
}

func nativeFormValues(ctx *app.RequestContext) url.Values {
	values := make(url.Values)
	ctx.PostArgs().VisitAll(func(key, value []byte) {
		values.Add(string(key), string(value))
	})
	ctx.QueryArgs().VisitAll(func(key, value []byte) {
		values.Add(string(key), string(value))
	})
	return values
}

func stripePayload(payload []byte) ([]byte, error) {
	if len(payload) > maxStripePayloadSize {
		return nil, errNotifyPayloadTooLarge
	}
	return payload, nil
}

func cryptomusPayload(payload []byte) ([]byte, error) {
	if len(payload) > maxCryptomusPayloadSize {
		return nil, errNotifyPayloadTooLarge
	}
	return payload, nil
}

func uniqueFormValues(values url.Values) (map[string]string, error) {
	params := make(map[string]string, len(values))
	for key, value := range values {
		if len(value) != 1 {
			return nil, fmt.Errorf("callback parameter %q must occur exactly once", key)
		}
		params[key] = value[0]
	}
	return params, nil
}

func epayNotifyRequest(params map[string]string) *dto.EPayNotifyRequest {
	return &dto.EPayNotifyRequest{
		Pid:         params["pid"],
		TradeNo:     params["trade_no"],
		OutTradeNo:  params["out_trade_no"],
		Type:        params["type"],
		Name:        params["name"],
		Money:       params["money"],
		TradeStatus: params["trade_status"],
		Param:       params["param"],
		Sign:        params["sign"],
		SignType:    params["sign_type"],
	}
}
