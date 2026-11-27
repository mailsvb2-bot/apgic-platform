package runtimepostgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// AcademyBankForTerminal selects a trusted operator-approved bank credential
// for a webhook's UNTRUSTED terminal selector. This does NOT authenticate the
// webhook: AcademyCommitBankCapture must independently verify its signature
// and the persisted original Commerce order, terminal, amount and GetState.
// A webhook cannot introduce or authorize a different seller or terminal.
func (c *Checker) AcademyBankForTerminal(ctx context.Context, terminal string) (*payments.TBankAcquiring, error) {
	if ctx == nil || c == nil || c.db == nil || terminal == "" || len(terminal) > 128 {
		return nil, academy.ErrUnavailable
	}
	var configID, approvedDigest string
	err := c.db.QueryRowContext(ctx, `
 SELECT seller.provider_config_id::text,seller.approved_config_sha256
 FROM academy_bank_seller_bindings seller
 JOIN payment_provider_config_versions cfg ON cfg.id=seller.provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 WHERE seller.terminal_key=$1 AND seller.revoked_at IS NULL
  AND seller.credential_version_ref=cfg.credential_version_ref
  AND cfg.status='ACTIVE' AND ci.status='ACTIVE'
  AND ci.capability_class='PAYMENT_PROVIDER' AND ci.provider_kind='TBANK'
 `, terminal).Scan(&configID, &approvedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, academy.ErrDenied
	}
	if err != nil {
		return nil, err
	}
	seller, err := academyLoadApprovedBankSellerSecret(configID, terminal, approvedDigest)
	if err != nil {
		return nil, err
	}
	caPath := os.Getenv("APGIC_TBANK_CA_BUNDLE_PATH")
	if !filepath.IsAbs(caPath) {
		return nil, academy.ErrUnavailable
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, academy.ErrUnavailable
	}
	bank, err := payments.NewTBankAcquiringWithCABundle(terminal, seller.Password, ca)
	if err != nil {
		return nil, academy.ErrUnavailable
	}
	return bank, nil
}
