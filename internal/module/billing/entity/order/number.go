package order

import (
	cryptorand "crypto/rand"
	"fmt"
	"math/big"

	"github.com/perfect-panel/server/pkg/timeutil"
)

// tradeNoRandomDigits is the width of the random part of a trade number. With
// the second-resolution timestamp in front, two numbers collide only when
// they are made in the same second and draw the same 14 digits: for 10,000
// numbers in one second that is a 5·10⁻⁷ chance (8 digits made it 39%). The
// database's unique trade-no index would turn a collision into a failed
// purchase.
// 【18 位，别改回 14】fork 的 TestGenerateTradeNoUnique 在 14 位下稳定碰撞
// （同一秒上万次，生日问题下概率可观）。crypto/rand 保证均匀，位宽到 18
// 把空间提到 10^18；列是 varchar(255)，宽度无存储约束。
const tradeNoRandomDigits = 18

var tradeNoRandomBound = new(big.Int).Exp(big.NewInt(10), big.NewInt(tradeNoRandomDigits), nil)

// GenerateTradeNo returns a fixed-width numeric trade number: a 14-digit
// timestamp in the application's time zone followed by 14 uniformly random
// digits from crypto/rand. crypto/rand cannot fail in practice; the panic
// matches how the password salt handles the same error.
func GenerateTradeNo() string {
	n, err := cryptorand.Int(cryptorand.Reader, tradeNoRandomBound)
	if err != nil {
		panic(fmt.Errorf("generate trade number entropy: %w", err))
	}
	return timeutil.Now().Format("20060102150405") + fmt.Sprintf("%0*d", tradeNoRandomDigits, n)
}
