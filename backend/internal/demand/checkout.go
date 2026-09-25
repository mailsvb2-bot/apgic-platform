package demand

import (
	"strings"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/commerce"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/legal"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/payments"
)

const (
	checkoutNotice = "APGIC не принимает деньги. Оплату исполняет внешний провайдер, получатель — специалист."
	platformRole   = "MARKETPLACE_INTERMEDIARY"
	routingVersion = "routing-conformance-v1"
)

type CheckoutOption struct {
	MethodCode         string `json:"method_code"`
	RailCode           string `json:"rail_code"`
	ProviderID         string `json:"provider_id"`
	ExecutionOwner     string `json:"execution_owner"`
	AmountMinor        int64  `json:"amount_minor"`
	Currency           string `json:"currency"`
	PaymentRecipientID string `json:"payment_recipient_id"`
	PlatformRole       string `json:"platform_role"`
	APGICAcceptsFunds  bool   `json:"apgic_accepts_funds"`
}

type CheckoutInstruction struct {
	ID                 string        `json:"id"`
	HoldID             string        `json:"hold_id"`
	BookingID          string        `json:"booking_id"`
	BookingState       booking.State `json:"booking_state"`
	OrderID            string        `json:"order_id"`
	ProviderID         string        `json:"provider_id"`
	MethodCode         string        `json:"method_code"`
	RailCode           string        `json:"rail_code"`
	AmountMinor        int64         `json:"amount_minor"`
	Currency           string        `json:"currency"`
	ExecutionOwner     string        `json:"execution_owner"`
	PaymentRecipientID string        `json:"payment_recipient_id"`
	PlatformRole       string        `json:"platform_role"`
	APGICAcceptsFunds  bool          `json:"apgic_accepts_funds"`
	Notice             string        `json:"notice"`
	ReasonCode         string        `json:"reason_code"`
}

func externalProviders() []payments.ProviderInstance {
	return []payments.ProviderInstance{{
		Manifest: payments.ProviderManifest{
			Version:                   "conformance-external-v1",
			ProviderID:                "external-bank",
			Certified:                 true,
			CertificationEvidenceRefs: []string{"conformance:not-production-psp"},
			ExecutionOwner:            payments.ExternalExecutionOwner,
			Methods:                   []payments.MethodCode{payments.MethodBankCard, payments.MethodSBP},
			Rails:                     []payments.RailCode{payments.RailBankTransfer},
			Currencies:                []string{"RUB"},
			Jurisdictions:             []string{JurisdictionRU},
		},
		Status: payments.ProviderActive,
		Health: payments.HealthHealthy,
	}}
}

func (s *Service) CheckoutOptions(holdID, clientIdentityID string) ([]CheckoutOption, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hold, slot, err := s.activeHoldLocked(holdID, clientIdentityID)
	if err != nil {
		return nil, err
	}
	recipient := s.recipientLocked(slot.SpecialistID)
	amount := priceOf(s.catalog.candidates, slot.SpecialistID)
	methods := payments.EligibleMethods(payments.CatalogContext{
		Currency:     "RUB",
		Jurisdiction: JurisdictionRU,
		Rail:         payments.RailBankTransfer,
	}, externalProviders())
	options := make([]CheckoutOption, 0, len(methods))
	for _, method := range methods {
		if method == payments.MethodWallet {
			continue
		}
		options = append(options, CheckoutOption{
			MethodCode:         string(method),
			RailCode:           string(payments.RailBankTransfer),
			ProviderID:         "external-bank",
			ExecutionOwner:     payments.ExternalExecutionOwner,
			AmountMinor:        amount,
			Currency:           "RUB",
			PaymentRecipientID: recipient,
			PlatformRole:       platformRole,
			APGICAcceptsFunds:  false,
		})
	}
	if err := rejectCustody(recipient, payments.ExternalExecutionOwner); err != nil {
		return nil, err
	}
	_ = hold
	return options, nil
}

