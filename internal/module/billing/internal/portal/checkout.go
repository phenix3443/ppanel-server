package portal

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/infra/taskqueue"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/billing/entity/payment"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/billing/internal/checkout"
	"github.com/perfect-panel/server/internal/module/billing/internal/exchangerate"
	payment2 "github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/alipay"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/cryptomus"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/epay"
	"github.com/perfect-panel/server/internal/module/billing/internal/payment/stripe"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

// PurchaseCheckoutLogic handles the checkout process for various payment methods
// including EPay, Stripe, Alipay F2F, Cryptomus, and balance payments
type PurchaseCheckoutLogic struct {
	logger.Logger
	ctx  context.Context
	deps CheckoutDependencies
}

// CheckoutDependencies contains the infrastructure ports required by the
// checkout use case. Keep it specific to this use case instead of passing the
// application-wide Application into business logic.
type CheckoutDependencies struct {
	Store              CheckoutStore
	GuestCheckoutCache GuestCheckoutCache
	ActivationQueue    ActivationQueue
	Config             CheckoutConfig
	ExchangeRateCache  ExchangeRateCache
}

// CheckoutConfig is the configuration snapshot consumed by checkout.
type CheckoutConfig struct {
	Host              string
	SiteName          string
	CurrencyUnit      string
	CurrencyAccessKey string
}

// GuestCheckoutCache provides the one Redis operation needed to validate
// legacy guest checkout capabilities.
type GuestCheckoutCache interface {
	Get(ctx context.Context, key string) *redis.StringCmd
}

// ActivationQueue publishes order activation tasks after a successful balance
// payment.
type ActivationQueue interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// ExchangeRateCache is shared with the rate refresh task. It is deliberately
// limited to the checkout use case's read/write needs.
type ExchangeRateCache interface {
	Get() float64
	Set(float64)
}

// CheckoutStore is the persistence port required by checkout. It prevents the
// use case from depending on the full repository facade.
type CheckoutStore interface {
	FindOrderByOrderNo(ctx context.Context, orderNo string) (*order.Order, error)
	FindPayment(ctx context.Context, id int64) (*payment.Payment, error)
	FindUser(ctx context.Context, id int64) (*user.User, error)
	UpdatePaymentExpectation(ctx context.Context, orderNo string, amount int64, currency string) (bool, error)
	SetPaymentTradeNoIfEmpty(ctx context.Context, orderNo, tradeNo string) (bool, error)
	UpdateOrderStatusFrom(ctx context.Context, orderNo string, from, status uint8) (bool, error)
	ClearUserCache(ctx context.Context, users ...*user.User) error
	InTx(ctx context.Context, fn func(CheckoutTransaction) error) error
}

// CheckoutTransaction is the subset of persistence operations that must share
// the balance-payment transaction.
type CheckoutTransaction interface {
	FindOrderByOrderNoForUpdate(ctx context.Context, orderNo string) (*order.Order, error)
	FindUserForUpdate(ctx context.Context, id int64) (*walletEntity.Wallet, error)
	UpdateUserBalance(ctx context.Context, data *walletEntity.Wallet) error
	InsertSystemLog(ctx context.Context, data *log.SystemLog) error
	UpdateOrder(ctx context.Context, data *order.Order) error
	UpdateOrderStatusFrom(ctx context.Context, orderNo string, from, status uint8) (bool, error)
}

type checkoutStore struct {
	store Store
}

type checkoutTransaction struct {
	store repository.BillingStore
}

// NewCheckoutStore adapts the application's repository facade at the
// composition boundary to the checkout use case's narrow persistence port.
func NewCheckoutStore(store Store) CheckoutStore {
	return checkoutStore{store: store}
}

func (s checkoutStore) FindOrderByOrderNo(ctx context.Context, orderNo string) (*order.Order, error) {
	return s.store.Order().FindOneByOrderNo(ctx, orderNo)
}

func (s checkoutStore) FindPayment(ctx context.Context, id int64) (*payment.Payment, error) {
	return s.store.Payment().FindOne(ctx, id)
}

func (s checkoutStore) FindUser(ctx context.Context, id int64) (*user.User, error) {
	return s.store.User().FindOne(ctx, id)
}

func (s checkoutStore) UpdatePaymentExpectation(ctx context.Context, orderNo string, amount int64, currency string) (bool, error) {
	return s.store.Order().UpdatePaymentExpectation(ctx, orderNo, amount, currency)
}

