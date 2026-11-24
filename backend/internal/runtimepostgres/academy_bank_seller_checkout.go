package runtimepostgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

// The seller policy is provisioned by an operator only. It must be approved
// in academy_bank_seller_bindings and its full byte digest must match the
// immutable approval before a client can trigger an acquiring request.
// Secrets NEVER enter SQL, browser responses or logs.
type academyBankSellerSecret struct {
	TerminalKey             string `json:"terminal_key"`
	Password                string `json:"terminal_password"`
	SellerRef               string `json:"seller_ref"`
	CommercialOwnerRef      string `json:"commercial_owner_ref"`
	PaymentRecipientRef     string `json:"payment_recipient_ref"`
	PlatformRole            string `json:"platform_role"`
	FiscalResponsibilityRef string `json:"fiscal_responsibility_ref"`
	RefundResponsibilityRef string `json:"refund_responsibility_ref"`
	PayoutBeneficiaryRef    string `json:"payout_beneficiary_ref"`
	LegalPolicyVersion      string `json:"legal_policy_version"`
	PricingPolicyVersion    string `json:"pricing_policy_version"`
	CommissionPolicyVersion string `json:"commission_policy_version"`
	RoutingPolicyVersion    string `json:"routing_policy_version"`
	CommissionMinor         int64  `json:"commission_minor"`
	FiscalTaxation          string `json:"fiscal_taxation"`
	ReceiptTax              string `json:"receipt_tax"`
	ReceiptPaymentObject    string `json:"receipt_payment_object"`
	ReceiptEmail            string `json:"receipt_email"`
	Description             string `json:"description"`
}

func academyLoadApprovedBankSellerSecret(configID, terminal, expectedDigest string) (academyBankSellerSecret, error) {
	if !academyMediaUUID(configID) || terminal == "" || len(expectedDigest) != 64 {
		return academyBankSellerSecret{}, academy.ErrUnavailable
	}
	directory := filepath.Clean(os.Getenv("APGIC_ACADEMY_TBANK_SECRETS_DIR"))
	if !filepath.IsAbs(directory) || !strings.HasPrefix(directory, "/run/secrets/apgic/") {
		return academyBankSellerSecret{}, academy.ErrUnavailable
	}
	return academyLoadApprovedBankSellerSecretAtPath(filepath.Join(directory, configID+".json"), terminal, expectedDigest)
}

// Isolated test seam for secure file material. The actual runtime always
// obtains this path from a server-controlled /run/secrets directory.
func academyLoadApprovedBankSellerSecretAtPath(target, terminal, expectedDigest string) (academyBankSellerSecret, error) {
	info, err := os.Lstat(target)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() <= 0 || info.Size() > 16384 {
		return academyBankSellerSecret{}, academy.ErrUnavailable
	}
	raw, err := os.ReadFile(target)
	if err != nil || len(raw) > 16384 {
		return academyBankSellerSecret{}, academy.ErrUnavailable
	}
	digest := sha256.Sum256(raw)
	encoded := hex.EncodeToString(digest[:])
	if subtle.ConstantTimeCompare([]byte(encoded), []byte(expectedDigest)) != 1 {
		return academyBankSellerSecret{}, academy.ErrDenied
	}
	var cfg academyBankSellerSecret
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&cfg) != nil {
		return academyBankSellerSecret{}, academy.ErrUnavailable
	}
	if _, err = decoder.Token(); err != io.EOF {
		return academyBankSellerSecret{}, academy.ErrUnavailable
	}
	if cfg.TerminalKey != terminal || cfg.Password == "" || cfg.Description == "" ||
		len(cfg.Description) > 140 || cfg.ReceiptEmail == "" ||
		cfg.FiscalTaxation == "" || cfg.ReceiptTax == "" || cfg.ReceiptPaymentObject == "" {
		return academyBankSellerSecret{}, academy.ErrDenied
	}
	return cfg, nil
}

