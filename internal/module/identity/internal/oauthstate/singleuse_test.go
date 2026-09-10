package oauthstate

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// A signed callback payload may be redeemed once. A client re-submitting the
// same payload right after a timed-out exchange must still succeed, while a
// later repeat is a replay and is refused.
func TestClaimSingleUseAllowsRetriesButRefusesLaterReplays(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()
	key := "telegram:" + PayloadFingerprint("tgAuthResult-payload")
	start := time.Unix(1785000000, 0)

	allowed, err := ClaimSingleUse(ctx, client, key, start, time.Minute, 5*time.Minute)
	if err != nil || !allowed {
		t.Fatalf("first claim allowed=%v err=%v, want true/nil", allowed, err)
	}

	allowed, err = ClaimSingleUse(ctx, client, key, start.Add(20*time.Second), time.Minute, 5*time.Minute)
	if err != nil || !allowed {
		t.Fatalf("retry within grace allowed=%v err=%v, want true/nil", allowed, err)
	}

	// Past the grace window the same payload is a replay.
	allowed, err = ClaimSingleUse(ctx, client, key, start.Add(90*time.Second), time.Minute, 5*time.Minute)
	if err != nil {
		t.Fatalf("replay claim err=%v", err)
	}
	if allowed {
		t.Fatal("replay outside the grace window was allowed")
	}

	// Once the record expires the fingerprint is reusable again, which is
	// harmless: the payload's own freshness check has lapsed by then.
	server.FastForward(6 * time.Minute)
	allowed, err = ClaimSingleUse(ctx, client, key, start.Add(6*time.Minute), time.Minute, 5*time.Minute)
	if err != nil || !allowed {
		t.Fatalf("claim after expiry allowed=%v err=%v, want true/nil", allowed, err)
	}
}

func TestClaimSingleUseIsolatesDistinctPayloads(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	ctx := context.Background()

	first := "telegram:" + PayloadFingerprint("payload-one")
	second := "telegram:" + PayloadFingerprint("payload-two")
	if first == second {
		t.Fatal("distinct payloads produced the same fingerprint")
	}
	for _, key := range []string{first, second} {
		allowed, err := ClaimSingleUse(ctx, client, key, time.Unix(1785000000, 0), time.Minute, 5*time.Minute)
		if err != nil || !allowed {
			t.Fatalf("claim %s allowed=%v err=%v, want true/nil", key, allowed, err)
		}
	}
}

// The stored fingerprint must not reveal the callback payload itself.
func TestPayloadFingerprintDoesNotEmbedThePayload(t *testing.T) {
	payload := "tgAuthResult-secret-bearer"
	if got := PayloadFingerprint(payload); len(got) != 64 || got == payload {
		t.Fatalf("fingerprint = %q, want a 64-char digest", got)
	}
}