func (s checkoutStore) SetPaymentTradeNoIfEmpty(ctx context.Context, orderNo, tradeNo string) (bool, error) {
	return s.store.Order().SetPaymentTradeNoIfEmpty(ctx, orderNo, tradeNo)
}

func (s checkoutStore) UpdateOrderStatusFrom(ctx context.Context, orderNo string, from, status uint8) (bool, error) {
	return s.store.Order().UpdateOrderStatusFrom(ctx, orderNo, from, status)
}

func (s checkoutStore) ClearUserCache(ctx context.Context, users ...*user.User) error {
	return s.store.UserCache().ClearUserCache(ctx, users...)
}

func (s checkoutStore) InTx(ctx context.Context, fn func(CheckoutTransaction) error) error {
	// The balance payment touches orders, the wallet and audit logs only, so
	// the transaction is billing-scoped.
	return s.store.InBillingTx(ctx, func(store repository.BillingStore) error {
		return fn(checkoutTransaction{store: store})
	})
}

func (s checkoutTransaction) FindOrderByOrderNoForUpdate(ctx context.Context, orderNo string) (*order.Order, error) {
	return s.store.Order().FindOneByOrderNoForUpdate(ctx, orderNo)
}

func (s checkoutTransaction) FindUserForUpdate(ctx context.Context, id int64) (*walletEntity.Wallet, error) {
	return s.store.Wallet().FindOneForUpdate(ctx, id)
}

func (s checkoutTransaction) UpdateUserBalance(ctx context.Context, data *walletEntity.Wallet) error {
	return s.store.Wallet().UpdateBalanceFields(ctx, data)
}

func (s checkoutTransaction) InsertSystemLog(ctx context.Context, data *log.SystemLog) error {
	return s.store.Log().Insert(ctx, data)
}

func (s checkoutTransaction) UpdateOrder(ctx context.Context, data *order.Order) error {
	return s.store.Order().Update(ctx, data)
}

func (s checkoutTransaction) UpdateOrderStatusFrom(ctx context.Context, orderNo string, from, status uint8) (bool, error) {
	return s.store.Order().UpdateOrderStatusFrom(ctx, orderNo, from, status)
}