func (s *Service) CreateCheckout(holdID, clientIdentityID, methodCode string) (*CheckoutInstruction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hold, slot, err := s.activeHoldLocked(holdID, clientIdentityID)
	if err != nil {
		return nil, err
	}
	if existing := s.instructions[hold.ID]; existing != nil {
		if existing.MethodCode == methodCode {
			copyInstruction := *existing
			return &copyInstruction, nil
		}
		return nil, ErrCheckoutLocked
	}
	method := payments.MethodCode(methodCode)
	if method == payments.MethodWallet || method == "" {
		return nil, ErrMethodNotEligible
	}
	recipient := s.recipientLocked(slot.SpecialistID)
	if err := rejectCustody(recipient, payments.ExternalExecutionOwner); err != nil {
		return nil, err
	}
	amount := priceOf(s.catalog.candidates, slot.SpecialistID)
	now := s.now().UTC()
	orderID := newID("ord-")
	legalSnapshot := legal.TransactionSnapshot{
		SellerOrServiceProviderID: recipient,
		CommercialOwnerID:         recipient,
		PaymentRecipientID:        recipient,
		PlatformRole:              platformRole,
		FiscalResponsibilityID:    recipient,
		RefundResponsibilityID:    recipient,
		PayoutBeneficiaryID:       recipient,
		PolicyVersion:             "legal-conformance-v1",
	}
	if err := legalSnapshot.Validate(); err != nil {
		return nil, err
	}
	if _, err := commerce.NewOrderSnapshot(commerce.OrderSnapshot{
		ID:                      orderID,
		BookingID:               hold.BookingID,
		OfferRef:                "offer:" + slot.SpecialistID,
		PriceSourceRef:          "catalog:conformance",
		AmountMinor:             amount,
		Currency:                "RUB",
		CommissionMinor:         0,
		PricingPolicyVersion:    "pricing-conformance-v1",
		CommissionPolicyVersion: "commission-conformance-v1",
		LegalSnapshotRef:        legalSnapshot.PolicyVersion,
		SellerRef:               recipient,
		CommercialOwnerRef:      recipient,
		PaymentRecipientRef:     recipient,
		PlatformRole:            platformRole,
		FiscalResponsibilityRef: recipient,
		RefundResponsibilityRef: recipient,
		PayoutBeneficiaryRef:    recipient,
		CapturedAt:              now,
	}); err != nil {
		return nil, err
	}
	decision, err := payments.SelectProvider(payments.TransactionContext{
		OrderID:      orderID,
		AmountMinor:  amount,
		Currency:     "RUB",
		Jurisdiction: JurisdictionRU,
		Method:       method,
		Rail:         payments.RailBankTransfer,
	}, externalProviders(), payments.RoutingPolicy{
		Version:          routingVersion,
		OrderedProviders: []payments.ProviderID{"external-bank"},
	})
	if err != nil {
		return nil, ErrMethodNotEligible
	}
	instruction := payments.PaymentInstruction{
		OrderID:          orderID,
		ProviderID:       decision.ProviderID,
		Method:           method,
		Rail:             payments.RailBankTransfer,
		AmountMinor:      amount,
		Currency:         "RUB",
		IdempotencyKey:   hold.ID + ":" + methodCode,
		RoutingPolicyRef: decision.PolicyVersion,
	}
	if err := instruction.Validate(); err != nil {
		return nil, err
	}
	booked := s.bookings[hold.BookingID]
	result, err := booked.Transition(booking.StatePendingPayment, now)
	if err != nil {
		return nil, err
	}
	hold.BookingState = result.To
	created := &CheckoutInstruction{
		ID:                 newID("chk-"),
		HoldID:             hold.ID,
		BookingID:          hold.BookingID,
		BookingState:       result.To,
		OrderID:            orderID,
		ProviderID:         string(decision.ProviderID),
		MethodCode:         methodCode,
		RailCode:           string(payments.RailBankTransfer),
		AmountMinor:        amount,
		Currency:           "RUB",
		ExecutionOwner:     payments.ExternalExecutionOwner,
		PaymentRecipientID: recipient,
		PlatformRole:       platformRole,
		APGICAcceptsFunds:  false,
		Notice:             checkoutNotice,
		ReasonCode:         decision.ReasonCode,
	}
	s.instructions[hold.ID] = created
	copyInstruction := *created
	return &copyInstruction, nil
}

func (s *Service) activeHoldLocked(holdID, clientIdentityID string) (*Hold, Slot, error) {
	s.expireHoldsLocked()
	hold, ok := s.holds[holdID]
	if !ok {
		return nil, Slot{}, ErrHoldNotFound
	}
	if hold.ClientIdentityID != clientIdentityID {
		return nil, Slot{}, ErrIdentityMismatch
	}
	if hold.State != "ACTIVE" {
		return nil, Slot{}, ErrHoldNotActive
	}
	slot, ok := s.slot(hold.SlotID)
	if !ok {
		return nil, Slot{}, ErrSlotNotFound
	}
	return hold, slot, nil
}

func (s *Service) recipientLocked(specialistID string) string {
	for _, candidate := range s.catalog.candidates {
		if candidate.Profile.ID == specialistID {
			return candidate.Profile.IdentityID
		}
	}
	return ""
}

func rejectCustody(recipient, executionOwner string) error {
	foldedRecipient := strings.ToLower(recipient)
	if recipient == "" || strings.Contains(foldedRecipient, "apgic") || executionOwner != payments.ExternalExecutionOwner {
		return ErrCustodyForbidden
	}
	return nil
}
