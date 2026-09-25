package demand

import (
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/communication"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/connector"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/notification"
)

const joinLead = 15 * time.Minute

type BookingNotice struct {
	ID                  string `json:"id"`
	BookingID           string `json:"booking_id"`
	Purpose             string `json:"purpose"`
	Transactional       bool   `json:"transactional"`
	RequiresGrowthOptIn bool   `json:"requires_growth_opt_in"`
	Idempotent          bool   `json:"idempotent"`
}

type JoinResult struct {
	Decision           string    `json:"decision"`
	ReasonCode         string    `json:"reason_code"`
	ProviderInstanceID string    `json:"provider_instance_id,omitempty"`
	OpensAt            time.Time `json:"opens_at"`
	ClosesAt           time.Time `json:"closes_at"`
	Notice             string    `json:"notice"`
}

func (s *Service) recordBookingNoticeLocked(bookingID string, now time.Time) {
	if _, exists := s.notices[bookingID]; exists {
		return
	}
	intent, err := notification.NewTransactional(notification.Intent{
		ID:             newID("ntf-"),
		BookingID:      bookingID,
		Purpose:        "BOOKING_CONFIRMED",
		IdempotencyKey: "booking-confirmed:" + bookingID,
		DataClass:      "TRANSACTIONAL_BOOKING",
		CreatedAt:      now,
	})
	if err != nil {
		return
	}
	s.notices[bookingID] = &BookingNotice{
		ID:                  intent.ID,
		BookingID:           bookingID,
		Purpose:             intent.Purpose,
		Transactional:       intent.Transactional,
		RequiresGrowthOptIn: intent.RequiresGrowthOptIn(),
	}
}

func (s *Service) Fulfillment(bookingID, identityID string) (*BookingNotice, *JoinResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	booked := s.bookings[bookingID]
	if booked == nil {
		return nil, nil, ErrOrderNotFound
	}
	notice := s.notices[bookingID]
	var noticeCopy *BookingNotice
	if notice != nil {
		copied := *notice
		noticeCopy = &copied
	}
	slot, _ := s.slot(booked.SlotID)
	specialistID := s.recipientLocked(slot.SpecialistID)
	now := s.now().UTC()
	provider, err := connector.NewInstance("comm-external", connector.CapabilityCommunication, "EXTERNAL_COMMUNICATION_PROVIDER", "conformance:not-production-media")
	if err != nil {
		return nil, nil, err
	}
	provider.Status = connector.StatusActive
	decision, err := communication.AuthorizeJoin(
		communication.JoinRequest{
			IdentityID:     identityID,
			Role:           communication.RoleClient,
			IdempotencyKey: "join:" + bookingID + ":" + identityID,
			Now:            now,
		},
		communication.BookingAccess{
			BookingID:            booked.ID,
			State:                booked.State,
			ClientIdentityID:     booked.ClientIdentityID,
			SpecialistIdentityID: specialistID,
		},
		communication.EntitlementProof{
			ID:          "ent-" + bookingID,
			BookingID:   bookingID,
			IdentityID:  booked.ClientIdentityID,
			Active:      booked.State == "CONFIRMED",
			EvidenceRef: "payment-evidence:" + bookingID,
			ExpiresAt:   booked.EndsAt,
		},
		communication.JoinWindow{OpensAt: booked.StartsAt.Add(-joinLead), ClosesAt: booked.EndsAt},
		provider,
		communication.JoinPolicy{Version: "join-conformance-v1", MaxCredentialTTL: 10 * time.Minute},
	)
	if err != nil {
		return nil, nil, err
	}
	providerID := ""
	if decision.CredentialRequest != nil {
		providerID = decision.ProviderInstanceID
	}
	return noticeCopy, &JoinResult{
		Decision:           string(decision.Decision),
		ReasonCode:         decision.ReasonCode,
		ProviderInstanceID: providerID,
		OpensAt:            booked.StartsAt.Add(-joinLead),
		ClosesAt:           booked.EndsAt,
		Notice:             joinNotice(decision.ReasonCode),
	}, nil
}

func joinNotice(reason string) string {
	switch reason {
	case communication.ReasonAllowed:
		return "Вход разрешён внешнему провайдеру связи. Медиапоток не идёт через APGIC."
	case communication.ReasonOutsideJoinWindow:
		return "Вход откроется за 15 минут до начала. Сейчас окно закрыто."
	case communication.ReasonBookingNotJoinable:
		return "Войти можно только в подтверждённую бронь."
	case communication.ReasonRoleMismatch:
		return "Эта бронь принадлежит другому клиенту."
	default:
		return "Вход в консультацию закрыт."
	}
}