// NewPurchaseCheckoutLogic creates a new instance of PurchaseCheckoutLogic
// for handling purchase checkout operations across different payment platforms
func NewPurchaseCheckoutLogic(ctx context.Context, deps CheckoutDependencies) *PurchaseCheckoutLogic {
	return &PurchaseCheckoutLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// PurchaseCheckout processes the checkout for an order using the specified payment method
// It validates the order, retrieves payment configuration, and routes to the appropriate payment handler
func (l *PurchaseCheckoutLogic) PurchaseCheckout(req *dto.CheckoutOrderRequest) (resp *dto.CheckoutOrderResponse, err error) {

	// Validate and retrieve order information
	orderInfo, err := l.deps.Store.FindOrderByOrderNo(l.ctx, req.OrderNo)
	if err != nil {
		l.Logger.Error("[PurchaseCheckout] Find order failed", logger.Field("error", err.Error()), logger.Field("orderNo", req.OrderNo))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.OrderNotExist), "order not exist: %v", req.OrderNo)
	}

	// Verify order is in pending payment status (status = 1)
	if orderInfo.Status != 1 {
		l.Logger.Error("[PurchaseCheckout] Order status error", logger.Field("status", orderInfo.Status))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order status error: %v", orderInfo.Status)
	}
	if err := l.authorizeCheckout(orderInfo, req); err != nil {
		return nil, err
	}

	// Retrieve payment method configuration
	paymentConfig, err := l.deps.Store.FindPayment(l.ctx, orderInfo.PaymentId)
	if err != nil {
		l.Logger.Error("[PurchaseCheckout] Database query error", logger.Field("error", err.Error()), logger.Field("payment", orderInfo.Method))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find payment method error: %v", err.Error())
	}
	if err := ensurePaymentAvailable(paymentConfig); err != nil {
		return nil, err
	}
	// Route to appropriate payment handler based on payment platform
	switch payment2.ParsePlatform(orderInfo.Method) {
	case payment2.EPay:
		// Process EPay payment - generates payment URL for redirect
		url, err := l.epayPayment(paymentConfig, orderInfo, req.ReturnUrl)
		if err != nil {
			l.Logger.Error("[PurchaseCheckout] epay error", logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "epayPayment error: %v", err.Error())
		}
		resp = &dto.CheckoutOrderResponse{
			CheckoutUrl: url,
			Type:        "url", // Client should redirect to URL
		}

	case payment2.Stripe:
		// Process Stripe payment - creates payment sheet for client-side processing
		stripePayment, err := l.stripePayment(paymentConfig.Config, orderInfo, "")
		if err != nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "stripePayment error: %v", err.Error())
		}
		resp = &dto.CheckoutOrderResponse{
			Type:   "stripe", // Client should use Stripe SDK
			Stripe: stripePayment,
		}

	case payment2.AlipayF2F:
		// Process Alipay Face-to-Face payment - generates QR code
		url, err := l.alipayF2fPayment(paymentConfig, orderInfo)
		if err != nil {
			l.Errorw("[PurchaseCheckout] alipayF2fPayment error", logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "alipayF2fPayment error: %v", err.Error())
		}
		resp = &dto.CheckoutOrderResponse{
			Type:        "qr", // Client should display QR code
			CheckoutUrl: url,
		}

	case payment2.Cryptomus:
		// Process Cryptomus crypto payment - generates hosted invoice URL for redirect
		url, err := l.cryptomusPayment(paymentConfig, orderInfo, req.ReturnUrl)
		if err != nil {
			l.Errorw("[PurchaseCheckout] cryptomusPayment error", logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "cryptomusPayment error: %v", err.Error())
		}
		resp = &dto.CheckoutOrderResponse{
			CheckoutUrl: url,
			Type:        "url", // Client should redirect to URL
		}

	case payment2.Balance:
		// Process balance payment - validate user and process payment immediately
		if orderInfo.UserId == 0 {
			l.Errorw("[PurchaseCheckout] user not found", logger.Field("userId", orderInfo.UserId))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "user not found")
		}

		// Retrieve user information for balance validation
		userInfo, err := l.deps.Store.FindUser(l.ctx, orderInfo.UserId)
		if err != nil {
			l.Errorw("[PurchaseCheckout] FindOne User error", logger.Field("error", err.Error()))
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOne error: %s", err.Error())
		}

		// Process balance payment with gift amount priority logic
		if err = l.balancePayment(userInfo, orderInfo); err != nil {
			return nil, err
		}

		resp = &dto.CheckoutOrderResponse{
			Type: "balance", // Payment completed immediately
		}

	default:
		l.Errorw("[PurchaseCheckout] payment method not found", logger.Field("method", orderInfo.Method))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "payment method not found")
	}
	return
}

// authorizeCheckout keeps user-owned orders bound to their authenticated
// owner while guest orders use a short-lived, cryptographically-random
// checkout capability kept only in the temporary-order record.
func (l *PurchaseCheckoutLogic) authorizeCheckout(orderInfo *order.Order, req *dto.CheckoutOrderRequest) error {
	if orderInfo.UserId != 0 {
		currentUser, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
		if !ok || currentUser.Id != orderInfo.UserId {
			return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "order does not belong to the current user")
		}
		return nil
	}

	if req.CheckoutToken == "" {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "guest checkout token is required")
	}
	if orderInfo.GuestCheckoutTokenHash != "" {
		if subtle.ConstantTimeCompare([]byte(orderInfo.GuestCheckoutTokenHash), []byte(order.CheckoutTokenHash(req.CheckoutToken))) != 1 {
			return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "guest checkout token is invalid")
		}
		return nil
	}
	// Compatibility for guest orders created before checkout capabilities were
	// persisted on the order itself.
	cacheKey := fmt.Sprintf(order.TempOrderCacheKey, orderInfo.OrderNo)
	value, err := l.deps.GuestCheckoutCache.Get(l.ctx, cacheKey).Result()
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "guest checkout token is invalid")
	}
	var tempOrder order.TemporaryOrderInfo
	if err := tempOrder.Unmarshal([]byte(value)); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "guest checkout token is invalid")
	}
	if tempOrder.OrderNo != orderInfo.OrderNo || tempOrder.CheckoutToken == "" ||
		subtle.ConstantTimeCompare([]byte(tempOrder.CheckoutToken), []byte(req.CheckoutToken)) != 1 {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "guest checkout token is invalid")
	}
	return nil
}

