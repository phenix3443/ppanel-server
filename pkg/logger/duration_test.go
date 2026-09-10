package logger

import (
	"testing"
	"time"
)

func TestLogDurationFormattingCompatibility(t *testing.T) {
	for _, tc := range []struct {
		duration time.Duration
		text     string
	}{
		{0, "0.0ms"}, {time.Millisecond, "1.0ms"}, {1500 * time.Microsecond, "1.5ms"}, {-time.Second, "-1000.0ms"},
	} {
		if got := reprLogDuration(tc.duration); got != tc.text {
			t.Fatalf("duration = %q, want %q", got, tc.text)
		}
	}
	called := false
	newLimitedExecutor(1000).logOrDiscard(func() { called = true })
	if !called {
		t.Fatal("first log was discarded")
	}
}
