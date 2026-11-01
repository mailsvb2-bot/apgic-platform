package runtimepostgres

import (
	"os"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyStoreCloudCredential persists only authenticated encrypted credentials.
// The encryption key is required from the server environment; missing keys
// fail closed. This is not a browser endpoint: OAuth callbacks must validate
// their state/PKCE before invoking this operation.
func (c *Checker) AcademyStoreCloudCredential(actor, provider, token string) error {
	if c == nil || c.db == nil {
		return academy.ErrUnavailable
	}
	if !academyMediaUUID(actor) || provider != "YANDEX_DISK" || token == "" {
		return academy.ErrInvalid
	}
	secret := os.Getenv("APGIC_ACADEMY_CLOUD_TOKEN_KEY")
	codec, err := academy.NewCloudTokenCipher(secret)
	if err != nil {
		return academy.ErrUnavailable
	}
	encrypted, err := codec.Encrypt(actor, provider, token)
	if err != nil {
		return academy.ErrInvalid
	}
	ctx, cancel := academyContext()
	defer cancel()
	_, err = c.db.ExecContext(ctx, `INSERT INTO academy_cloud_connections(
 author_id,provider,encrypted_access_token)
 VALUES($1::uuid,$2,$3)
 ON CONFLICT(author_id,provider) DO UPDATE SET
 encrypted_access_token=EXCLUDED.encrypted_access_token,updated_at=now()`,
		actor, provider, encrypted)
	return err
}

// AcademyReadCloudCredential is server-side only and must NEVER be exposed
// through a response DTO, error trace, observability event or log.
func (c *Checker) AcademyReadCloudCredential(actor, provider string) (string, error) {
	if c == nil || c.db == nil {
		return "", academy.ErrUnavailable
	}
	if !academyMediaUUID(actor) || provider != "YANDEX_DISK" {
		return "", academy.ErrDenied
	}
	codec, err := academy.NewCloudTokenCipher(os.Getenv("APGIC_ACADEMY_CLOUD_TOKEN_KEY"))
	if err != nil {
		return "", academy.ErrUnavailable
	}
	ctx, cancel := academyContext()
	defer cancel()
	var encrypted string
	err = c.db.QueryRowContext(ctx, `SELECT encrypted_access_token
 FROM academy_cloud_connections WHERE author_id=$1::uuid AND provider=$2`,
		actor, provider).Scan(&encrypted)
	if err != nil {
		return "", academy.ErrDenied
	}
	token, err := codec.Decrypt(actor, provider, encrypted)
	if err != nil {
		return "", academy.ErrUnavailable
	}
	return token, nil
}

func (c *Checker) AcademyDisconnectCloud(actor, provider string) error {
	if c == nil || c.db == nil {
		return academy.ErrUnavailable
	}
	if !academyMediaUUID(actor) || provider != "YANDEX_DISK" {
		return academy.ErrDenied
	}
	ctx, cancel := academyContext()
	defer cancel()
	result, err := c.db.ExecContext(ctx, `DELETE FROM academy_cloud_connections
 WHERE author_id=$1::uuid AND provider=$2`, actor, provider)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return academy.ErrNotFound
	}
	return nil
}

