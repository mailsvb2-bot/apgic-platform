package runtimepostgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/academy"
)

// AcademyBankLegalPolicy must be sourced from approved server-side seller,
// fiscal and commercial configuration. Never populate these values from a
// browser request, course author label or placeholder defaults.
type AcademyBankLegalPolicy struct {
	SellerRef               string
	CommercialOwnerRef      string
	PaymentRecipientRef     string
	PlatformRole            string
	FiscalResponsibilityRef string
	RefundResponsibilityRef string
	PayoutBeneficiaryRef    string
	LegalPolicyVersion      string
	PricingPolicyVersion    string
	CommissionPolicyVersion string
	RoutingPolicyVersion    string
	CommissionMinor         int64
	ProviderConfigID        string
}

// AcademyBankAttempt is an immutable persisted Commerce instruction.
// CREATED never means a bank charge was initiated or captured.
type AcademyBankAttempt struct {
	OrderID           string
	AttemptID         string
	ProviderConfigID  string
	ProductID         string
	CourseID          string
	BuyerID           string
	AmountMinor       int64
	Currency          string
	State             string
	ProviderReference sql.NullString
	Replayed          bool
}

// AcademyPrepareBankAttempt writes exactly one canonical legal snapshot,
// Academy order, routing decision and CREATED payment attempt in one
// PostgreSQL transaction. It DOES NOT contact the PSP and DOES NOT issue
// an entitlement. The Init executor must only run after this commits.
// A replay never authorizes a second bank Init by itself.
func (c *Checker) AcademyPrepareBankAttempt(buyer, courseID, idempotency string, policy AcademyBankLegalPolicy) (AcademyBankAttempt, error) {
	if !academyMediaUUID(buyer) || !academyMediaUUID(courseID) ||
		len(idempotency) < 8 || len(idempotency) > 128 ||
		strings.Trim(idempotency, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._~-") != "" ||
		!academyMediaUUID(policy.ProviderConfigID) {
		return AcademyBankAttempt{}, academy.ErrInvalid
	}
	for _, v := range []string{policy.SellerRef, policy.CommercialOwnerRef,
		policy.PaymentRecipientRef, policy.PlatformRole, policy.FiscalResponsibilityRef,
		policy.RefundResponsibilityRef, policy.PayoutBeneficiaryRef,
		policy.LegalPolicyVersion, policy.PricingPolicyVersion,
		policy.CommissionPolicyVersion, policy.RoutingPolicyVersion} {
		if strings.TrimSpace(v) == "" || strings.TrimSpace(v) != v || len(v) > 200 {
			return AcademyBankAttempt{}, academy.ErrInvalid
		}
	}
	ctx, cancel, tx, err := academyBegin(c)
	defer cancel()
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	defer tx.Rollback()
	// Identical buyer+key cannot create concurrent orders across HTTP workers.
	if _, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,887012))`, buyer+"/"+idempotency); err != nil {
		return AcademyBankAttempt{}, err
	}
	var existing AcademyBankAttempt
	err = tx.QueryRowContext(ctx, `SELECT o.id::text,pa.id::text,route.provider_config_id::text,
 o.product_id::text,substring(o.offer_ref from 16),o.direct_buyer_identity_id::text,
 o.amount_minor,o.currency,pa.state,pa.provider_reference
 FROM orders o JOIN payment_attempts pa ON pa.order_id=o.id
 JOIN payment_routing_decisions route ON route.id=pa.routing_decision_id
 WHERE o.direct_buyer_identity_id=$1::uuid AND o.direct_checkout_idempotency_key=$2
 ORDER BY pa.created_at LIMIT 1`, buyer, idempotency).Scan(
		&existing.OrderID, &existing.AttemptID, &existing.ProviderConfigID,
		&existing.ProductID, &existing.CourseID, &existing.BuyerID,
		&existing.AmountMinor, &existing.Currency, &existing.State, &existing.ProviderReference)
	if err == nil {
		if existing.CourseID != courseID || existing.ProviderConfigID != policy.ProviderConfigID {
			return AcademyBankAttempt{}, academy.ErrConflict
		}
		existing.Replayed = true
		if err = tx.Commit(); err != nil {
			return AcademyBankAttempt{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return AcademyBankAttempt{}, err
	}
	if err = academyConfigureLearnerVideoRelease(ctx, tx); err != nil {
		return AcademyBankAttempt{}, err
	}
	var productID, commercialOwner, revenueBeneficiary, productOwnerRef string
	var price, revision int64
	var direction sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT p.id::text,p.commercial_owner_ref,p.revenue_beneficiary_ref,
 lower(p.owner_type)||'/'||p.owner_id::text,p.organization_direction_id::text,
 t.price_minor,t.revision
 FROM academy_courses c JOIN products p ON p.id=c.product_id
 JOIN commerce_product_access_terms t ON t.product_id=p.id
 WHERE c.id=$1::uuid AND c.state='PUBLISHED' AND p.status='PUBLISHED'
 AND t.access_mode='PAID' AND t.currency='RUB' AND t.price_minor>0
 AND EXISTS(SELECT 1 FROM academy_course_versions v
  WHERE v.course_id=c.id AND v.state='PUBLISHED'
   AND v.version_no=(SELECT max(x.version_no) FROM academy_course_versions x
    WHERE x.course_id=c.id AND x.state='PUBLISHED')
   AND academy_video_learner_delivery_ready(v.id))
 FOR SHARE OF p,t`, courseID).Scan(
		&productID, &commercialOwner, &revenueBeneficiary, &productOwnerRef,
		&direction, &price, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return AcademyBankAttempt{}, academy.ErrState
	}
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	if policy.CommercialOwnerRef != commercialOwner || policy.PayoutBeneficiaryRef != revenueBeneficiary ||
		policy.CommissionMinor < 0 || policy.CommissionMinor > price {
		return AcademyBankAttempt{}, academy.ErrDenied
	}
	// Only certified, active provider configurations with recent healthy
	// evidence may receive new bank attempts. Old snapshots are not sufficient.
	var healthID string
	err = tx.QueryRowContext(ctx, `SELECT h.id::text
 FROM payment_provider_config_versions cfg
 JOIN connector_instances ci ON ci.id=cfg.provider_instance_id
 JOIN LATERAL(
  SELECT id,health,guardrail_action,observed_at FROM payment_provider_health_snapshots
  WHERE provider_config_id=cfg.id ORDER BY observed_at DESC LIMIT 1
 ) h ON true
 WHERE cfg.id=$1::uuid AND cfg.status='ACTIVE'
  AND cfg.effective_from<=now() AND ci.status='ACTIVE'
  AND ci.capability_class='PAYMENT_PROVIDER'
  AND ci.provider_kind='TBANK'
  AND 'RU'=ANY(cfg.jurisdiction_codes) AND 'RUB'=ANY(cfg.currencies)
  AND 'BANK_CARD'=ANY(cfg.method_codes)
  AND 'APGIC_PAYMENT_PROVIDER'=ANY(cfg.rail_codes)
  AND EXISTS (
    SELECT 1 FROM academy_bank_seller_bindings binding
    WHERE binding.product_owner_ref=$2
      AND binding.provider_config_id=cfg.id
      AND binding.credential_version_ref=cfg.credential_version_ref
      AND binding.revoked_at IS NULL
  )
  AND h.health='HEALTHY' AND h.guardrail_action='ALLOW_NEW_ATTEMPTS'
  AND h.observed_at>=now()-interval '10 minutes'
 FOR SHARE OF cfg`, policy.ProviderConfigID, productOwnerRef).Scan(&healthID)
	if errors.Is(err, sql.ErrNoRows) {
		return AcademyBankAttempt{}, academy.ErrUnavailable
	}
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	orderID, err := academyID()
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	legalID, err := academyID()
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	routeID, err := academyID()
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	attemptID, err := academyID()
	if err != nil {
		return AcademyBankAttempt{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO legal_transaction_snapshots(
 id,transaction_ref,seller_or_service_provider_id,commercial_owner_id,
 payment_recipient_id,platform_role,fiscal_responsibility_id,
 refund_responsibility_id,payout_beneficiary_id,policy_version,occurred_at)
 VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())`,
		legalID, "order/"+orderID, policy.SellerRef, policy.CommercialOwnerRef,
		policy.PaymentRecipientRef, policy.PlatformRole, policy.FiscalResponsibilityRef,
		policy.RefundResponsibilityRef, policy.PayoutBeneficiaryRef, policy.LegalPolicyVersion)
	if err != nil {
		return AcademyBankAttempt{}, fmt.Errorf("canonical bank legal snapshot: %w", err)
	}
	// The DB guard rechecks current price, author ownership, delivery status,
	// published product and legal role snapshot at the actual insert boundary.
	_, err = tx.ExecContext(ctx, `INSERT INTO orders(
 id,booking_id,direct_buyer_identity_id,direct_checkout_idempotency_key,
 offer_ref,price_source_ref,amount_minor,currency,commission_minor,
 pricing_policy_version,commission_policy_version,legal_snapshot_id,
 product_id,organization_direction_id,product_owner_ref,author_refs,
 seller_ref,commercial_owner_ref,payment_recipient_ref,platform_role,
 fiscal_responsibility_ref,refund_responsibility_ref,payout_beneficiary_ref,
 captured_at)
 SELECT $1::uuid,NULL,$2::uuid,$3,
 'academy/course/'||$4::text,'academy/offer/revision/'||$5::text,
 $6,'RUB',$7,$8,$9,$10::uuid,p.id,p.organization_direction_id,
 lower(p.owner_type)||'/'||p.owner_id::text,p.author_refs,
 $11,$12,$13,$14,$15,$16,$17,now()
 FROM products p WHERE p.id=$18::uuid`,
		orderID, buyer, idempotency, courseID, revision, price, policy.CommissionMinor,
		policy.PricingPolicyVersion, policy.CommissionPolicyVersion, legalID,
		policy.SellerRef, policy.CommercialOwnerRef, policy.PaymentRecipientRef,
		policy.PlatformRole, policy.FiscalResponsibilityRef,
		policy.RefundResponsibilityRef, policy.PayoutBeneficiaryRef, productID)
	if err != nil {
		return AcademyBankAttempt{}, fmt.Errorf("canonical academy bank order: %w", err)
	}
	// Candidate and health evidence are sourced from the certified provider and
	// its recorded health snapshot, never a fabricated HTTP request payload.
	_, err = tx.ExecContext(ctx, `INSERT INTO payment_routing_decisions(
 id,order_id,policy_version,provider_config_id,jurisdiction_code,
 selected_method_code,selected_rail_code,candidate_evidence,health_snapshot,
 health_snapshot_id,decided_at)
 SELECT $1::uuid,$2::uuid,$3,cfg.id,'RU','BANK_CARD','APGIC_PAYMENT_PROVIDER',
 jsonb_build_array(jsonb_build_object(
  'provider_config_id',cfg.id,'certification_evidence_refs',cfg.certification_evidence_refs,
  'eligible',true)),
 jsonb_build_object('health',h.health,'guardrail_action',h.guardrail_action,
  'evidence_refs',h.evidence_refs,'observed_at',h.observed_at),
 h.id,now()
 FROM payment_provider_config_versions cfg
 JOIN payment_provider_health_snapshots h ON h.id=$5::uuid AND h.provider_config_id=cfg.id
 WHERE cfg.id=$4::uuid`, routeID, orderID, policy.RoutingPolicyVersion, policy.ProviderConfigID, healthID)
	if err != nil {
		return AcademyBankAttempt{}, fmt.Errorf("canonical bank routing: %w", err)
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO payment_attempts(
 id,order_id,routing_decision_id,idempotency_key,amount_minor,currency,
 state,created_at,updated_at)
 VALUES($1::uuid,$2::uuid,$3::uuid,$4,$5,'RUB','CREATED',now(),now())`,
		attemptID, orderID, routeID, idempotency, price)
	if err != nil {
		return AcademyBankAttempt{}, fmt.Errorf("canonical bank CREATED attempt: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return AcademyBankAttempt{}, err
	}
	return AcademyBankAttempt{OrderID: orderID, AttemptID: attemptID,
		ProviderConfigID: policy.ProviderConfigID, ProductID: productID, CourseID: courseID,
		BuyerID: buyer, AmountMinor: price, Currency: "RUB", State: "CREATED"}, nil
}
