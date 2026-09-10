package order

import (
	cryptorand "crypto/rand"
	"fmt"
	"time"
)

// GenerateTradeNo returns a fixed-width numeric trade number: a 14-digit
// timestamp plus 8 digits from crypto/rand. Seeding from the clock alone
// produced identical numbers for concurrent requests within the same
// nanosecond; the database's unique trade-no index turns those collisions
// into visible purchase failures. crypto/rand cannot fail in practice; the
// panic matches how the password salt handles the same error.
func GenerateTradeNo() string {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		panic(fmt.Errorf("generate trade number entropy: %w", err))
	}
	digits := make([]byte, len(buf))
	for i, v := range buf {
		digits[i] = '0' + v%10
	}
	return time.Now().Format("20060102150405") + string(digits)
}
