// Package protocolkey preserves the key and short-identifier encodings used
// by external protocols. These compatibility transforms are not token issuers.
package protocolkey

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// Md5Encode is retained for providers whose signing protocol requires MD5.
func Md5Encode(value string, upper bool) string {
	sum := md5.Sum([]byte(value))
	encoded := hex.EncodeToString(sum[:])
	if upper {
		return strings.ToUpper(encoded)
	}
	return encoded
}
