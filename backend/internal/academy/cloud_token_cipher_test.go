package academy

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCloudTokenCipherOwnerBinding(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := NewCloudTokenCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	value, err := cipher.Encrypt("identity-one", "YANDEX_DISK", "secret-access-token")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(value, "secret-access-token") {
		t.Fatal("token written in plaintext")
	}
	plain, err := cipher.Decrypt("identity-one", "YANDEX_DISK", value)
	if err != nil || plain != "secret-access-token" {
		t.Fatal("credential round trip failed")
	}
	if _, err := cipher.Decrypt("identity-two", "YANDEX_DISK", value); err == nil {
		t.Fatal("other identity decrypted credential")
	}
	if _, err := cipher.Decrypt("identity-one", "EXTERNAL", value); err == nil {
		t.Fatal("other provider decrypted credential")
	}
	other, err := cipher.Encrypt("identity-one", "YANDEX_DISK", "secret-access-token")
	if err != nil || other == value {
		t.Fatal("nonce was not unique")
	}
}

func TestCloudTokenCipherRejectsInvalidKeyAndTampering(t *testing.T) {
	if _, err := NewCloudTokenCipher("invalid"); err == nil {
		t.Fatal("invalid key accepted")
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := NewCloudTokenCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	value, err := cipher.Encrypt("identity-one", "YANDEX_DISK", "secret")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	blob[len(blob)-1] ^= 0x01
	if _, err := cipher.Decrypt("identity-one", "YANDEX_DISK", base64.RawURLEncoding.EncodeToString(blob)); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
}
