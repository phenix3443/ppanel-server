package maintenance

import (
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/repository"
)

type RateDependencies struct {
	Store        repository.Store
	ExchangeRate *billing.CurrencyRateCache
}
