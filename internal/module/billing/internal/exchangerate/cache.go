package exchangerate

import (
	"math"
	"sync/atomic"
)

// Cache stores the most recently fetched exchange rate safely for concurrent
// readers. It is shared by the exchange-rate refresh task and checkout flows.
type Cache struct {
	value atomic.Uint64
}

// NewCache creates a cache with the supplied initial rate.
func NewCache(rate float64) *Cache {
	cache := &Cache{}
	cache.Set(rate)
	return cache
}

// Get returns the cached exchange rate.
func (c *Cache) Get() float64 {
	if c == nil {
		return 0
	}
	return math.Float64frombits(c.value.Load())
}

// Set replaces the cached exchange rate.
func (c *Cache) Set(rate float64) {
	if c == nil {
		return
	}
	c.value.Store(math.Float64bits(rate))
}
