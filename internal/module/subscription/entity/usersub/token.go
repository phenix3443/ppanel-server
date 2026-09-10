package usersub

import (
	"crypto/sha256"
	"encoding/hex"
)

// TokenFromOrder preserves the subscription token derivation used by existing
// orders. Changing this policy is separate from replacing UUID libraries.
func TokenFromOrder(orderNo string) string {
	hash := sha256.Sum256([]byte(orderNo))
	return hex.EncodeToString(hash[:16])
}
