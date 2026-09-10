package maintenance

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/pkg/logger"
)

type RateLogic struct {
	deps RateDependencies
}

func NewRateLogic(deps RateDependencies) *RateLogic {
	return &RateLogic{deps: deps}
}

func (l *RateLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	// Retrieve system currency configuration
	currency, err := l.deps.Store.System().GetCurrencyConfig(ctx)
	if err != nil {
		logger.Errorw("[PurchaseCheckout] GetCurrencyConfig error", logger.Field("error", err.Error()))
		return err
	}
	// Parse currency configuration
	configs := struct {
		CurrencyUnit   string
		CurrencySymbol string
		AccessKey      string
	}{}
	config.SystemConfigSliceReflectToStruct(currency, &configs)

	// Skip conversion if no exchange rate API key configured
	if configs.AccessKey == "" {
		logger.Debugf("[RateLogic] skip exchange rate, no access key configured")
		return nil
	}
	// Update exchange rates
	result, err := billing.ConvertCurrency(configs.CurrencyUnit, "CNY", configs.AccessKey, 1)
	if err != nil {
		logger.Errorw("[RateLogic] GetExchangeRete error", logger.Field("error", err.Error()))
		return err
	}
	l.deps.ExchangeRate.Set(result)
	logger.WithContext(ctx).Infof("[RateLogic] GetExchangeRete success, result: %+v", result)
	return nil
}
