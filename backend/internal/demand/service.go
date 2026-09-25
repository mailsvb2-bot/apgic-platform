package demand

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/mailsvb2-bot/apgic-platform/backend/internal/booking"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/consultation"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/identity"
	"github.com/mailsvb2-bot/apgic-platform/backend/internal/marketplace"
)

const holdTTL = 15 * time.Minute

var (
	ErrTextRequired       = errors.New("help intent free text is required")
	ErrIntentNotFound     = errors.New("help intent not found")
	ErrTopicRequired      = errors.New("at least one topic is required")
	ErrTopicUnknown       = errors.New("topic is outside the conformance catalog")
	ErrNotConfirmed       = errors.New("help intent must be confirmed before matching")
	ErrSpecialistNotFound = errors.New("specialist not found")
	ErrSlotNotFound       = errors.New("slot not found")
	ErrSlotNotExclusive   = errors.New("slot is not exclusive")
	ErrSlotUnavailable    = errors.New("slot is not available")
	ErrSlotHeld           = errors.New("slot already has an active hold")
	ErrIdentityMismatch   = errors.New("client identity does not own the help intent")
	ErrHoldNotFound       = errors.New("slot hold not found")
	ErrHoldNotActive      = errors.New("slot hold is not active")
	ErrMethodNotEligible  = errors.New("payment method is not eligible")
	ErrCheckoutLocked     = errors.New("checkout method is already locked")
	ErrCustodyForbidden   = errors.New("APGIC must not accept funds")
	ErrOrderNotFound      = errors.New("checkout order not found")
	ErrEvidenceMismatch   = errors.New("provider evidence does not match the instruction")
	ErrDuplicateEffect    = errors.New("order already has a captured economic effect")
	ErrCancelNotAllowed   = errors.New("booking cannot be cancelled")
	ErrConsultNotReady    = errors.New("consultation is not ready")
	ErrConsultEvidence    = errors.New("consultation completion evidence required")
	ErrRecoveryInvalid    = errors.New("invalid communication recovery input")
	ErrNotDeletion        = errors.New("deactivation is not account deletion")
	ErrPurposeConsent     = errors.New("purpose-specific consent required")
)

type Intent struct {
	ID                string                       `json:"id"`
	ClientIdentityID  string                       `json:"client_identity_id"`
	FreeText          string                       `json:"free_text"`
	Topics            []string                     `json:"topics"`
	Goals             []string                     `json:"goals"`
	Context           map[string]string            `json:"context"`
	Status            marketplace.HelpIntentStatus `json:"status"`
	ReasonCodes       []string                     `json:"reason_codes"`
	Notice            string                       `json:"notice"`
	DiagnosisAsserted bool                         `json:"diagnosis_asserted"`
	CatalogMode       string                       `json:"catalog_mode"`
}

type MatchCard struct {
	SpecialistID string   `json:"specialist_id"`
	DisplayName  string   `json:"display_name"`
	Profession   string   `json:"profession"`
	PriceMinor   int64    `json:"price_minor"`
	Currency     string   `json:"currency"`
	Format       string   `json:"format"`
	Sponsored    bool     `json:"sponsored"`
	ReasonCodes  []string `json:"reason_codes"`
}

type Hold struct {
	ID               string        `json:"id"`
	BookingID        string        `json:"booking_id"`
	SlotID           string        `json:"slot_id"`
	ClientIdentityID string        `json:"client_identity_id"`
	State            string        `json:"state"`
	BookingState     booking.State `json:"booking_state"`
	ExpiresAt        time.Time     `json:"expires_at"`
	ReasonCode       string        `json:"reason_code"`
}

type Service struct {
	mu            sync.Mutex
	now           func() time.Time
	catalog       catalog
	intents       map[string]*Intent
	owners        map[string]*identity.Identity
	holds         map[string]*Hold
	slotHolds     map[string]string
	bookings      map[string]*booking.Booking
	instructions  map[string]*CheckoutInstruction
	evidence      map[string]*PaymentEvidence
	orderEvidence map[string]string
	reversals     map[string]*Cancellation
	notices       map[string]*BookingNotice
	sessions      map[string]*consultation.Session
	projection    marketplace.SearchProjection
	searchStale   bool
	deletions     map[string]*AccountDeletion
}

func NewConformanceService(now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{
		now:           now,
		catalog:       conformanceCatalog(now().UTC()),
		intents:       map[string]*Intent{},
		owners:        map[string]*identity.Identity{},
		holds:         map[string]*Hold{},
		slotHolds:     map[string]string{},
		bookings:      map[string]*booking.Booking{},
		instructions:  map[string]*CheckoutInstruction{},
		evidence:      map[string]*PaymentEvidence{},
		orderEvidence: map[string]string{},
		reversals:     map[string]*Cancellation{},
		notices:       map[string]*BookingNotice{},
		sessions:      map[string]*consultation.Session{},
		deletions:     map[string]*AccountDeletion{},
	}
}

