package legal

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidAcceptance = errors.New("invalid legal acceptance")

type Acceptance struct {
	ID              string
	IdentityID      string
	DocumentID      string
	DocumentVersion string
	EvidenceHash    string
	AcceptedAt      time.Time
}

func NewAcceptance(
	id string,
	identityID string,
	documentID string,
	documentVersion string,
	evidenceHash string,
	acceptedAt time.Time,
) (Acceptance, error) {
	acceptance := Acceptance{
		ID:              strings.TrimSpace(id),
		IdentityID:      strings.TrimSpace(identityID),
		DocumentID:      strings.TrimSpace(documentID),
		DocumentVersion: strings.TrimSpace(documentVersion),
		EvidenceHash:    strings.TrimSpace(evidenceHash),
		AcceptedAt:      acceptedAt,
	}
	if acceptance.ID == "" ||
		acceptance.IdentityID == "" ||
		acceptance.DocumentID == "" ||
		acceptance.DocumentVersion == "" ||
		acceptance.EvidenceHash == "" ||
		acceptance.AcceptedAt.IsZero() {
		return Acceptance{}, ErrInvalidAcceptance
	}
	return acceptance, nil
}

func (a Acceptance) Covers(documentID, documentVersion string) bool {
	return a.DocumentID == documentID && a.DocumentVersion == documentVersion
}
