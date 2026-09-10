// Package adminpayment implements the payment-method management subdomain of
// the billing module. Only the module facade may reach it.
package adminpayment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	paymentModel "github.com/perfect-panel/server/internal/module/billing/entity/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/stripe"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// Transactor mirrors the facade's billing-scoped transaction port.
type Transactor interface {
	InBillingTx(ctx context.Context, fn func(repository.BillingStore) error) error
}

type Service struct {
	payments repository.PaymentRepo
	orders   repository.OrderRepo
	tx       Transactor
	host     string
}

func NewService(payments repository.PaymentRepo, orders repository.OrderRepo, tx Transactor, host string) *Service {
	return &Service{payments: payments, orders: orders, tx: tx, host: host}
}

func (s *Service) Create(ctx context.Context, req *dto.CreatePaymentMethodRequest) (*dto.PaymentConfig, error) {
	log := logger.WithContext(ctx)
	if payment.ParsePlatform(req.Platform) == payment.UNSUPPORTED {
		log.Errorw("unsupported payment platform", logger.Field("mark", req.Platform))
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "UNSUPPORTED_PAYMENT_PLATFORM"), "unsupported payment platform: %s", req.Platform)
	}
	if err := validatePaymentFee(req.FeeMode, req.FeePercent, req.FeeAmount); err != nil {
		return nil, err
	}
	config := parsePaymentPlatformConfig(ctx, payment.ParsePlatform(req.Platform), req.Config)
	if config == "" {
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_PAYMENT_CONFIG"), "invalid payment config")
	}
	var paymentMethod = &paymentModel.Payment{
		Name:        req.Name,
		Platform:    req.Platform,
		Icon:        req.Icon,
		Domain:      req.Domain,
		Description: req.Description,
		Config:      config,
		FeeMode:     req.FeeMode,
		FeePercent:  req.FeePercent,
		FeeAmount:   req.FeeAmount,
		Sort:        req.Sort,
		Enable:      req.Enable,
		Token:       random.KeyNew(8, 1),
	}
	err := s.tx.InBillingTx(ctx, func(store repository.BillingStore) error {
		if req.Platform == "Stripe" {
			var cfg paymentModel.StripeConfig
			if err := cfg.Unmarshal([]byte(paymentMethod.Config)); err != nil {
				log.Errorf("[CreatePaymentMethod] unmarshal stripe config error: %s", err.Error())
				return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "unmarshal stripe config error: %s", err.Error())
			}
			if cfg.SecretKey == "" {
				log.Error("[CreatePaymentMethod] stripe secret key is empty")
				return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "stripe secret key is empty")
			}

			// Create Stripe webhook endpoint
			client := stripe.NewClient(stripe.Config{
				SecretKey: cfg.SecretKey,
				PublicKey: cfg.PublicKey,
			})
			url := fmt.Sprintf("%s/v1/notify/Stripe/%s", strings.TrimSuffix(req.Domain, "/"), paymentMethod.Token)
			endpoint, err := client.CreateWebhookEndpoint(url)
			if err != nil {
				log.Errorw("[CreatePaymentMethod] create stripe webhook endpoint error", logger.Field("error", err.Error()))
				return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "create stripe webhook endpoint error: %s", err.Error())
			}
			cfg.WebhookSecret = endpoint.Secret
			content, _ := cfg.Marshal()
			paymentMethod.Config = string(content)
		}
		if err := store.Payment().Insert(ctx, paymentMethod); err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert payment method error: %s", err.Error())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	resp := &dto.PaymentConfig{}
	mapping.DeepCopy(resp, paymentMethod)
	var configMap map[string]interface{}
	_ = json.Unmarshal([]byte(paymentMethod.Config), &configMap)
	resp.Config = configMap
	return resp, nil
}

func (s *Service) Update(ctx context.Context, req *dto.UpdatePaymentMethodRequest) (*dto.PaymentConfig, error) {
	log := logger.WithContext(ctx)
	if payment.ParsePlatform(req.Platform) == payment.UNSUPPORTED {
		log.Errorw("unsupported payment platform", logger.Field("mark", req.Platform))
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "UNSUPPORTED_PAYMENT_PLATFORM"), "unsupported payment platform: %s", req.Platform)
	}
	method, err := s.payments.FindOne(ctx, req.Id)
	if err != nil {
		log.Errorw("find payment method error", logger.Field("id", req.Id), logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find payment method error: %s", err.Error())
	}
	if method.Platform != req.Platform {
		return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "PAYMENT_PLATFORM_IMMUTABLE"), "payment platform cannot be changed")
	}
	if err := validatePaymentFee(req.FeeMode, req.FeePercent, req.FeeAmount); err != nil {
		return nil, err
	}
	if req.Sort == 0 {
		req.Sort = method.Sort
	}
	// Balance is an internal checkout method: it has no gateway credentials
	// to validate and no callback URL, so toggling it must neither parse a
	// platform config nor be blocked by the pending-order guard.
	config := method.Config
	if payment.ParsePlatform(req.Platform) != payment.Balance {
		config = parsePaymentPlatformConfig(ctx, payment.ParsePlatform(req.Platform), req.Config)
		if config == "" {
			return nil, errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_PAYMENT_CONFIG"), "invalid payment config")
		}
		if method.Config != config || method.Domain != req.Domain {
			pending, err := s.orders.CountPendingByPaymentID(ctx, method.Id)
			if err != nil {
				return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "count pending payment orders: %v", err)
			}
			if pending > 0 {
				return nil, errors.Wrapf(xerr.NewErrCodeMsg(409, "PAYMENT_METHOD_HAS_PENDING_ORDERS"), "payment method has %d pending orders", pending)
			}
		}
	}
	mapping.DeepCopy(method, req)
	method.Config = config
	if err := s.payments.Update(ctx, method); err != nil {
		log.Errorw("update payment method error", logger.Field("id", req.Id), logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update payment method error: %s", err.Error())
	}
	resp := &dto.PaymentConfig{}
	mapping.DeepCopy(resp, method)
	var configMap map[string]interface{}
	_ = json.Unmarshal([]byte(method.Config), &configMap)
	resp.Config = configMap
	return resp, nil
}

