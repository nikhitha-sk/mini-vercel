// Package secrets encrypts project environment variables at rest with
// AES-256-GCM. Plaintext values only exist inside the API request that sets
// them and inside the build container that consumes them.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
)

type Box struct{ aead cipher.AEAD }

func New(key string) (*Box, error) {
	raw, err := hex.DecodeString(key)
	if err != nil || len(raw) != 32 {
		raw, err = base64.StdEncoding.DecodeString(key)
		if err != nil || len(raw) != 32 {
			return nil, errors.New("MV_ENCRYPTION_KEY must be 32 bytes, hex or base64 encoded")
		}
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal encrypts plaintext. additional binds the ciphertext to a context
// (project id + key) so rows cannot be swapped between projects.
func (b *Box) Seal(plaintext, additional string) (ciphertext, nonce []byte, err error) {
	nonce = make([]byte, b.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return b.aead.Seal(nil, nonce, []byte(plaintext), []byte(additional)), nonce, nil
}

func (b *Box) Open(ciphertext, nonce []byte, additional string) (string, error) {
	pt, err := b.aead.Open(nil, nonce, ciphertext, []byte(additional))
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