func (s *Service) CreateIntent(freeText string) (*Intent, error) {
	if isBlank(freeText) {
		return nil, ErrTextRequired
	}
	suggestion := interpret(freeText)
	owner, err := identity.New(newID("idn-"), identity.RoleClient)
	if err != nil {
		return nil, err
	}
	draft, err := marketplace.NewHelpIntent(newID("hi-"), freeText, marketplace.HelpIntentInterpretation{
		Topics: suggestion.Topics,
		Goals:  suggestion.Goals,
		Context: map[string]string{
			"format":       FormatOnline,
			"language":     LanguageRU,
			"jurisdiction": JurisdictionRU,
			"age_group":    AgeGroupAdult,
		},
	})
	if err != nil {
		return nil, err
	}
	intent := &Intent{
		ID:                draft.ID,
		ClientIdentityID:  owner.ID,
		FreeText:          draft.FreeText,
		Topics:            append([]string(nil), draft.Topics...),
		Goals:             append([]string(nil), draft.Goals...),
		Context:           cloneMap(draft.Context),
		Status:            draft.Status,
		ReasonCodes:       append([]string(nil), suggestion.ReasonCodes...),
		Notice:            InterpretationNotice,
		DiagnosisAsserted: false,
		CatalogMode:       CatalogModeConformance,
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.intents[intent.ID] = intent
	s.owners[owner.ID] = owner
	return cloneIntent(intent), nil
}

func (s *Service) ConfirmIntent(id string, topics, goals []string, context map[string]string) (*Intent, error) {
	cleanedTopics := compact(topics)
	if len(cleanedTopics) == 0 {
		return nil, ErrTopicRequired
	}
	for _, topic := range cleanedTopics {
		if !s.knownTopic(topic) {
			return nil, ErrTopicUnknown
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	intent, ok := s.intents[id]
	if !ok {
		return nil, ErrIntentNotFound
	}
	merged := cloneMap(intent.Context)
	for key, value := range context {
		switch key {
		case "format", "language", "jurisdiction", "age_group":
			if clean := cleanToken(value); clean != "" {
				merged[key] = clean
			}
		}
	}
	domain := marketplace.HelpIntent{
		ID:       intent.ID,
		FreeText: intent.FreeText,
		Status:   intent.Status,
	}
	domain.Confirm(cleanedTopics, compact(goals), merged)
	intent.Topics = append([]string(nil), domain.Topics...)
	intent.Goals = append([]string(nil), domain.Goals...)
	intent.Context = cloneMap(domain.Context)
	intent.Status = domain.Status
	intent.ReasonCodes = []string{ReasonInterpretationLexicon, ReasonNotADiagnosis, "HELP_INTENT_CONFIRMED"}
	intent.DiagnosisAsserted = false
	return cloneIntent(intent), nil
}

func (s *Service) Matches(intentID, topic string) ([]MatchCard, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent, ok := s.intents[intentID]
	if !ok {
		return nil, "", ErrIntentNotFound
	}
	if topic == "" && len(intent.Topics) > 0 {
		topic = intent.Topics[0]
	}
	domainIntent := marketplace.HelpIntent{
		ID:       intent.ID,
		FreeText: intent.FreeText,
		Topics:   append([]string(nil), intent.Topics...),
		Goals:    append([]string(nil), intent.Goals...),
		Context:  cloneMap(intent.Context),
		Status:   intent.Status,
	}
	matches, err := marketplace.MatchEligible(s.catalog.policy, marketplace.MatchRequest{
		Intent:       domainIntent,
		Topic:        topic,
		Jurisdiction: contextOr(intent.Context, "jurisdiction", JurisdictionRU),
		Format:       contextOr(intent.Context, "format", FormatOnline),
		AgeGroup:     contextOr(intent.Context, "age_group", AgeGroupAdult),
		Language:     contextOr(intent.Context, "language", LanguageRU),
	}, s.catalog.candidates)
	if err != nil {
		return nil, topic, ErrNotConfirmed
	}
	ranked := marketplace.Rank(marketplace.RankingPolicy{Version: RankingVersion}, matches)
	cards := make([]MatchCard, 0, len(ranked))
	for _, match := range ranked {
		cards = append(cards, MatchCard{
			SpecialistID: match.SpecialistID,
			DisplayName:  match.DisplayName,
			Profession:   professionOf(s.catalog.candidates, match.SpecialistID),
			PriceMinor:   priceOf(s.catalog.candidates, match.SpecialistID),
			Currency:     "RUB",
			Format:       FormatOnline,
			Sponsored:    match.Sponsored,
			ReasonCodes:  append([]string(nil), match.ReasonCodes...),
		})
	}
	return cards, topic, nil
}

func (s *Service) Slots(specialistID string) ([]Slot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.catalog.names[specialistID]; !ok || specialistID == "spec-draft" {
		return nil, ErrSpecialistNotFound
	}
	s.expireHoldsLocked()
	now := s.now().UTC()
	out := make([]Slot, 0, 2)
	for _, slot := range s.catalog.slots {
		if slot.SpecialistID != specialistID || !slot.StartsAt.After(now) {
			continue
		}
		out = append(out, slot)
	}
	return out, nil
}

func (s *Service) AcquireHold(intentID, slotID, clientIdentityID string) (*Hold, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	intent, ok := s.intents[intentID]
	if !ok {
		return nil, ErrIntentNotFound
	}
	if intent.Status != marketplace.HelpIntentConfirmed {
		return nil, ErrNotConfirmed
	}
	if intent.ClientIdentityID != clientIdentityID {
		return nil, ErrIdentityMismatch
	}
	slot, ok := s.slot(slotID)
	if !ok {
		return nil, ErrSlotNotFound
	}
	if !slot.Exclusive {
		return nil, ErrSlotNotExclusive
	}
	now := s.now().UTC()
	if !slot.StartsAt.After(now) {
		return nil, ErrSlotUnavailable
	}
	s.expireHoldsLocked()
	if existingID, held := s.slotHolds[slot.ID]; held {
		existing := s.holds[existingID]
		if existing != nil && existing.ClientIdentityID == clientIdentityID && existing.State == "ACTIVE" {
			copyHold := *existing
			return &copyHold, nil
		}
		return nil, ErrSlotHeld
	}
	expires := now.Add(holdTTL)
	if !slot.StartsAt.After(expires) {
		return nil, ErrSlotUnavailable
	}
	created, err := booking.New(newID("book-"), slot.ID, newID("hold-"), clientIdentityID, expires, slot.StartsAt, slot.EndsAt, now)
	if err != nil {
		return nil, err
	}
	hold := &Hold{
		ID:               created.HoldID,
		BookingID:        created.ID,
		SlotID:           slot.ID,
		ClientIdentityID: clientIdentityID,
		State:            "ACTIVE",
		BookingState:     created.State,
		ExpiresAt:        expires,
		ReasonCode:       "BOOK_HOLD_ACQUIRED",
	}
	s.bookings[created.ID] = created
	s.holds[hold.ID] = hold
	s.slotHolds[slot.ID] = hold.ID
	copyHold := *hold
	return &copyHold, nil
}

func (s *Service) knownTopic(topic string) bool {
	for _, rule := range s.catalog.policy.Rules {
		if rule.Topic == topic {
			return true
		}
	}
	return false
}

func (s *Service) slot(id string) (Slot, bool) {
	for _, slot := range s.catalog.slots {
		if slot.ID == id {
			return slot, true
		}
	}
	return Slot{}, false
}

func (s *Service) expireHoldsLocked() {
	now := s.now().UTC()
	for slotID, holdID := range s.slotHolds {
		hold := s.holds[holdID]
		if hold == nil || !hold.ExpiresAt.After(now) {
			if hold != nil {
				hold.State = "EXPIRED"
			}
			delete(s.slotHolds, slotID)
		}
	}
}

func professionOf(candidates []marketplace.Candidate, id string) string {
	for _, candidate := range candidates {
		if candidate.Profile.ID == id {
			return candidate.Profile.Profession
		}
	}
	return ""
}

func priceOf(candidates []marketplace.Candidate, id string) int64 {
	for _, candidate := range candidates {
		if candidate.Profile.ID == id {
			return candidate.PriceMinor
		}
	}
	return 0
}

func contextOr(values map[string]string, key, fallback string) string {
	if values == nil || values[key] == "" {
		return fallback
	}
	return values[key]
}

func compact(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		cleaned := cleanToken(value)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		out = append(out, cleaned)
	}
	return out
}

func cloneMap(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func cloneIntent(intent *Intent) *Intent {
	copyIntent := *intent
	copyIntent.Topics = append([]string(nil), intent.Topics...)
	copyIntent.Goals = append([]string(nil), intent.Goals...)
	copyIntent.Context = cloneMap(intent.Context)
	copyIntent.ReasonCodes = append([]string(nil), intent.ReasonCodes...)
	return &copyIntent
}

func newID(prefix string) string {
	var buf [6]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(buf[:])
}