// alipayF2fPayment processes Alipay Face-to-Face payment by generating a QR code
// It handles currency conversion and creates a pre-payment trade for QR code scanning
func (l *PurchaseCheckoutLogic) alipayF2fPayment(pay *payment.Payment, info *order.Order) (string, error) {
	// Parse Alipay F2F configuration from payment settings
	f2FConfig := &payment.AlipayF2FConfig{}
	if err := f2FConfig.Unmarshal([]byte(pay.Config)); err != nil {
		l.Errorw("[PurchaseCheckout] Unmarshal Alipay config error", logger.Field("error", err.Error()))
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Unmarshal error: %s", err.Error())
	}

	// Build notification URL for payment status callbacks
	notifyUrl := ""
	if pay.Domain != "" {
		notifyUrl = strings.TrimSuffix(pay.Domain, "/") + "/v1/notify/" + pay.Platform + "/" + pay.Token
	} else {
		host, ok := l.ctx.Value(requestctx.CtxKeyRequestHost).(string)
		if !ok {
			host = l.deps.Config.Host
		}
		notifyUrl = "https://" + strings.TrimSuffix(host, "/") + "/v1/notify/" + pay.Platform + "/" + pay.Token
	}

	// Initialize Alipay client with configuration
	client := alipay.NewClient(alipay.Config{
		AppId:       f2FConfig.AppId,
		PrivateKey:  f2FConfig.PrivateKey,
		PublicKey:   f2FConfig.PublicKey,
		InvoiceName: f2FConfig.InvoiceName,
		NotifyURL:   notifyUrl,
		Sandbox:     f2FConfig.Sandbox,
		Gateway:     f2FConfig.Gateway,
	})
	if client == nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "initialize Alipay client failed")
	}

	// Convert order amount to CNY using current exchange rate
	amount, err := l.queryExchangeRate("CNY", info.Amount)
	if err != nil {
		l.Errorw("[PurchaseCheckout] queryExchangeRate error", logger.Field("error", err.Error()))
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "queryExchangeRate error: %s", err.Error())
	}
	convertAmount, err := payment2.ParseAmount(epay.FormatMoney(amount))
	if err != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "invalid Alipay amount: %v", err)
	}
	if err := l.persistPaymentExpectation(info, convertAmount, "CNY"); err != nil {
		return "", err
	}

	// Create pre-payment trade and generate QR code
	QRCode, err := client.PreCreateTrade(l.ctx, alipay.Order{
		OrderNo: info.OrderNo,
		Amount:  convertAmount,
	})
	if err != nil {
		l.Errorw("[PurchaseCheckout] PreCreateTrade error", logger.Field("error", err.Error()))
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "PreCreateTrade error: %s", err.Error())
	}
	return QRCode, nil
}

// stripePayment processes Stripe payment by creating a payment sheet
// It supports various payment methods including WeChat Pay and Alipay through Stripe
func (l *PurchaseCheckoutLogic) stripePayment(config string, info *order.Order, identifier string) (*dto.StripePayment, error) {
	// Parse Stripe configuration from payment settings
	stripeConfig := &payment.StripeConfig{}

	if err := stripeConfig.Unmarshal([]byte(config)); err != nil {
		l.Errorw("[PurchaseCheckout] Unmarshal Stripe config error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Unmarshal error: %s", err.Error())
	}

	// Initialize Stripe client with API credentials
	client := stripe.NewClient(stripe.Config{
		SecretKey:     stripeConfig.SecretKey,
		PublicKey:     stripeConfig.PublicKey,
		WebhookSecret: stripeConfig.WebhookSecret,
	})
	if err := l.persistPaymentExpectation(info, info.Amount, strings.ToUpper(l.deps.Config.CurrencyUnit)); err != nil {
		return nil, err
	}

	stripeOrder := &stripe.Order{
		OrderNo:   info.OrderNo,
		Subscribe: strconv.FormatInt(info.SubscribeId, 10),
		Amount:    info.Amount,
		Currency:  strings.ToLower(l.deps.Config.CurrencyUnit),
		Payment:   stripeConfig.Payment,
	}
	// A pending order owns exactly one Stripe PaymentIntent.  Reusing it is
	// essential: accepting two client secrets would allow a user to pay an
	// older intent whose callback no longer matches the stored trade number.
	var (
		result *stripe.PaymentSheet
		err    error
	)
	if info.TradeNo != "" {
		result, err = client.GetPaymentSheet(stripeOrder, info.TradeNo)
	} else {
		result, err = client.CreatePaymentSheet(stripeOrder, &stripe.User{
			UserId: info.UserId,
			Email:  identifier,
		})
		if err == nil {
			claimed, claimErr := l.deps.Store.SetPaymentTradeNoIfEmpty(l.ctx, info.OrderNo, result.TradeNo)
			if claimErr != nil {
				_ = client.CancelPaymentIntent(result.TradeNo)
				return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "claim Stripe payment intent: %v", claimErr)
			}
			if !claimed {
				// Another concurrent checkout won the order's one intent. Cancel
				// ours and expose the winner's client secret instead.
				if cancelErr := client.CancelPaymentIntent(result.TradeNo); cancelErr != nil {
					return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "cancel duplicate Stripe payment intent: %v", cancelErr)
				}
				latest, latestErr := l.deps.Store.FindOrderByOrderNo(l.ctx, info.OrderNo)
				if latestErr != nil || latest.Status != 1 || latest.TradeNo == "" {
					if latestErr != nil {
						return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "reload Stripe payment intent: %v", latestErr)
					}
					return nil, errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order no longer has a pending Stripe payment intent")
				}
				result, err = client.GetPaymentSheet(stripeOrder, latest.TradeNo)
			} else {
				info.TradeNo = result.TradeNo
			}
		}
	}
	if err != nil {
		l.Errorw("[PurchaseCheckout] create or retrieve Stripe payment sheet error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Stripe payment sheet error: %s", err.Error())
	}

	// Prepare response data for client-side Stripe integration
	stripePayment := &dto.StripePayment{
		PublishableKey: stripeConfig.PublicKey,
		ClientSecret:   result.ClientSecret,
		Method:         stripeConfig.Payment,
	}

	return stripePayment, nil
}