func (s *Service) Delete(ctx context.Context, req *dto.DeletePaymentMethodRequest) error {
	method, err := s.payments.FindOne(ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find payment method error: %v", err)
	}
	// The seeded balance method is an internal checkout method the storefront
	// depends on; deleting it breaks every balance purchase with an opaque
	// record-not-found until the seed row is restored by hand.
	if payment.ParsePlatform(method.Platform) == payment.Balance {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "PAYMENT_METHOD_INTERNAL"), "the balance payment method cannot be deleted")
	}
	pending, err := s.orders.CountPendingByPaymentID(ctx, req.Id)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "count pending payment orders: %v", err)
	}
	if pending > 0 {
		return errors.Wrapf(xerr.NewErrCodeMsg(409, "PAYMENT_METHOD_HAS_PENDING_ORDERS"), "payment method has %d pending orders", pending)
	}
	if err := s.payments.Delete(ctx, req.Id); err != nil {
		logger.WithContext(ctx).Errorw("delete payment method error", logger.Field("id", req.Id), logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete payment method error: %s", err.Error())
	}
	return nil
}

func (s *Service) List(ctx context.Context, req *dto.GetPaymentMethodListRequest) (*dto.GetPaymentMethodListResponse, error) {
	total, list, err := s.payments.FindListByPage(ctx, req.Page, req.Size, &paymentModel.Filter{
		Search: req.Search,
		Mark:   req.Platform,
		Enable: req.Enable,
	})
	if err != nil {
		logger.WithContext(ctx).Errorw("find payment method list error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find payment method list error: %s", err.Error())
	}
	resp := &dto.GetPaymentMethodListResponse{
		Total: total,
		List:  make([]dto.PaymentMethodDetail, len(list)),
	}

	for i, v := range list {
		config := make(map[string]interface{})
		_ = json.Unmarshal([]byte(v.Config), &config)
		notifyUrl := ""

		if payment.ParsePlatform(v.Platform) != payment.Balance {
			notifyUrl = v.Domain
			if notifyUrl == "" {
				notifyUrl = "https://" + s.host
			}
			notifyUrl = strings.TrimSuffix(notifyUrl, "/") + "/v1/notify/" + v.Platform + "/" + v.Token
		}
		resp.List[i] = dto.PaymentMethodDetail{
			Id:          v.Id,
			Name:        v.Name,
			Platform:    v.Platform,
			Icon:        v.Icon,
			Domain:      v.Domain,
			Config:      config,
			FeeMode:     v.FeeMode,
			FeePercent:  v.FeePercent,
			FeeAmount:   v.FeeAmount,
			Sort:        v.Sort,
			Enable:      *v.Enable,
			NotifyURL:   notifyUrl,
			Description: v.Description,
		}
	}
	return resp, nil
}

func (s *Service) Platforms(_ context.Context) (*dto.PaymentPlatformResponse, error) {
	return &dto.PaymentPlatformResponse{List: payment.GetSupportedPlatforms()}, nil
}

func validatePaymentFee(mode uint, percent, amount int64) error {
	if mode > 3 || percent < 0 || percent > 100 || amount < 0 {
		return errors.Wrapf(xerr.NewErrCodeMsg(400, "INVALID_PAYMENT_FEE"), "invalid payment fee configuration")
	}
	return nil
}

func parsePaymentPlatformConfig(ctx context.Context, platform payment.Platform, config interface{}) string {
	data, err := json.Marshal(config)
	if err != nil {
		logger.WithContext(ctx).Errorw("marshal config error", logger.Field("platform", platform), logger.Field("error", err.Error()))
		return ""
	}

	// 通用处理函数
	handleConfig := func(name string, target interface {
		Unmarshal([]byte) error
		Marshal() ([]byte, error)
	}) string {
		if err = target.Unmarshal(data); err != nil {
			logger.WithContext(ctx).Errorw("parse "+name+" config error", logger.Field("error", err.Error()))
			return ""
		}
		content, err := target.Marshal()
		if err != nil {
			logger.WithContext(ctx).Errorw("marshal "+name+" config error", logger.Field("error", err.Error()))
			return ""
		}
		return string(content)
	}

	switch platform {
	case payment.Stripe:
		return handleConfig("Stripe", &paymentModel.StripeConfig{})
	case payment.AlipayF2F:
		return handleConfig("Alipay", &paymentModel.AlipayF2FConfig{})
	case payment.EPay:
		return handleConfig("Epay", &paymentModel.EPayConfig{})
	case payment.Cryptomus:
		var cfg paymentModel.CryptomusConfig
		if err := cfg.Unmarshal(data); err != nil {
			return ""
		}
		cfg.MerchantID = strings.TrimSpace(cfg.MerchantID)
		cfg.APIKey = strings.TrimSpace(cfg.APIKey)
		if cfg.MerchantID == "" || cfg.APIKey == "" {
			return ""
		}
		content, err := cfg.Marshal()
		if err != nil {
			return ""
		}
		return string(content)
	default:
		return ""
	}
}
