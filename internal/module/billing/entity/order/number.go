package order

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	"time"
)

// tradeNoRandomDigits is the width of the random suffix. The timestamp only
// resolves to the second, so numbers minted within the same second are
// separated by this suffix alone — it has to be wide enough that a burst of
// orders in one second does not collide.
//
// 【8 位不够，别改回去】按生日问题，10^8 的空间下同一秒内一万次生成的
// 碰撞概率是 39%。这不是偶发：TestGenerateTradeNoUnique 稳定挂在
// 2000~9000 次之间。18 位把空间提到 10^18，同样一万次的碰撞概率是 5e-9。
// 列是 varchar(255)，宽度没有存储约束。
const tradeNoRandomDigits = 18

// GenerateTradeNo returns a fixed-width numeric trade number: a 14-digit
// timestamp plus an 18-digit random suffix. Seeding from the clock alone
// produced identical numbers for concurrent requests within the same
// nanosecond; the database's unique trade-no index turns those collisions
// into visible purchase failures.
//
// 【整个 uint64 取模，不要逐字节取模】逐字节取 %10 会把 64 bit 的熵压到
// 每字节 1 位十进制，位宽再大也没用。
func GenerateTradeNo() string {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		panic(fmt.Errorf("generate trade number entropy: %w", err))
	}
	n := binary.BigEndian.Uint64(buf[:]) % pow10(tradeNoRandomDigits)
	return time.Now().Format("20060102150405") +
		fmt.Sprintf("%0*d", tradeNoRandomDigits, n)
}

func pow10(n int) uint64 {
	v := uint64(1)
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}
