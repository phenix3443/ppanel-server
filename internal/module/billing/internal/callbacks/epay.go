package callbacks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	payment2 "github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/epay"
	"github.com/perfect-panel/server/internal/module/billing/internal/settle"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// EPayNotifyMeta carries the raw callback transport details needed to verify
// the gateway signature.
type EPayNotifyMeta struct {
	Method string
	Params map[string]string
}

// EPayNotify authenticates and settles an EPay gateway callback.
func (s *Service) EPayNotify(ctx context.Context, meta EPayNotifyMeta, req *dto.EPayNotifyRequest) error {
	l := logger.WithContext(ctx)
	if meta.Params == nil {
		meta.Params = make(map[string]string)
	}
	if req == nil {
		return errors.New("callback request is missing")
	}
	data, ok := ctx.Value(requestctx.CtxKeyPayment).(*payment.Payment)
	if !ok {
		l.Error("[EPayNotify] Payment not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "payment config not found")
	}

	credentials, err := epayCredentialsForPayment(data)
	if err != nil {
		l.Errorw("[EPayNotify] Unmarshal config failed", logger.Field("error", err.Error()))
		return err
	}
	client := epay.NewClient(credentials.merchantID, credentials.endpoint, credentials.key, credentials.paymentType)
	if !client.VerifySign(meta.Params) {
		l.Error("[EPayNotify] Verify sign failed",
			logger.Field("orderNo", req.OutTradeNo),
			logger.Field("method", meta.Method),
		)
		return errors.New("verify sign failed")
	}
	callbackAmount, err := validateEPayCallback(req, meta.Params, credentials)
	if err != nil {
		l.Error("[EPayNotify] Callback validation failed", logger.Field("orderNo", req.OutTradeNo), logger.Field("error", err.Error()))
		return err
	}

	orderInfo, err := s.orders.FindOneByOrderNo(ctx, req.OutTradeNo)
	if err != nil {
		l.Error("[EPayNotify] Find order failed", logger.Field("error", err.Error()), logger.Field("orderNo", req.OutTradeNo))
		return errors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order not exist: %v", req.OutTradeNo)
	}
	if err := validateOrderPayment(orderInfo, data); err != nil {
		l.Error("[EPayNotify] Order payment binding failed", logger.Field("orderNo", req.OutTradeNo), logger.Field("error", err.Error()))
		return err
	}
	if finished, err := finishedOrderDuplicate(ctx, orderInfo, req.TradeNo); err != nil {
		return err
	} else if finished {
		return nil
	}
	if err := validateOrderCanSettle(orderInfo); err != nil {
		return err
	}
	if err := validatePaymentExpectation(orderInfo, callbackAmount, "CNY"); err != nil {
		l.Error("[EPayNotify] Payment amount validation failed", logger.Field("orderNo", req.OutTradeNo), logger.Field("error", err.Error()))
		return err
	}

	queried, err := client.QueryOrder(req.OutTradeNo)
	if err != nil {
		if errors.Is(err, epay.ErrQueryNotSupported) {
			// This gateway does not implement the order query API.
			// The callback signature was already verified above, so it is safe
			// to proceed without the active confirmation step.
			l.Infow("[EPayNotify] Gateway does not support order query; accepting signature-verified callback",
				logger.Field("orderNo", req.OutTradeNo),
			)
		} else {
			l.Error("[EPayNotify] Gateway order query failed", logger.Field("orderNo", req.OutTradeNo), logger.Field("error", err.Error()))
			return err
		}
	} else {
		if err := validateQueriedEPayOrder(queried, req, credentials, callbackAmount); err != nil {
			l.Error("[EPayNotify] Gateway order validation failed", logger.Field("orderNo", req.OutTradeNo), logger.Field("error", err.Error()))
			return err
		}
	}

	if err := s.settle(ctx, orderInfo, req.TradeNo); err != nil {
		l.Error("[EPayNotify] Settle order failed", logger.Field("orderNo", req.OutTradeNo), logger.Field("error", err.Error()))
		return err
	}
	l.Info("[EPayNotify] Notify processed", logger.Field("orderNo", req.OutTradeNo))
	return nil
}

type epayCredentials struct {
	merchantID  string
	endpoint    string
	key         string
	paymentType string
}

func epayCredentialsForPayment(data *payment.Payment) (epayCredentials, error) {
	var result epayCredentials
	switch payment2.ParsePlatform(data.Platform) {
	case payment2.EPay:
		var config payment.EPayConfig
		if err := json.Unmarshal([]byte(data.Config), &config); err != nil {
			return result, err
		}
		result = epayCredentials{
			merchantID:  config.Pid,
			endpoint:    config.Url,
			key:         config.Key,
			paymentType: config.Type,
		}
	default:
		return result, errors.New("unsupported EPay callback platform")
	}
	if result.merchantID == "" || result.endpoint == "" || result.key == "" {
		return result, errors.New("incomplete payment configuration")
	}
	return result, nil
}

func validateEPayCallback(req *dto.EPayNotifyRequest, params map[string]string, credentials epayCredentials) (int64, error) {
	if req == nil {
		return 0, errors.New("callback request is missing")
	}
	fields := map[string]string{
		"pid":          req.Pid,
		"trade_no":     req.TradeNo,
		"out_trade_no": req.OutTradeNo,
		"type":         req.Type,
		"name":         req.Name,
		"money":        req.Money,
		"trade_status": req.TradeStatus,
		"param":        req.Param,
		"sign":         req.Sign,
		"sign_type":    req.SignType,
	}
	for name, value := range fields {
		if params[name] != value {
			return 0, fmt.Errorf("callback parameter mismatch: %s", name)
		}
	}
	if req.OutTradeNo == "" || len(req.OutTradeNo) > 255 || strings.TrimSpace(req.OutTradeNo) != req.OutTradeNo {
		return 0, errors.New("invalid order number")
	}
	if err := settle.ValidateTradeNo(req.TradeNo); err != nil {
		return 0, err
	}
	if req.Pid != credentials.merchantID {
		return 0, errors.New("merchant id mismatch")
	}
	if credentials.paymentType != "" && req.Type != credentials.paymentType {
		return 0, errors.New("payment type mismatch")
	}
	if req.TradeStatus != "TRADE_SUCCESS" {
		return 0, errors.New("trade status is not success")
	}
	if !strings.EqualFold(req.SignType, "MD5") {
		return 0, errors.New("unsupported signature type")
	}
	amount, err := epay.ParseMoney(req.Money)
	if err != nil {
		return 0, errors.New("invalid callback money")
	}
	return amount, nil
}

func validateQueriedEPayOrder(result *epay.QueryResult, req *dto.EPayNotifyRequest, credentials epayCredentials, callbackAmount int64) error {
	if result == nil || !result.Paid {
		return errors.New("gateway order is not paid")
	}
	// Some compatible gateways expose only a payment status from their query
	// endpoint. The callback above is still signature-verified and its payment
	// fields are validated before this point, so status confirmation is useful
	// without pretending omitted fields were verified by the query API.
	if result.StatusOnly {
		return nil
	}
	if result.OrderNo != req.OutTradeNo {
		return errors.New("gateway order number mismatch")
	}
	if result.TradeNo == "" || result.TradeNo != req.TradeNo {
		return errors.New("gateway trade number mismatch")
	}
	if result.MerchantID != credentials.merchantID {
		return errors.New("gateway merchant id mismatch")
	}
	if result.Type != req.Type || (credentials.paymentType != "" && result.Type != credentials.paymentType) {
		return errors.New("gateway payment type mismatch")
	}
	queriedAmount, err := epay.ParseMoney(result.Money)
	if err != nil || queriedAmount != callbackAmount {
		return errors.New("gateway payment amount mismatch")
	}
	return nil
}
