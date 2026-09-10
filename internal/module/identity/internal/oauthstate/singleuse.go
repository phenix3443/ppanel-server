package oauthstate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/redis/go-redis/v9"
)

// claimScript records the first use of a callback payload and reports whether
// the caller may proceed. A repeat inside the grace window still succeeds so
// that a client retrying a timed-out exchange is not punished for it; later
// repeats are replays and are refused. The Lua form keeps the read and the
// write atomic across concurrent callbacks.
var claimScript = redis.NewScript(`
local first = redis.call("GET", KEYS[1])
if first then
  if tonumber(ARGV[1]) - tonumber(first) <= tonumber(ARGV[2]) then
    return 1
  end
  return 0
end
redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[3])
return 1
`)

// PayloadFingerprint derives the key material for a single-use claim. The
// signed callback itself is never stored, only its digest.
func PayloadFingerprint(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// ClaimSingleUse reports whether a signed callback payload may be redeemed.
// It returns false once the payload has already been redeemed outside the
// retry grace window. The clock is supplied by the caller so the window is
// evaluated against the same time source as the payload's own freshness
// check.
func ClaimSingleUse(ctx context.Context, client *redis.Client, key string, now time.Time, grace, ttl time.Duration) (bool, error) {
	allowed, err := claimScript.Run(ctx, client, []string{key},
		now.Unix(), int64(grace.Seconds()), int64(ttl.Seconds())).Int64()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}
