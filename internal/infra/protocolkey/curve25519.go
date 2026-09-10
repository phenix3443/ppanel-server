package protocolkey

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"

	"github.com/pkg/errors"
)

func Curve25519Genkey(stdEncoding bool, inputBase64 string) (public, private string, err error) {
	encoding := base64.RawURLEncoding
	if stdEncoding {
		encoding = base64.StdEncoding
	}
	var privateKey []byte
	if inputBase64 != "" {
		privateKey, err = encoding.DecodeString(inputBase64)
		if err != nil {
			return "", "", err
		}
		if len(privateKey) != 32 {
			return "", "", errors.New("Invalid length of private key.")
		}
	} else {
		privateKey = make([]byte, 32)
		if _, err := rand.Read(privateKey); err != nil {
			return "", "", err
		}
	}
	// Preserve the private bytes returned by the old implementation. X25519
	// performs the remaining scalar pruning when deriving the public key.
	privateKey[0] &= 248
	privateKey[31] &= 127
	key, err := ecdh.X25519().NewPrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	return encoding.EncodeToString(key.PublicKey().Bytes()), encoding.EncodeToString(privateKey), nil
}
