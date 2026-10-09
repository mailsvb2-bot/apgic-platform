package privacy

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const PurposeGrowthSessionProjection = "GROWTH_SESSION_PROJECTION"

var (
	ErrInvalidConsent = errors.New("invalid consent record")
	ErrConsentNotFound = errors.New("consent record not found")
	ErrConsentRevoked = errors.New("consent record is revoked")
)

type ConsentRecord struct {
	ID                string          `json:"consent_id"`
	SubjectID         string          `json:"subject_id"`
	Purpose           string          `json:"purpose"`
	Scope             string          `json:"scope"`
	PolicyVersion     string          `json:"policy_version"`
	TextHashOrVersion string          `json:"text_hash_or_version"`
	GrantedAt         time.Time       `json:"granted_at"`
	RevokedAt         *time.Time      `json:"revoked_at,omitempty"`
	Source            string          `json:"source"`
	ProofMetadata     json.RawMessage `json:"proof_metadata"`
}

type ConsentStore interface {
	RecordConsent(ConsentRecord) (persisted ConsentRecord, idempotent bool, err error)
	RevokeConsent(consentID, subjectID string, revokedAt time.Time) (ConsentRecord, bool, error)
	ActiveConsent(subjectID, purpose, scope string, at time.Time) (ConsentRecord, bool, error)
}

func NewConsentRecord(input ConsentRecord) (ConsentRecord, error) {
	record := input
	record.ID = strings.TrimSpace(record.ID)
	record.SubjectID = strings.TrimSpace(record.SubjectID)
	record.Purpose = strings.TrimSpace(record.Purpose)
	record.Scope = strings.TrimSpace(record.Scope)
	record.PolicyVersion = strings.TrimSpace(record.PolicyVersion)
	record.TextHashOrVersion = strings.TrimSpace(record.TextHashOrVersion)
	record.Source = strings.TrimSpace(record.Source)
	if record.ID == "" || record.SubjectID == "" || record.Purpose == "" ||
		record.Scope == "" || record.PolicyVersion == "" || record.TextHashOrVersion == "" ||
		record.GrantedAt.IsZero() || record.Source == "" ||
		len(record.ProofMetadata) == 0 || !json.Valid(record.ProofMetadata) {
		return ConsentRecord{}, ErrInvalidConsent
	}
	var proof map[string]any
	if err := json.Unmarshal(record.ProofMetadata, &proof); err != nil || proof == nil {
		return ConsentRecord{}, ErrInvalidConsent
	}
	record.GrantedAt = record.GrantedAt.UTC()
	record.ProofMetadata = append(json.RawMessage(nil), record.ProofMetadata...)
	if record.RevokedAt != nil {
		value := record.RevokedAt.UTC()
		if value.Before(record.GrantedAt) {
			return ConsentRecord{}, ErrInvalidConsent
		}
		record.RevokedAt = &value
	}
	return record, nil
}

func GrowthConsentScope(bookingID string) string {
	bookingID = strings.TrimSpace(bookingID)
	if bookingID == "" {
		return ""
	}
	return "booking/" + bookingID
}
