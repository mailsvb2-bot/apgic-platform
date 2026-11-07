package academy

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// CloudTokenCipher encrypts provider credentials before durable storage.
// Keys must be supplied by the deployment secret manager, never by clients
// or committed configuration. Ciphertexts are bound to the owner identity.
type CloudTokenCipher struct {
	key [32]byte
}

func NewCloudTokenCipher(keyBase64 string) (CloudTokenCipher, error) {
	var result CloudTokenCipher
	decoded, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil || len(decoded) != 32 {
		return result, errors.New("cloud media encryption key must contain exactly 32 bytes")
	}
	copy(result.key[:], decoded)
	return result, nil
}

func (c CloudTokenCipher) Encrypt(ownerID, provider, token string) (string, error) {
	if ownerID == "" || provider != "YANDEX_DISK" || token == "" || len(token) > 8192 {
		return "", errors.New("invalid cloud credential")
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	aad := []byte("APGIC/academy/cloud-media/v1/" + provider + "/" + ownerID)
	encoded := aead.Seal(nonce, nonce, []byte(token), aad)
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func (c CloudTokenCipher) Decrypt(ownerID, provider, encrypted string) (string, error) {
	if ownerID == "" || provider != "YANDEX_DISK" || encrypted == "" {
		return "", errors.New("invalid cloud credential")
	}
	blob, err := base64.RawURLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", errors.New("invalid encrypted credential")
	}
	block, err := aes.NewCipher(c.key[:])
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("invalid encrypted credential")
	}
	aad := []byte("APGIC/academy/cloud-media/v1/" + provider + "/" + ownerID)
	plain, err := aead.Open(nil, blob[:aead.NonceSize()], blob[aead.NonceSize():], aad)
	if err != nil {
		return "", errors.New("cloud credential authentication failed")
	}
	return string(plain), nil
}
