package exchangerate

import "testing"

func TestCache(t *testing.T) {
	cache := NewCache(6.75)
	if got := cache.Get(); got != 6.75 {
		t.Fatalf("Get() = %v, want 6.75", got)
	}

	cache.Set(7.1)
	if got := cache.Get(); got != 7.1 {
		t.Fatalf("Get() after Set = %v, want 7.1", got)
	}
}
