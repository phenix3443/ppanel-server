package render

import "testing"

func TestClientFingerprint(t *testing.T) {
	cases := []struct {
		name        string
		security    string
		fingerprint string
		want        string
	}{
		{"reality 未配指纹时补默认值", "reality", "", "chrome"},
		{"reality 已配指纹时保留", "reality", "firefox", "firefox"},
		{"tls 未配指纹时不补", "tls", "", ""},
		{"无加密不补", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clientFingerprint(c.security, c.fingerprint); got != c.want {
				t.Fatalf("clientFingerprint(%q, %q) = %q, want %q", c.security, c.fingerprint, got, c.want)
			}
		})
	}
}