// AcademyStartBankCheckout is the real Commerce-owned implementation of the
// signed buyer HTTP adapter, not a second payment pipeline. The default
// configuration disables bank initiation. Each enabled seller must have a
// PostgreSQL operator approval and an exact-matching protected secret file.
func (c *Checker) AcademyStartBankCheckout(
	ctx context.Context, buyer, courseID, idempotencyKey string,
) (academy.BankCheckoutRedirect, error) {
	if ctx == nil || !academyMediaUUID(buyer) || !academyMediaUUID(courseID) ||
		idempotencyKey == "" {
		return academy.BankCheckoutRedirect{}, academy.ErrInvalid
	}
	if c == nil || c.db == nil || os.Getenv("APGIC_ACADEMY_TBANK_CHECKOUT_ENABLED") != "true" {
		return academy.BankCheckoutRedirect{}, academy.ErrUnavailable
	}
	offer, err := c.AcademyCheckoutOffer(courseID)
	if err != nil {
		return academy.BankCheckoutRedirect{}, err
	}
	if offer.PriceMinor <= 0 || offer.Currency != "RUB" || offer.AccessMode != "PAID" {
		return academy.BankCheckoutRedirect{}, academy.ErrState
	}
	var configID, terminal, approvedHash string
	err = c.db.QueryRowContext(ctx, `
 SELECT seller.provider_config_id::text,seller.terminal_key,
  seller.approved_config_sha256
 FROM academy_courses course
 JOIN products product ON product.id=course.product_id
 JOIN academy_bank_seller_bindings seller ON
  seller.product_owner_ref=lower(product.owner_type)||'/'||product.owner_id::text
 JOIN payment_provider_config_versions cfg ON cfg.id=seller.provider_config_id
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 WHERE course.id=$1::uuid AND course.state='PUBLISHED'
  AND product.status='PUBLISHED' AND seller.revoked_at IS NULL
  AND cfg.credential_version_ref=seller.credential_version_ref
  AND cfg.status='ACTIVE' AND ci.status='ACTIVE'
  AND ci.capability_class='PAYMENT_PROVIDER' AND ci.provider_kind='TBANK'
 `, courseID).Scan(&configID, &terminal, &approvedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return academy.BankCheckoutRedirect{}, academy.ErrUnavailable
	}
	if err != nil {
		return academy.BankCheckoutRedirect{}, err
	}
	seller, err := academyLoadApprovedBankSellerSecret(configID, terminal, approvedHash)
	if err != nil {
		return academy.BankCheckoutRedirect{}, err
	}
	rootPath := os.Getenv("APGIC_TBANK_CA_BUNDLE_PATH")
	if !filepath.IsAbs(rootPath) {
		return academy.BankCheckoutRedirect{}, academy.ErrUnavailable
	}
	roots, err := os.ReadFile(rootPath)
	if err != nil {
		return academy.BankCheckoutRedirect{}, academy.ErrUnavailable
	}
	bank, err := payments.NewTBankAcquiringWithCABundle(terminal, seller.Password, roots)
	if err != nil {
		return academy.BankCheckoutRedirect{}, academy.ErrUnavailable
	}
	policy := AcademyBankLegalPolicy{
		SellerRef: seller.SellerRef, CommercialOwnerRef: seller.CommercialOwnerRef,
		PaymentRecipientRef: seller.PaymentRecipientRef, PlatformRole: seller.PlatformRole,
		FiscalResponsibilityRef: seller.FiscalResponsibilityRef,
		RefundResponsibilityRef: seller.RefundResponsibilityRef,
		PayoutBeneficiaryRef:    seller.PayoutBeneficiaryRef,
		LegalPolicyVersion:      seller.LegalPolicyVersion,
		PricingPolicyVersion:    seller.PricingPolicyVersion,
		CommissionPolicyVersion: seller.CommissionPolicyVersion,
		RoutingPolicyVersion:    seller.RoutingPolicyVersion,
		CommissionMinor:         seller.CommissionMinor, ProviderConfigID: configID,
	}
	receipt, err := json.Marshal(map[string]any{
		"Taxation": seller.FiscalTaxation, "Email": seller.ReceiptEmail,
		"Items": []map[string]any{{
			"Name": seller.Description, "Price": offer.PriceMinor,
			"Quantity": 1, "Amount": offer.PriceMinor,
			"Tax": seller.ReceiptTax, "PaymentObject": seller.ReceiptPaymentObject,
		}},
		"Payments": map[string]any{"Electronic": offer.PriceMinor},
	})
	if err != nil {
		return academy.BankCheckoutRedirect{}, academy.ErrUnavailable
	}
	outcome, err := c.AcademyRunBankCheckout(ctx, buyer, courseID, idempotencyKey,
		policy, receipt, seller.Description,
		"https://apgic.ru/v1/payments/tbank/notification", bank)
	if err != nil {
		return academy.BankCheckoutRedirect{}, err
	}
	return academy.BankCheckoutRedirect{
		OrderID: outcome.OrderID, PaymentURL: outcome.PaymentURL, State: outcome.State,
	}, nil
}
