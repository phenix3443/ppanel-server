package protocolkey

import "testing"

func TestMD5ProviderCompatibility(t *testing.T) {
	for _, tc := range []struct {
		value string
		upper bool
		want  string
	}{
		{"hello", false, "5d41402abc4b2a76b9719d911017c592"},
		{"hello", true, "5D41402ABC4B2A76B9719D911017C592"},
		{"", false, "d41d8cd98f00b204e9800998ecf8427e"},
	} {
		if got := Md5Encode(tc.value, tc.upper); got != tc.want {
			t.Fatalf("provider digest=%q, want %q", got, tc.want)
		}
	}
}
