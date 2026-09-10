package callbacks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/alipay"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// AlipayNotify authenticates and settles an Alipay F2F notification.
func (s *Service) AlipayNotify(ctx context.Context, form url.Values) error {
	l := logger.WithContext(ctx)
	data, ok := ctx.Value(requestctx.CtxKeyPayment).(*payment.Payment)
	if !ok {
		return fmt.Errorf("payment config not found")
	}
	var config payment.AlipayF2FConfig
	if err := json.Unmarshal([]byte(data.Config), &config); err != nil {
		l.Error("[AlipayNotify] Unmarshal config failed", logger.Field("error", err.Error()))
		return err
	}
	client := alipay.NewClient(alipay.Config{
		AppId:       config.AppId,
		PrivateKey:  config.PrivateKey,
		PublicKey:   config.PublicKey,
		InvoiceName: config.InvoiceName,
		NotifyURL:   data.Domain + "/v1/payment/alipay/notify",
		Sandbox:     config.Sandbox,
		Gateway:     config.Gateway,
	})
	if client == nil {
		return errors.New("initialize Alipay client failed")
	}
	notify, err := client.DecodeNotification(form)
	if err != nil {
		l.Error("[AlipayNotify] Decode notification failed", logger.Field("error", err.Error()))
		return err
	}
	if notify.Status == alipay.Success || notify.Status == alipay.Finished {
		orderInfo, err := s.orders.FindOneByOrderNo(ctx, notify.OrderNo)
		if err != nil {
			l.Error("[AlipayNotify] Find order failed", logger.Field("error", err.Error()), logger.Field("orderNo", notify.OrderNo))
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order not exist: %v", notify.OrderNo)
		}

		if finished, err := validateAlipayCallback(ctx, orderInfo, data, &config, notify); err != nil {
			return err
		} else if finished {
			return nil
		}
		trade, err := client.QueryTrade(ctx, notify.OrderNo)
		if err != nil {
			return err
		}
		if !trade.Status.Paid() {
			return errors.New("Alipay trade is not paid")
		}
		if err := s.settle(ctx, orderInfo, notify.TradeNo); err != nil {
			return err
		}
		l.Info("[AlipayNotify] Notify status success", logger.Field("orderNo", notify.OrderNo))
	} else {
		l.Error("[AlipayNotify] Notify status failed", logger.Field("status", string(notify.Status)))
	}
	return nil
}

func validateAlipayCallback(ctx context.Context, orderInfo *order.Order, paymentConfig *payment.Payment, config *payment.AlipayF2FConfig, notify *alipay.Notification) (bool, error) {
	if notify == nil {
		return false, errors.New("Alipay callback is missing")
	}
	if err := validateOrderPayment(orderInfo, paymentConfig); err != nil {
		return false, err
	}
	if notify.AppId != config.AppId {
		return false, errors.New("Alipay app id mismatch")
	}
	if finished, err := finishedOrderDuplicate(ctx, orderInfo, notify.TradeNo); err != nil || finished {
		return finished, err
	}
	if err := validateOrderCanSettle(orderInfo); err != nil {
		return false, err
	}
	if err := validatePaymentExpectation(orderInfo, notify.Amount, "CNY"); err != nil {
		return false, err
	}
	return false, nil
}
