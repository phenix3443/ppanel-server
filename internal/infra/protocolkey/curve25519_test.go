package protocolkey

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestCurve25519PreservesRFC7748PublicAndLegacyPrivateBytes(t *testing.T) {
	private, _ := hex.DecodeString("77076d0a7318a57d3c16c17251b26645df4c2f87ebc0992ab177fba51db92c2a")
	public, _ := hex.DecodeString("8520f0098930a754748b7ddcb43ef75a0dbf3a0d26381af4eba4a98eaa9b4e6a")
	for _, std := range []bool{false, true} {
		encoding := base64.RawURLEncoding
		if std {
			encoding = base64.StdEncoding
		}
		gotPublic, gotPrivate, err := Curve25519Genkey(std, encoding.EncodeToString(private))
		if err != nil {
			t.Fatal(err)
		}
		if gotPublic != encoding.EncodeToString(public) {
			t.Fatalf("public key changed: %s", gotPublic)
		}
		wantPrivate := bytes.Clone(private)
		wantPrivate[0] &= 248
		wantPrivate[31] &= 127
		if gotPrivate != encoding.EncodeToString(wantPrivate) {
			t.Fatal("legacy private key normalization changed")
		}
	}
}

func TestCurve25519GenerateAndRejectMalformedKeys(t *testing.T) {
	public, private, err := Curve25519Genkey(false, "")
	if err != nil || len(public) != 43 || len(private) != 43 {
		t.Fatalf("generated keys: %v", err)
	}
	repeatedPublic, repeatedPrivate, err := Curve25519Genkey(false, private)
	if err != nil || repeatedPublic != public || repeatedPrivate != private {
		t.Fatal("generated private key does not round trip")
	}
	for _, key := range []string{"%%%", "AA", base64.RawURLEncoding.EncodeToString(make([]byte, 33))} {
		if _, _, err := Curve25519Genkey(false, key); err == nil {
			t.Fatal("invalid private key accepted")
		}
	}
}
