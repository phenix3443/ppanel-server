package usersub

import "testing"

func TestSubscribeCacheKeyUsesV3UserListKey(t *testing.T) {
	keys := (&Subscribe{Id: 8, UserId: 7, Token: "token"}).GetCacheKeys()
	want := "cache:user:subscribe:user:v3:7"
	for _, key := range keys {
		if key == want {
			return
		}
	}
	t.Fatalf("subscription cache keys = %#v, want %q", keys, want)
}
