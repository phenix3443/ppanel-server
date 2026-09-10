package alipay

import (
	"context"
	"net/url"

	"github.com/perfect-panel/server/internal/module/billing/internal/payment"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
	"github.com/smartwalle/alipay/v3"
)

type Config struct {
	AppId       string
	PrivateKey  string
	PublicKey   string
	InvoiceName string
	NotifyURL   string
	Sandbox     bool
	// Gateway overrides the sandbox gateway URL; Alipay has retired sandbox
	// hosts before, and tests point it at a local fake gateway. Ignored in
	// production, where only the official gateway may receive credentials.
	Gateway string
}

type Notification struct {
	OrderNo string
	Amount  int64
	Status  Status
	TradeNo string
	AppId   string
}

type Status string

const (
	Success  Status = "TRADE_SUCCESS"
	Pending  Status = "WAIT_BUYER_PAY"
	Closed   Status = "TRADE_CLOSED"
	Finished Status = "TRADE_FINISHED"
	Error    Status = "TRADE_ERROR"
)

// Paid reports whether the status proves the buyer's money was collected;
// TRADE_FINISHED is the terminal paid state after the refund window closes.
func (s Status) Paid() bool {
	return s == Success || s == Finished
}

// ErrTradeNotExist reports that the gateway holds no trade for the order.
// A face-to-face trade is created only when the buyer scans the QR code, so
// a missing trade proves no money was collected for the order.
var ErrTradeNotExist = errors.New("alipay trade does not exist")

const tradeNotExistSubCode = "ACQ.TRADE_NOT_EXIST"

// Trade is the gateway's authoritative view of an order as returned by the
// alipay.trade.query endpoint. Amount is populated only for paid trades.
type Trade struct {
	OrderNo string
	TradeNo string
	Amount  int64
	Status  Status
}

type Client struct {
	Config
	client *alipay.Client
}
type Order struct {
	OrderNo string
	Amount  int64
}

func NewClient(c Config) *Client {
	var opts []alipay.OptionFunc
	if c.Gateway != "" {
		opts = append(opts, alipay.WithSandboxGateway(c.Gateway))
	}
	client, err := alipay.New(c.AppId, c.PrivateKey, !c.Sandbox, opts...)
	if err != nil {
		logger.Error("[Alipay] NewClient failed: ", logger.Field("errors", err), logger.Field("appId", c.AppId), logger.Field("sandbox", c.Sandbox))
		return nil
	}
	err = client.LoadAliPayPublicKey(c.PublicKey)
	if err != nil {
		logger.Error("[Alipay] Load public key failed: ", logger.Field("errors", err), logger.Field("appId", c.AppId), logger.Field("sandbox", c.Sandbox))
		return nil
	}
	return &Client{
		Config: c,
		client: client,
	}
}

func (c *Client) PreCreateTrade(ctx context.Context, order Order) (string, error) {
	amountString := payment.FormatFloat(float64(order.Amount)/float64(100), 2)
	trade, err := c.client.TradePreCreate(ctx, alipay.TradePreCreate{
		Trade: alipay.Trade{
			OutTradeNo:  order.OrderNo,
			TotalAmount: amountString,
			Subject:     c.InvoiceName,
			NotifyURL:   c.NotifyURL,
			// Keep Alipay's payment window aligned with the local deferred
			// close task.  Otherwise a QR code could still be paid after the
			// order was closed and any reserved balance/inventory was restored.
			TimeoutExpress: "15m",
		},
	})
	if err != nil {
		return "", err
	}
	if trade.Code != alipay.CodeSuccess {
		return "", errors.New("PreCreateTrade failed: " + trade.Msg)
	}
	return trade.QRCode, nil
}

func (c *Client) QueryTrade(ctx context.Context, orderNo string) (*Trade, error) {
	rsp, err := c.client.TradeQuery(ctx, alipay.TradeQuery{
		OutTradeNo: orderNo,
	})
	if err != nil {
		return nil, asTradeNotExist(err)
	}
	if rsp.Code != alipay.CodeSuccess {
		if rsp.SubCode == tradeNotExistSubCode {
			return nil, ErrTradeNotExist
		}
		return nil, errors.New("QueryTrade failed: " + rsp.Msg + " " + rsp.SubMsg)
	}
	trade := &Trade{
		OrderNo: rsp.OutTradeNo,
		TradeNo: rsp.TradeNo,
	}
	switch rsp.TradeStatus {
	case alipay.TradeStatusSuccess:
		trade.Status = Success
	case alipay.TradeStatusWaitBuyerPay:
		trade.Status = Pending
	case alipay.TradeStatusClosed:
		trade.Status = Closed
	case alipay.TradeStatusFinished:
		trade.Status = Finished
	default:
		return nil, errors.New("QueryTrade failed: unexpected trade status " + string(rsp.TradeStatus))
	}
	if trade.Status.Paid() {
		amount, err := payment.ParseAmount(rsp.TotalAmount)
		if err != nil {
			return nil, errors.Wrap(err, "invalid trade amount")
		}
		trade.Amount = amount
	}
	return trade, nil
}

// CloseTrade voids an unpaid trade at the gateway so its QR code can no
// longer collect money. Closing a trade the gateway never created reports
// ErrTradeNotExist, and the gateway rejects closing a trade that was already
// paid, so a success here proves no payment can arrive afterwards.
func (c *Client) CloseTrade(ctx context.Context, orderNo string) error {
	rsp, err := c.client.TradeClose(ctx, alipay.TradeClose{
		OutTradeNo: orderNo,
	})
	if err != nil {
		return asTradeNotExist(err)
	}
	if rsp.Code != alipay.CodeSuccess {
		if rsp.SubCode == tradeNotExistSubCode {
			return ErrTradeNotExist
		}
		return errors.New("CloseTrade failed: " + rsp.Msg + " " + rsp.SubMsg)
	}
	return nil
}

// asTradeNotExist maps the SDK's business failure for a missing trade — which
// the SDK surfaces as an error when the gateway response carries no signature
// — onto ErrTradeNotExist and passes every other error through unchanged.
func asTradeNotExist(err error) error {
	var gatewayErr *alipay.Error
	if errors.As(err, &gatewayErr) && gatewayErr.SubCode == tradeNotExistSubCode {
		return ErrTradeNotExist
	}
	return err
}

func (c *Client) DecodeNotification(form url.Values) (*Notification, error) {
	notify, err := c.client.DecodeNotification(form)
	if err != nil {
		return nil, err
	}
	amount, err := payment.ParseAmount(notify.TotalAmount)
	if err != nil {
		return nil, errors.Wrap(err, "invalid notification amount")
	}

	return &Notification{
		OrderNo: notify.OutTradeNo,
		Amount:  amount,
		Status:  Status(notify.TradeStatus),
		TradeNo: notify.TradeNo,
		AppId:   notify.AppId,
	}, nil
}