// epayPayment processes EPay payment by generating a payment URL for redirect
// It handles currency conversion and creates a payment URL for external payment processing
func (l *PurchaseCheckoutLogic) epayPayment(config *payment.Payment, info *order.Order, returnUrl string) (string, error) {
	var err error
	// Parse EPay configuration from payment settings
	epayConfig := &payment.EPayConfig{}
	if err := epayConfig.Unmarshal([]byte(config.Config)); err != nil {
		l.Errorw("[PurchaseCheckout] Unmarshal EPay config error", logger.Field("error", err.Error()))
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Unmarshal error: %s", err.Error())
	}
	// Initialize EPay client with merchant credentials
	client := epay.NewClient(epayConfig.Pid, epayConfig.Url, epayConfig.Key, epayConfig.Type)
	var amount float64
	if l.deps.Config.CurrencyUnit != "CNY" {
		// Convert order amount to CNY using current exchange rate
		amount, err = l.queryExchangeRate("CNY", info.Amount)
		if err != nil {
			l.Logger.Error("[PurchaseCheckout] queryExchangeRate error", logger.Field("error", err.Error()))
			return "", err
		}
	} else {
		amount = float64(info.Amount) / float64(100)
	}
	expectedAmount, err := epay.ParseMoney(epay.FormatMoney(amount))
	if err != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "invalid EPay amount: %v", err)
	}
	if err := l.persistPaymentExpectation(info, expectedAmount, "CNY"); err != nil {
		return "", err
	}

	baseURL := l.paymentPublicBaseURL(config)
	notifyURL := baseURL + "/v1/notify/" + config.Platform + "/" + config.Token
	return client.CreatePayUrl(epay.Order{
		Name:      l.deps.Config.SiteName,
		Amount:    amount,
		OrderNo:   info.OrderNo,
		SignType:  "MD5",
		NotifyUrl: notifyURL,
		ReturnUrl: returnUrl,
	})
}

