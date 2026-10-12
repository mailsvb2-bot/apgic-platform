// Package persona owns APGIC Avatar Companion business semantics. No external
// persona runtime, model SDK, transcript or provider credential belongs here.
package persona

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	PrivateEphemeral          = "PRIVATE_EPHEMERAL"
	StatePending              = "PENDING"
	StateActive               = "ACTIVE"
	StateFailed               = "FAILED"
	StateEnded                = "ENDED"
	ReasonProviderUnavailable = "PERSONA_PROVIDER_UNAVAILABLE"
)

var (
	ErrInvalidRequest = errors.New("PERSONA_REQUEST_INVALID")
	ErrNotFound       = errors.New("PERSONA_NOT_FOUND")
	ErrUnavailable    = errors.New("PERSONA_NOT_CONFIGURED")
	ErrConflict       = errors.New("PERSONA_STATE_CONFLICT")
	ErrIdempotency    = errors.New("PERSONA_IDEMPOTENCY_CONFLICT")
)
var codePattern = regexp.MustCompile(`^[a-z]{2}(-[A-Z]{2})?$`)

type Start struct {
	PersonaID   string `json:"persona_id"`
	Purpose     string `json:"session_purpose"`
	Mode        string `json:"mode"`
	Language    string `json:"language"`
	Style       string `json:"style"`
	PrivacyMode string `json:"privacy_mode"`
}

func (s Start) Validate() error {
	if strings.TrimSpace(s.PersonaID) == "" || !oneOf(s.Purpose, "LISTEN", "ORGANIZE_THOUGHTS", "PREPARE_HUMAN", "WAIT_FOR_HUMAN", "NAVIGATE") ||
		!oneOf(s.Mode, "VIDEO", "VOICE", "TEXT") || !codePattern.MatchString(s.Language) ||
		!oneOf(s.Style, "CALM", "NEUTRAL", "BUSINESS", "WARM") || s.PrivacyMode != PrivateEphemeral {
		return ErrInvalidRequest
	}
	return nil
}
func oneOf(s string, candidates ...string) bool {
	for _, c := range candidates {
		if s == c {
			return true
		}
	}
	return false
}

type CatalogItem struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Language      string `json:"language"`
	VoiceProfile  string `json:"voice_profile"`
	VisualProfile string `json:"visual_profile"`
	SupportsVideo bool   `json:"supports_video"`
	SupportsVoice bool   `json:"supports_voice"`
	SupportsText  bool   `json:"supports_text"`
}
type Session struct {
	ID                  string     `json:"session_id"`
	UserID              string     `json:"-"`
	PersonaID           string     `json:"persona_id"`
	ProviderInstanceID  string     `json:"-"`
	ProviderSessionRef  string     `json:"-"`
	Purpose             string     `json:"session_purpose"`
	PrivacyMode         string     `json:"privacy_mode"`
	Mode                string     `json:"mode"`
	Language            string     `json:"language"`
	VoiceProfile        string     `json:"voice_profile"`
	VisualProfile       string     `json:"visual_profile"`
	State               string     `json:"state"`
	SafetyState         string     `json:"safety_state"`
	CreatedAt           time.Time  `json:"created_at"`
	StartedAt           *time.Time `json:"started_at,omitempty"`
	EndedAt             *time.Time `json:"ended_at,omitempty"`
	HandoffRequestedAt  *time.Time `json:"handoff_requested_at,omitempty"`
	WaitlistRequestedAt *time.Time `json:"waitlist_requested_at,omitempty"`
	ConsentedSummaryID  *string    `json:"consented_summary_id,omitempty"`
	PolicyVersion       string     `json:"policy_version"`
}

func AllowedTransition(before, after string) bool {
	switch before {
	case StatePending:
		return after == StateActive || after == StateFailed
	case StateActive:
		return after == StateEnded
	default:
		return false
	}
}
func SafeHandoffProjection(s Session) map[string]string {
	// No session content, provider identifier or inferred diagnosis may cross
	// the boundary into matching, growth, intelligence or booking.
	return map[string]string{"avatar_session_id": s.ID, "next_action": "NEW_HELP_INTENT_REQUIRED"}
}
