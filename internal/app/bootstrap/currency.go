package bootstrap

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/pkg/logger"
)

func Currency(ctx *Dependencies) {
	// Retrieve system currency configuration
	currency, err := ctx.Store.System().GetCurrencyConfig(context.Background())
	if err != nil {
		logger.Errorf("[INIT] Failed to get currency configuration: %v", err.Error())
		panic(fmt.Sprintf("[INIT] Failed to get currency configuration: %v", err.Error()))
	}
	// Parse currency configuration
	configs := struct {
		CurrencyUnit   string
		CurrencySymbol string
		AccessKey      string
	}{}
	config.SystemConfigSliceReflectToStruct(currency, &configs)
	ctx.ExchangeRate.Set(0) // Default exchange rate to 0
	currencyConfig := config.Currency{
		Unit:      configs.CurrencyUnit,
		Symbol:    configs.CurrencySymbol,
		AccessKey: configs.AccessKey,
	}
	ctx.updateConfig(func(current *config.Config) { current.Currency = currencyConfig })
	logger.Info("[INIT] Currency configuration loaded",
		logger.Field("unit", currencyConfig.Unit),
		logger.Field("symbol", currencyConfig.Symbol),
		logger.Field("provider_configured", currencyConfig.AccessKey != ""),
	)
}