// cryptomusPayment processes a Cryptomus crypto payment by creating a hosted
// invoice in the system currency; the gateway converts it to the payer's
// chosen cryptocurrency at payment time.
func (l *PurchaseCheckoutLogic) cryptomusPayment(config *payment.Payment, info *order.Order, returnUrl string) (string, error) {
	cryptomusConfig := &payment.CryptomusConfig{}
	if err := cryptomusConfig.Unmarshal([]byte(config.Config)); err != nil {
		l.Errorw("[PurchaseCheckout] Unmarshal Cryptomus config error", logger.Field("error", err.Error()))
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Unmarshal error: %s", err.Error())
	}
	client := cryptomus.NewClient(cryptomus.Config{
		MerchantID: cryptomusConfig.MerchantID,
		APIKey:     cryptomusConfig.APIKey,
	})

	currency := strings.ToUpper(l.deps.Config.CurrencyUnit)
	if err := l.persistPaymentExpectation(info, info.Amount, currency); err != nil {
		return "", err
	}

	// A pending order owns exactly one Cryptomus invoice. The claimed trade
	// number keeps every checkout retry on the same hosted URL, so a callback
	// can never reference an invoice the order does not know about.
	if info.TradeNo != "" {
		invoice, err := client.GetInvoice(info.TradeNo, "")
		if err != nil {
			l.Errorw("[PurchaseCheckout] Cryptomus invoice lookup error", logger.Field("error", err.Error()))
			return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Cryptomus invoice lookup error: %s", err.Error())
		}
		return l.cryptomusInvoiceURL(invoice, info)
	}

	notifyURL := l.paymentPublicBaseURL(config) + "/v1/notify/" + config.Platform + "/" + config.Token
	invoice, err := client.CreateInvoice(cryptomus.Order{
		OrderNo:   info.OrderNo,
		Amount:    info.Amount,
		Currency:  currency,
		NotifyURL: notifyURL,
		ReturnURL: returnUrl,
		// The invoice must not outlive the order's close window: a payment made
		// after the order closed could no longer be fulfilled.
		Lifetime: checkout.CloseOrderTimeMinutes * 60,
	})
	if err != nil {
		// The gateway enforces one active invoice per order number. A previous
		// checkout may have created it and crashed before claiming the trade
		// number, so recover that invoice instead of failing the checkout.
		existing, queryErr := client.GetInvoice("", info.OrderNo)
		if queryErr != nil {
			l.Errorw("[PurchaseCheckout] Create Cryptomus invoice error", logger.Field("error", err.Error()))
			return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "create Cryptomus invoice: %s", err.Error())
		}
		invoice = existing
	}

	claimed, claimErr := l.deps.Store.SetPaymentTradeNoIfEmpty(l.ctx, info.OrderNo, invoice.UUID)
	if claimErr != nil {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "claim Cryptomus invoice: %v", claimErr)
	}
	if !claimed {
		// Another concurrent checkout claimed the order's one invoice first;
		// expose that invoice instead. The loser invoice is never referenced
		// again and simply expires at the gateway.
		latest, latestErr := l.deps.Store.FindOrderByOrderNo(l.ctx, info.OrderNo)
		if latestErr != nil {
			return "", errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "reload Cryptomus invoice: %v", latestErr)
		}
		if latest.Status != 1 || latest.TradeNo == "" {
			return "", errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order no longer has a pending Cryptomus invoice")
		}
		if latest.TradeNo != invoice.UUID {
			invoice, err = client.GetInvoice(latest.TradeNo, "")
			if err != nil {
				return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Cryptomus invoice lookup error: %s", err.Error())
			}
		}
	} else {
		info.TradeNo = invoice.UUID
	}
	return l.cryptomusInvoiceURL(invoice, info)
}

// cryptomusInvoiceURL guards the reused invoice against drift: the checkout
// URL handed to the payer must belong to this order and charge exactly the
// persisted payment expectation.
func (l *PurchaseCheckoutLogic) cryptomusInvoiceURL(invoice *cryptomus.Invoice, info *order.Order) (string, error) {
	if invoice.OrderNo != info.OrderNo {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Cryptomus invoice order mismatch")
	}
	amount, err := cryptomus.ParseMoney(invoice.Amount)
	if err != nil || amount != info.PaymentAmount || !strings.EqualFold(invoice.Currency, info.PaymentCurrency) {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Cryptomus invoice does not match payment expectation")
	}
	if invoice.URL == "" {
		return "", errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Cryptomus invoice has no checkout URL")
	}
	return invoice.URL, nil
}

func (l *PurchaseCheckoutLogic) paymentPublicBaseURL(config *payment.Payment) string {
	baseURL := strings.TrimSuffix(config.Domain, "/")
	if baseURL == "" {
		host, ok := l.ctx.Value(requestctx.CtxKeyRequestHost).(string)
		if !ok || host == "" {
			host = l.deps.Config.Host
		}
		baseURL = "https://" + strings.TrimSuffix(host, "/")
	}
	return strings.TrimSuffix(baseURL, "/")
}

