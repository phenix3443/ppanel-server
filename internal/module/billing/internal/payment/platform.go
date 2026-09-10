package payment

import (
	"github.com/perfect-panel/server/internal/infra/integration"
)

type Platform int

const (
	Stripe Platform = iota
	AlipayF2F
	EPay
	Balance
	Cryptomus
	UNSUPPORTED Platform = -1
)

var platformNames = map[string]Platform{
	"Stripe":      Stripe,
	"AlipayF2F":   AlipayF2F,
	"EPay":        EPay,
	"balance":     Balance,
	"Cryptomus":   Cryptomus,
	"unsupported": UNSUPPORTED,
}

func (p Platform) String() string {
	for k, v := range platformNames {
		if v == p {
			return k
		}
	}
	return "unsupported"
}

func ParsePlatform(s string) Platform {
	if p, ok := platformNames[s]; ok {
		return p
	}
	return UNSUPPORTED
}

// SupportedPlatformNames lists every platform that may be exposed for a new
// checkout. Keep this separate from GetSupportedPlatforms because balance is
// an internal checkout method rather than an administrator-configurable
// gateway.
func SupportedPlatformNames() []string {
	return []string{Stripe.String(), AlipayF2F.String(), EPay.String(), Balance.String(), Cryptomus.String()}
}

func GetSupportedPlatforms() []integration.Info {
	return []integration.Info{
		{
			Platform:    Stripe.String(),
			PlatformURL: "https://stripe.com",
			PlatformFieldDescription: map[string]string{
				"public_key":     "Publishable key",
				"secret_key":     "Secret key",
				"webhook_secret": "Webhook secret",
				"payment":        "Payment Method, only supported card/alipay/wechat_pay",
			},
		},
		{
			Platform:    AlipayF2F.String(),
			PlatformURL: "https://alipay.com",
			PlatformFieldDescription: map[string]string{
				"app_id":       "App ID",
				"private_key":  "Private Key",
				"public_key":   "Public Key",
				"invoice_name": "Invoice Name",
				"sandbox":      "Sandbox Mode",
			},
		},
		{
			Platform:    EPay.String(),
			PlatformURL: "",
			PlatformFieldDescription: map[string]string{
				"pid":  "PID",
				"url":  "URL",
				"key":  "Key",
				"type": "Type",
			},
		},
		{
			Platform:    Cryptomus.String(),
			PlatformURL: "https://cryptomus.com",
			PlatformFieldDescription: map[string]string{
				"merchant_id": "Merchant UUID",
				"api_key":     "Payment API Key",
			},
		},
	}
}
