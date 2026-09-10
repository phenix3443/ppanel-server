package logger

import (
	"fmt"
	"time"
)

// Keep a large initial offset so a zero lastTime permits the first log.
// Add (unlike AddDate) retains the monotonic clock used for rate limiting.
var logClockStart = time.Now().Add(-400 * 24 * time.Hour)

func relativeLogTime() time.Duration { return time.Since(logClockStart) }

// Preserve the public log field's existing formatting and precision.
func reprLogDuration(duration time.Duration) string {
	return fmt.Sprintf("%.1fms", float32(duration)/float32(time.Millisecond))
}