// queryExchangeRate converts the order amount from system currency to target currency
// It retrieves the current exchange rate and performs currency conversion if needed
func (l *PurchaseCheckoutLogic) queryExchangeRate(to string, src int64) (amount float64, err error) {
	// Convert cents to decimal amount
	amount = float64(src) / float64(100)

	// No conversion needed if target currency matches system currency
	if to == l.deps.Config.CurrencyUnit {
		return amount, nil
	}

	if l.deps.ExchangeRateCache != nil && l.deps.ExchangeRateCache.Get() != 0 && to == "CNY" {
		amount = amount * l.deps.ExchangeRateCache.Get()
		return amount, nil
	}

	// A gateway must never be sent a value merely relabelled as another
	// currency. Without a configured conversion source, reject non-CNY
	// checkout instead of silently charging the system-currency amount.
	if l.deps.Config.CurrencyAccessKey == "" {
		return 0, errors.New("exchange rate is not configured")
	}

	// Convert currency if system currency differs from target currency
	result, err := exchangerate.GetExchangeRate(l.deps.Config.CurrencyUnit, to, l.deps.Config.CurrencyAccessKey, 1)
	if err != nil {
		l.Logger.Error("[PurchaseCheckout] QueryExchangeRate error", logger.Field("error", err.Error()))
		return 0, err
	}
	if l.deps.ExchangeRateCache != nil {
		l.deps.ExchangeRateCache.Set(result)
	}
	return result * amount, nil
}

func (l *PurchaseCheckoutLogic) persistPaymentExpectation(info *order.Order, amount int64, currency string) error {
	currency = strings.ToUpper(currency)
	if info.PaymentCurrency != "" {
		if info.PaymentAmount != amount || !strings.EqualFold(info.PaymentCurrency, currency) {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "payment expectation does not match existing checkout")
		}
		return nil
	}

	updated, err := l.deps.Store.UpdatePaymentExpectation(l.ctx, info.OrderNo, amount, currency)
	if err != nil {
		l.Errorw("[PurchaseCheckout] Save payment expectation failed",
			logger.Field("error", err.Error()),
			logger.Field("orderNo", info.OrderNo),
		)
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "save payment expectation: %v", err)
	}
	if updated {
		info.PaymentAmount = amount
		info.PaymentCurrency = currency
		return nil
	}

	// Another concurrent checkout may have recorded the immutable snapshot
	// first. Reload it so identical retries can continue (and, for Stripe,
	// reuse the payment intent it already claimed) while a different amount or
	// currency remains rejected.
	latest, err := l.deps.Store.FindOrderByOrderNo(l.ctx, info.OrderNo)
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "reload payment expectation: %v", err)
	}
	if latest.Status != 1 {
		return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
	}
	if latest.PaymentCurrency == "" {
		return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "payment checkout is being initialized; retry")
	}
	if latest.PaymentAmount != amount || !strings.EqualFold(latest.PaymentCurrency, currency) {
		return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "payment expectation does not match existing checkout")
	}
	info.PaymentAmount = latest.PaymentAmount
	info.PaymentCurrency = latest.PaymentCurrency
	info.TradeNo = latest.TradeNo
	return nil
}

