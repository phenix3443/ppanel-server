package usersub

import "testing"

func TestTokenFromOrderPreservesStoredFormat(t *testing.T) {
	for _, tc := range []struct{ order, token string }{
		{"20241213222445955", "46382230918a861482e4f1c61aa0e930"},
		{"", "e3b0c44298fc1c149afbf4c8996fb924"},
	} {
		if got := TokenFromOrder(tc.order); got != tc.token {
			t.Fatalf("TokenFromOrder(%q)=%q, want %q", tc.order, got, tc.token)
		}
	}
}