// balancePayment processes balance payment with gift amount priority logic
// It prioritizes using gift amount first, then regular balance, and creates proper audit logs
func (l *PurchaseCheckoutLogic) balancePayment(u *user.User, o *order.Order) error {
	var err error
	var paidUser *walletEntity.Wallet
	if o.Amount == 0 {
		// No payment required for zero-amount orders
		l.Logger.Info(
			"[PurchaseCheckout] No payment required for zero-amount order",
			logger.Field("orderNo", o.OrderNo),
			logger.Field("userId", u.Id),
		)
		updated, err := l.deps.Store.UpdateOrderStatusFrom(l.ctx, o.OrderNo, 1, 2)
		if err != nil {
			l.Errorw("[PurchaseCheckout] Update order status error",
				logger.Field("error", err.Error()),
				logger.Field("orderNo", o.OrderNo),
				logger.Field("userId", u.Id))
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Update order status error: %s", err.Error())
		}
		if !updated {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}
		goto activation
	}

	err = l.deps.Store.InTx(l.ctx, func(store CheckoutTransaction) error {
		// Lock the order first so concurrent checkout requests for the same
		// order cannot both reach the balance debit path.
		orderInfo, err := store.FindOrderByOrderNoForUpdate(l.ctx, o.OrderNo)
		if err != nil {
			return err
		}
		if orderInfo.Status != 1 {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}

		// Retrieve the latest user information inside the transaction without
		// Redis cache and lock the row before checking or changing balances.
		userInfo, err := store.FindUserForUpdate(l.ctx, u.Id)
		if err != nil {
			return err
		}

		// Check if user has sufficient total balance (regular + gift)
		totalAvailable := userInfo.Balance + userInfo.GiftAmount
		if totalAvailable < o.Amount {
			return errors.Wrapf(xerr.NewErrCode(xerr.InsufficientBalance),
				"Insufficient balance: required %d, available %d", o.Amount, totalAvailable)
		}

		// Calculate payment distribution: prioritize gift amount first
		var giftUsed, balanceUsed int64
		remainingAmount := o.Amount

		if userInfo.GiftAmount >= remainingAmount {
			// Gift amount covers the entire payment
			giftUsed = remainingAmount
			balanceUsed = 0
		} else {
			// Use all available gift amount, then regular balance
			giftUsed = userInfo.GiftAmount
			balanceUsed = remainingAmount - giftUsed
		}

		// Update user balances
		userInfo.GiftAmount -= giftUsed
		userInfo.Balance -= balanceUsed

		// Save only the balance fields; do not write back a cached/stale user row.
		if err = store.UpdateUserBalance(l.ctx, userInfo); err != nil {
			return err
		}

		// Create gift amount log if gift amount was used
		if giftUsed > 0 {
			giftLog := &log.Gift{
				OrderNo: o.OrderNo,
				Type:    log.GiftTypeReduce, // Type 2 represents gift amount decrease/usage
				Amount:  giftUsed,
				Balance: userInfo.GiftAmount,
				Remark:  "Purchase payment",
			}
			content, _ := giftLog.Marshal()

			err = store.InsertSystemLog(l.ctx, &log.SystemLog{
				Type:     log.TypeGift.Uint8(),
				ObjectID: userInfo.UserId,
				Date:     timeutil.Now().Format(time.DateOnly),
				Content:  string(content),
			})
			if err != nil {
				return err
			}
		}

		// Create balance log if regular balance was used
		if balanceUsed > 0 {
			balanceLog := &log.Balance{
				Amount:    balanceUsed,
				Type:      log.BalanceTypePayment, // Type 3 represents payment deduction
				OrderNo:   o.OrderNo,
				Balance:   userInfo.Balance,
				Timestamp: timeutil.Now().UnixMilli(),
			}
			content, _ := balanceLog.Marshal()
			err = store.InsertSystemLog(l.ctx, &log.SystemLog{
				Type:     log.TypeBalance.Uint8(),
				ObjectID: userInfo.UserId,
				Date:     timeutil.Now().Format(time.DateOnly),
				Content:  string(content),
			})
			if err != nil {
				return err
			}
		}

		// Store gift amount used in order for potential refund tracking.
		// Keep any gift amount that was already recorded at order creation.
		if giftUsed > 0 {
			orderInfo.GiftAmount += giftUsed
			if err = store.UpdateOrder(l.ctx, orderInfo); err != nil {
				return err
			}
		}

		// Mark order as paid (status = 2)
		updated, err := store.UpdateOrderStatusFrom(l.ctx, o.OrderNo, 1, 2)
		if err != nil {
			return err
		}
		if !updated {
			return errors.Wrapf(xerr.NewErrCode(xerr.OrderStatusError), "order is no longer pending")
		}
		paidUser = userInfo
		return nil
	})

	if err != nil {
		l.Errorw("[PurchaseCheckout] Balance payment transaction error",
			logger.Field("error", err.Error()),
			logger.Field("orderNo", o.OrderNo),
			logger.Field("userId", u.Id))
		return err
	}
	if paidUser != nil {
		if cacheErr := l.deps.Store.ClearUserCache(l.ctx, &user.User{Id: paidUser.UserId}); cacheErr != nil {
			l.Errorw("[PurchaseCheckout] Clear user cache error",
				logger.Field("error", cacheErr.Error()),
				logger.Field("userId", paidUser.UserId))
		}
	}

activation:
	// Enqueue order activation task for immediate processing
	payload := taskqueue.ForthwithActivateOrderPayload{
		OrderNo: o.OrderNo,
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		l.Errorw("[PurchaseCheckout] Marshal activation payload error", logger.Field("error", err.Error()))
		return err
	}

	task := asynq.NewTask(taskqueue.ForthwithActivateOrder, bytes)
	_, err = l.deps.ActivationQueue.EnqueueContext(l.ctx, task, asynq.MaxRetry(5), asynq.TaskID(taskqueue.ActivationTaskID(o.OrderNo)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		err = nil
	}
	if err != nil {
		l.Errorw("[PurchaseCheckout] Enqueue activation task error", logger.Field("error", err.Error()))
		return err
	}

	l.Logger.Info("[PurchaseCheckout] Balance payment completed successfully",
		logger.Field("orderNo", o.OrderNo),
		logger.Field("userId", u.Id))
	return nil
}
